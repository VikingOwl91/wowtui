package worker

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/installer"
)

var smallLimits = installer.Limits{MaxFiles: 2, MaxFileSize: 20, MaxTotalSize: 30}

// The test binary can stand in for the private subcommand without building
// another executable or relying on environment flags inherited by the child.
func TestMain(m *testing.M) {
	if handled, code := Dispatch(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); handled {
		os.Exit(code)
	}
	if len(os.Args) == 2 {
		switch os.Args[1] {
		case "wait":
			time.Sleep(time.Minute)
			os.Exit(0)
		case "excess", "trailing", "contradiction", "early-exit":
			_, _ = io.Copy(io.Discard, os.Stdin)
			switch os.Args[1] {
			case "excess":
				_, _ = os.Stdout.Write(bytes.Repeat([]byte{' '}, messageLimit+1))
			case "trailing":
				_, _ = io.WriteString(os.Stdout, `{"ok":true} {}`)
			case "contradiction":
				_, _ = io.WriteString(os.Stdout, `{"ok":true,"error":"failure"}`)
			case "early-exit":
				_, _ = io.WriteString(os.Stdout, `{"ok":true}`)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}
	os.Exit(m.Run())
}

type zipEntry struct {
	name string
	data string
	mode os.FileMode
}

func fixture(t *testing.T, entries ...zipEntry) string {
	t.Helper()
	archive := filepath.Join(t.TempDir(), "addon.zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for _, entry := range entries {
		h := &zip.FileHeader{Name: entry.name}
		h.SetMode(entry.mode)
		file, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(file, entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return archive
}

func TestRunExtractsArchive(t *testing.T) {
	archive := fixture(t, zipEntry{"Example/Example.toc", "## Title: Example", 0644})
	staging := t.TempDir()
	if err := Run(t.Context(), exec.CommandContext(t.Context(), os.Args[0], Argument), archive, staging, smallLimits); err != nil {
		t.Fatal(err)
	}
	if err := Validate(staging, smallLimits); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(staging, "Example", "Example.toc")); err != nil || string(b) != "## Title: Example" {
		t.Fatalf("got %q, %v", b, err)
	}
}

// Exercise the installer's archive rejection cases across the process boundary.
func TestRunEnforcesArchiveChecks(t *testing.T) {
	file := zipEntry{"Addon/a", "12345", 0644}
	second := zipEntry{"Addon/b", "67890", 0644}
	for _, tc := range []struct {
		name    string
		entries []zipEntry
		limits  installer.Limits
		want    string
	}{
		{"parent path", []zipEntry{{"../outside", "", 0644}}, smallLimits, "escapes destination"},
		{"nested parent", []zipEntry{{"Addon/../../outside", "", 0644}}, smallLimits, "escapes destination"},
		{"absolute", []zipEntry{{"/outside", "", 0644}}, smallLimits, "absolute path"},
		{"symlink", []zipEntry{{"Addon/link", "a", os.ModeSymlink | 0777}}, smallLimits, "unsupported file type"},
		{"count", []zipEntry{file, second}, installer.Limits{MaxFiles: 1, MaxFileSize: 5, MaxTotalSize: 10}, "maximum file count"},
		{"file size", []zipEntry{file}, installer.Limits{MaxFiles: 2, MaxFileSize: 4, MaxTotalSize: 10}, "maximum file size"},
		{"total size", []zipEntry{file, second}, installer.Limits{MaxFiles: 2, MaxFileSize: 5, MaxTotalSize: 9}, "maximum total size"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			archive := fixture(t, tc.entries...)
			cmd := exec.CommandContext(t.Context(), os.Args[0], Argument)
			err := Run(t.Context(), cmd, archive, t.TempDir(), tc.limits)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted %q, got %v", tc.want, err)
			}
		})
	}
}

func TestServeRejectsInvalidRequest(t *testing.T) {
	for _, data := range []string{"", "null", "{}", `{"unknown":true}`, `{"archive":"missing","staging":"unused","limits":{}}`, "{}{}", strings.Repeat(" ", messageLimit+1)} {
		if err := Serve(strings.NewReader(data), io.Discard); err == nil {
			t.Fatalf("accepted malformed request (%d bytes)", len(data))
		}
	}
}

func TestRunRejectsInvalidWorkerReplies(t *testing.T) {
	for _, arg := range []string{"excess", "trailing", "contradiction", "early-exit"} {
		t.Run(arg, func(t *testing.T) {
			if err := Run(t.Context(), exec.CommandContext(t.Context(), os.Args[0], arg), "unused", "unused", smallLimits); err == nil {
				t.Fatal("accepted unsuccessful worker")
			}
		})
	}
}

func TestRunPropagatesArchiveFailure(t *testing.T) {
	dir := t.TempDir()
	err := Run(t.Context(), exec.CommandContext(t.Context(), os.Args[0], Argument), filepath.Join(dir, "missing.zip"), dir, smallLimits)
	if err == nil || !strings.Contains(err.Error(), "missing.zip") {
		t.Fatalf("wanted extractor diagnostic, got %v", err)
	}
}

func TestRunReapsCancelledWorker(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "wait")
	err := Run(ctx, cmd, "unused", "unused", smallLimits)
	if !errors.Is(err, context.DeadlineExceeded) || cmd.ProcessState == nil {
		t.Fatalf("worker result %v, state %v", err, cmd.ProcessState)
	}
}

func TestValidateRejectsBadStaging(t *testing.T) {
	for _, name := range []string{"root-file", "root-symlink", "symlink", "hardlink", "file-count", "file-size", "total-size"} {
		t.Run(name, func(t *testing.T) {
			stage := t.TempDir()
			file := filepath.Join(stage, "one")
			limits := smallLimits
			if err := os.WriteFile(file, []byte("data"), 0600); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "root-file":
				stage = file
			case "root-symlink":
				link := filepath.Join(t.TempDir(), "stage")
				if err := os.Symlink(stage, link); err != nil {
					t.Skipf("symlink unavailable: %s", err)
				}
				stage = link
			case "symlink":
				if err := os.Symlink(file, filepath.Join(stage, "link")); err != nil {
					t.Skipf("symlink unavailable: %s", err)
				}
			case "hardlink":
				if err := os.Link(file, filepath.Join(t.TempDir(), "link")); err != nil {
					t.Fatal(err)
				}
			case "file-size":
				limits.MaxFileSize = 3
			case "file-count", "total-size":
				if err := os.WriteFile(filepath.Join(stage, "two"), []byte("data"), 0600); err != nil {
					t.Fatal(err)
				}
				if name == "file-count" {
					limits.MaxFiles = 1
				} else {
					limits.MaxTotalSize = 7
				}
			}
			if err := Validate(stage, limits); err == nil {
				t.Fatal("accepted invalid tree")
			}
		})
	}
}
