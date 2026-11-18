package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/sys/unix"
	"somegit.dev/vikingowl/wowtui/internal/config"
	"somegit.dev/vikingowl/wowtui/internal/installer"
	"somegit.dev/vikingowl/wowtui/internal/sandbox/seccomp"
	"somegit.dev/vikingowl/wowtui/internal/sandbox/worker"
)

const sandboxArchive = "/input/archive.zip"
const sandboxStaging = "/output"

type runner struct {
	launch
	bwrap   string
	profile []byte
}

// Look only in system bin directories. A launcher selected from a user's
// project PATH would inherit access to the install before isolation.
func findBwrap() (string, error) {
	for _, path := range []string{"/usr/bin/bwrap", "/bin/bwrap"} {
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			return "", errors.New("system Bubblewrap binary is not executable")
		}
		return path, nil
	}
	return "", nil
}

func prepare(ctx context.Context, self string, policy config.Security, archive, staging string) (runner, error) {
	path, err := findBwrap()
	if err != nil {
		return runner{}, err
	}
	probe := func(profile []byte) error {
		return probeWorker(ctx, path, self, archive, staging, profile)
	}
	return selectLinux(path, policy, probe, seccomp.Profile)
}

var errNamespace = errors.New("Bubblewrap namespace creation is unavailable")
var errFilter = errors.New("kernel cannot install a seccomp filter")

func selectLinux(path string, policy config.Security, probe func([]byte) error, profile func() ([]byte, error)) (runner, error) {
	if path == "" {
		selected, err := withoutSandbox(policy, "Bubblewrap executable unavailable")
		return runner{launch: selected}, err
	}
	if err := probe(nil); err != nil {
		if !errors.Is(err, errNamespace) {
			return runner{}, err
		}
		selected, err := withoutSandbox(policy, errNamespace.Error())
		return runner{launch: selected}, err
	}
	r := runner{launch: launch{backend: "bubblewrap"}, bwrap: path}
	filter, err := profile()
	if err == nil {
		if len(filter) == 0 || len(filter)%8 != 0 {
			return runner{}, errors.New("invalid seccomp profile instruction length")
		}
		err = probe(filter)
		if err == nil {
			r.profile = filter
			r.backend = "bubblewrap+seccomp"
			return r, nil
		}
		if errors.Is(err, errFilter) {
			// EINVAL can also identify a broken BPF program. Only downgrade
			// if the kernel rejects a known-valid single ALLOW instruction.
			allow := []byte{unix.BPF_RET | unix.BPF_K, 0, 0, 0, 0, 0, 0xff, 0x7f}
			if control := probe(allow); !errors.Is(control, errFilter) {
				if control != nil {
					return runner{}, control
				}
				return runner{}, errors.New("kernel rejected the worker seccomp profile")
			}
		}
	}
	if !errors.Is(err, seccomp.ErrUnsupported) && !errors.Is(err, errFilter) {
		return runner{}, err
	}
	if policy.Seccomp == "required" {
		return runner{}, fmt.Errorf("required seccomp unavailable: %w", err)
	}
	r.warning = "seccomp unavailable: " + err.Error()
	return r, nil
}

// Each invocation owns a fresh filter descriptor. Bubblewrap consumes it
// before exec. Worker dispatch closes any descriptors its caller inherited.
func isolated(ctx context.Context, path, self, archive, staging string, profile []byte, argv ...string) (*exec.Cmd, func(), error) {
	args := []string{"--die-with-parent", "--new-session", "--unshare-all", "--as-pid-1", "--cap-drop", "ALL", "--clearenv",
		"--ro-bind", self, "/worker",
		"--ro-bind-try", "/lib", "/lib", "--ro-bind-try", "/lib64", "/lib64",
		"--ro-bind-try", "/usr/lib", "/usr/lib", "--ro-bind-try", "/usr/lib64", "/usr/lib64",
		"--proc", "/proc", "--dev", "/dev",
		"--ro-bind", archive, sandboxArchive, "--bind", staging, sandboxStaging,
		"--chdir", sandboxStaging, "--remount-ro", "/"}
	var files []*os.File
	closeFD := func() {}
	if profile != nil {
		fd, err := unix.MemfdCreate("wowtui-seccomp", unix.MFD_CLOEXEC)
		if err != nil {
			return nil, nil, err
		}
		f := os.NewFile(uintptr(fd), "seccomp")
		closeFD = func() { _ = f.Close() }
		if _, err := f.Write(profile); err != nil {
			closeFD()
			return nil, nil, err
		}
		if _, err := f.Seek(0, 0); err != nil {
			closeFD()
			return nil, nil, err
		}
		files = []*os.File{f}
		args = append(args, "--seccomp", "3")
	}
	args = append(args, "--", "/worker")
	cmd := exec.CommandContext(ctx, path, append(args, argv...)...)
	cmd.Env = []string{}
	cmd.ExtraFiles = files
	return cmd, closeFD, nil
}

type diagnostic struct{ buf bytes.Buffer }

func (d *diagnostic) Write(p []byte) (int, error) {
	n := len(p)
	if left := 4096 - d.buf.Len(); left < len(p) {
		p = p[:left]
	}
	_, _ = d.buf.Write(p)
	return n, nil
}

func probeWorker(ctx context.Context, path, self, archive, staging string, profile []byte) error {
	cmd, closeFD, err := isolated(ctx, path, self, archive, staging, profile, worker.ProbeArgument)
	if err != nil {
		return err
	}
	defer closeFD()
	var stderr diagnostic
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Only launcher diagnostics from a harmless probe qualify for
		// fallback. Never parse stderr from an archive-processing worker.
		msg := strings.TrimSpace(stderr.buf.String())
		if profile == nil && (strings.HasPrefix(msg, "bwrap: No permissions to create a new namespace,") ||
			strings.HasPrefix(msg, "bwrap: Creating new namespace failed, likely because the kernel does not support user namespaces") ||
			msg == "bwrap: Creating new namespace failed: Operation not permitted" ||
			msg == "bwrap: Creating new namespace failed: nesting depth or /proc/sys/user/max_*_namespaces exceeded (ENOSPC)" ||
			msg == "bwrap: loopback: Failed to create NETLINK_ROUTE socket: Operation not permitted") {
			return errNamespace
		}
		if profile != nil && (strings.HasPrefix(msg, "bwrap: Unable to set up system call filtering as requested: prctl(PR_SET_SECCOMP) reported EINVAL.") ||
			msg == "bwrap: prctl(PR_SET_SECCOMP): Operation not permitted" || msg == "bwrap: prctl(PR_SET_SECCOMP): Permission denied") {
			return errFilter
		}
		return fmt.Errorf("Bubblewrap worker capability probe failed: %w", err)
	}
	return nil
}

func (r runner) extract(ctx context.Context, self, archive, staging string, limits installer.Limits) error {
	if r.bwrap == "" {
		return direct(ctx, self, archive, staging, limits)
	}
	cmd, closeFD, err := isolated(ctx, r.bwrap, self, archive, staging, r.profile, worker.Argument)
	if err != nil {
		return err
	}
	defer closeFD()
	return worker.Run(ctx, cmd, sandboxArchive, sandboxStaging, limits)
}
