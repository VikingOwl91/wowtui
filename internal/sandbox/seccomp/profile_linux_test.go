//go:build linux && (amd64 || arm64)

package seccomp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// The native gate starts the same binary twice under Bubblewrap, toggling only
// the production filter. No syscall with side effects is needed to prove denial.
func TestNativeProfile(t *testing.T) {
	if os.Getenv("WOWTUI_TEST_NATIVE_SANDBOX") != "1" {
		t.Skip("set WOWTUI_TEST_NATIVE_SANDBOX=1 on a native Bubblewrap host")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	program, err := Profile()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "worker.bpf")
	if err := os.WriteFile(path, program, 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"plain", "filtered"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			args := []string{"--unshare-all", "--die-with-parent", "--new-session",
				"--ro-bind", self, "/probe", "--proc", "/proc", "--dev", "/dev",
				"--clearenv", "--setenv", "WOWTUI_PROBE_MODE", mode}
			for _, dir := range []string{"/lib", "/lib64", "/usr/lib", "/usr/lib64"} {
				args = append(args, "--ro-bind-try", dir, dir)
			}
			var f *os.File
			if mode == "filtered" {
				f, err = os.Open(path)
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				args = append(args, "--seccomp", "3")
			}
			args = append(args, "--", "/probe", "-test.run=^TestProfileProbe$")
			cmd := exec.CommandContext(ctx, "bwrap", args...)
			if f != nil {
				cmd.ExtraFiles = []*os.File{f}
			}
			out, err := cmd.CombinedOutput()
			if mode == "filtered" {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != 128+int(unix.SIGSYS) || !bytes.Contains(out, []byte("calling getpgid")) {
					t.Fatalf("want SIGSYS at getpgid; got %v\n%s", err, out)
				}
				return
			}
			if err != nil || !bytes.Contains(out, []byte("PASS")) {
				t.Fatalf("probe failed: %v\n%s", err, out)
			}
		})
	}
}

func TestProfileProbe(t *testing.T) {
	mode := os.Getenv("WOWTUI_PROBE_MODE")
	if mode == "" {
		t.Skip("native probe subprocess only")
	}
	fmt.Fprintln(os.Stdout, "calling getpgid")
	_, err := unix.Getpgid(0)
	if err != nil {
		t.Fatal(err)
	}
}
