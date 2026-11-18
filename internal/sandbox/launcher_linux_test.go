package sandbox

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"somegit.dev/vikingowl/wowtui/internal/config"
	"somegit.dev/vikingowl/wowtui/internal/installer"
	"somegit.dev/vikingowl/wowtui/internal/sandbox/seccomp"
	"somegit.dev/vikingowl/wowtui/internal/sandbox/worker"
)

func TestSelectLinux(t *testing.T) {
	broken := errors.New("bad probe")
	requiredSandbox := config.Security{Sandbox: "required"}
	requiredFilter := config.Security{Seccomp: "required"}
	for _, tc := range []struct {
		name                                              string
		policy                                            config.Security
		missing                                           bool
		namespaceErr, profileErr, filteredErr, controlErr error
		backend                                           string
		warn                                              bool
	}{
		{name: "auto", backend: "bubblewrap+seccomp"},
		{name: "required", policy: requiredFilter, backend: "bubblewrap+seccomp"},
		{name: "missing", missing: true, backend: "worker", warn: true},
		{name: "missing required", missing: true, policy: requiredSandbox},
		{name: "missing required filter", missing: true, policy: requiredFilter},
		{name: "denied namespace", namespaceErr: errNamespace, backend: "worker", warn: true},
		{name: "denied required", namespaceErr: errNamespace, policy: requiredFilter},
		{name: "bad worker probe", namespaceErr: broken},
		{name: "unknown arch", profileErr: seccomp.ErrUnsupported, backend: "bubblewrap", warn: true},
		{name: "unknown arch required", profileErr: seccomp.ErrUnsupported, policy: requiredFilter},
		{name: "kernel lacks seccomp", filteredErr: errFilter, controlErr: errFilter, backend: "bubblewrap", warn: true},
		{name: "kernel lacks seccomp strict", filteredErr: errFilter, controlErr: errFilter, policy: requiredFilter},
		{name: "sandbox strict with no seccomp", filteredErr: errFilter, controlErr: errFilter, policy: requiredSandbox, backend: "bubblewrap", warn: true},
		{name: "broken generator", profileErr: broken},
		{name: "rejected production filter", filteredErr: errFilter},
		{name: "failed control", filteredErr: errFilter, controlErr: broken},
		{name: "worker crashes", filteredErr: broken},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := "/usr/bin/bwrap"
			if tc.missing {
				path = ""
			}
			profile := func() ([]byte, error) { return make([]byte, 16), tc.profileErr }
			probe := func(b []byte) error {
				switch len(b) {
				case 0:
					return tc.namespaceErr
				case 16:
					return tc.filteredErr
				default:
					return tc.controlErr
				}
			}
			run, err := selectLinux(path, tc.policy, probe, profile)
			if (err != nil) != (tc.backend == "") || run.backend != tc.backend || (run.warning != "") != tc.warn {
				t.Fatalf("backend %q warning %q error %v", run.backend, run.warning, err)
			}
		})
	}
}

func TestMalformedProfile(t *testing.T) {
	for _, profile := range [][]byte{nil, {1}} {
		_, err := selectLinux("/usr/bin/bwrap", config.Security{}, func(b []byte) error {
			if b != nil {
				t.Fatal("malformed filter reached launcher")
			}
			return nil
		}, func() ([]byte, error) { return profile, nil })
		if err == nil {
			t.Fatal("malformed filter accepted")
		}
	}
}

func TestNativeIsolation(t *testing.T) {
	if os.Getenv("M3_NATIVE") != "1" {
		t.Skip("requires native Linux with Bubblewrap")
	}
	bwrap, err := findBwrap()
	if err != nil || bwrap == "" {
		t.Fatalf("find Bubblewrap: %v", err)
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "home", "private-marker")
	if err := os.Mkdir(filepath.Dir(marker), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(marker)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	// Model a descriptor passed into wowtui by its caller without FD_CLOEXEC.
	if _, err := unix.FcntlInt(f.Fd(), unix.F_SETFD, 0); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WOWTUI_SYNTHETIC_CREDENTIAL", "synthetic")
	stage := t.TempDir()
	archive := filepath.Join(dir, "archive")
	if err := os.WriteFile(archive, []byte("synthetic archive"), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	addr := listener.Addr().String()
	c, err := net.DialTimeout("tcp4", addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd, closeFD, err := isolated(ctx, bwrap, self, archive, stage, nil, "-test.run=^TestBoundaryChild$")
	if err != nil {
		t.Fatal(err)
	}
	defer closeFD()
	cmd.Stdin = strings.NewReader(marker + "\n" + addr)
	if out, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(out), "PASS") {
		t.Fatalf("boundary probe: %v %s", err, out)
	}
	if got, err := os.ReadFile(filepath.Join(stage, "ok")); err != nil || string(got) != "ok" {
		t.Fatalf("host staging marker: %v", err)
	}
}

func TestBoundaryChild(t *testing.T) {
	if len(os.Args) != 2 || os.Args[1] != "-test.run=^TestBoundaryChild$" {
		t.Skip("private test subprocess")
	}
	if handled, code := worker.Dispatch([]string{worker.ProbeArgument}, os.Stdin, os.Stdout, os.Stderr); !handled || code != 0 {
		t.Fatal("worker descriptor cleanup failed")
	}
	if os.Getpid() != 1 {
		t.Fatal("unexpected namespace supervisor")
	}
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 8192))
	if err != nil {
		t.Fatal(err)
	}
	marker, addr, ok := strings.Cut(string(data), "\n")
	if !ok {
		t.Fatal("missing synthetic markers")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("host marker is visible, stat error %v", err)
	}
	if _, ok := os.LookupEnv("WOWTUI_SYNTHETIC_CREDENTIAL"); ok {
		t.Fatal("host environment was inherited")
	}
	if _, err := os.ReadFile(sandboxArchive); err != nil {
		t.Fatal(err)
	}
	if f, err := os.OpenFile(sandboxArchive, os.O_WRONLY, 0); err == nil {
		f.Close()
		t.Fatal("archive is writable")
	}
	if err := os.WriteFile(filepath.Join(sandboxStaging, "ok"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if c, err := net.DialTimeout("tcp4", addr, time.Second); err == nil {
		c.Close()
		t.Fatal("host listener reachable")
	}
	fds, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	for _, fd := range fds {
		target, err := os.Readlink("/proc/self/fd/" + fd.Name())
		if err == nil && target == marker {
			t.Fatal("host marker descriptor was inherited")
		}
	}
}

func TestNativeWorkerFailure(t *testing.T) {
	if os.Getenv("M3_NATIVE") != "1" {
		t.Skip("requires native Linux with Bubblewrap")
	}
	self := filepath.Join(t.TempDir(), "wowtui")
	build := exec.Command("go", "build", "-o", self, "../..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build %v %s", err, out)
	}
	stage := t.TempDir()
	corrupt := filepath.Join(t.TempDir(), "archive")
	if err := os.WriteFile(corrupt, []byte("synthetic corrupt archive"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	r, err := prepare(ctx, self, config.Security{Seccomp: "required"}, corrupt, stage)
	if err != nil {
		t.Fatal(err)
	}
	if r.backend != "bubblewrap+seccomp" {
		t.Fatalf("unexpected backend %s", r.backend)
	}
	err = r.extract(ctx, self, corrupt, stage, installer.Limits{MaxFiles: 8, MaxFileSize: 128, MaxTotalSize: 1024})
	if err == nil || !strings.Contains(err.Error(), "zip: not a valid zip file") {
		t.Fatalf("worker archive result: %v", err)
	}
}
