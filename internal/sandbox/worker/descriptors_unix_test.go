//go:build unix

package worker

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"testing"

	"golang.org/x/sys/unix"
)

// The child proves that the exact same descriptor is exposed before dispatch
// and closed by either private entry point. The marker is an empty temp file.
func TestPrivateDispatchDropsInheritedDescriptor(t *testing.T) {
	const env = "WOWTUI_TEST_WORKER_DISPATCH"
	if arg := os.Getenv(env); arg != "" {
		flags, err := unix.FcntlInt(3, unix.F_GETFD, 0)
		if err != nil || flags&unix.FD_CLOEXEC != 0 {
			t.Fatalf("expected inherited fd 3, got flags %d, %v", flags, err)
		}
		handled, code := Dispatch([]string{arg}, os.Stdin, os.Stdout, os.Stderr)
		if !handled || code != 0 {
			t.Fatalf("worker dispatch returned %t %d", handled, code)
		}
		if _, err := unix.FcntlInt(3, unix.F_GETFD, 0); !errors.Is(err, unix.EBADF) {
			t.Fatalf("fd 3 after dispatch: %v", err)
		}
		return
	}
	for _, arg := range []string{Argument, ProbeArgument} {
		t.Run(arg, func(t *testing.T) {
			marker, err := os.CreateTemp(t.TempDir(), "marker")
			if err != nil {
				t.Fatal(err)
			}
			defer marker.Close()
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestPrivateDispatchDropsInheritedDescriptor$")
			cmd.Env = []string{env + "=" + arg}
			cmd.ExtraFiles = []*os.File{marker}
			if arg == Argument {
				// A shared stdout belongs to the go test harness, so send the
				// request directly and inspect the child exit status.
				archive := fixture(t, zipEntry{"A/A.toc", "ok", 0644})
				data, err := json.Marshal(request{Archive: archive, Staging: t.TempDir(), Limits: smallLimits})
				if err != nil {
					t.Fatal(err)
				}
				cmd.Stdin = bytes.NewReader(data)
			}
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("child result: %v, %s", err, output)
			}
		})
	}
}
