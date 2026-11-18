//go:build linux && m3native

package acceptance

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestNativeInstallUpdate runs the shipped entry point with both layers required.
func TestNativeInstallUpdate(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Fatalf("M3 native acceptance requires amd64 or arm64, got %s", runtime.GOARCH)
	}
	tmp := t.TempDir()
	binary := filepath.Join(tmp, "wowtui")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building wowtui: %v\n%s", err, output)
	}

	var version atomic.Int32
	version.Store(1)
	archives := [][]byte{
		makeZIP(t, "1"), makeZIP(t, "2"), []byte("this download is not a ZIP file"),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := version.Load()
		w.Header().Set("ETag", fmt.Sprintf("\"v%d\"", n))
		if r.Method != http.MethodHead {
			w.Write(archives[n-1])
		}
	}))
	t.Cleanup(server.Close)
	wow := filepath.Join(tmp, "wow")
	iface := filepath.Join(wow, "_retail_", "Interface")
	addons := filepath.Join(iface, "AddOns")
	if err := os.MkdirAll(addons, 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(tmp, "config.toml")
	configData := fmt.Sprintf("[wow]\npaths = [%q]\n[security]\nsandbox = \"required\"\nseccomp = \"required\"\n", wow)
	if err := os.WriteFile(config, []byte(configData), 0o600); err != nil {
		t.Fatal(err)
	}
	registry := filepath.Join(tmp, "registry.json")
	run := func(wantSuccess bool, args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, append([]string{"--config", config, "--registry", registry}, args...)...)
		cmd.Dir = tmp
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + tmp, "XDG_CACHE_HOME=" + filepath.Join(tmp, "cache")}
		output, err := cmd.CombinedOutput()
		if ctx.Err() != nil || (err == nil) != wantSuccess {
			t.Fatalf("%v: %v\n%s", args, err, output)
		}
		return string(output)
	}
	assertFiltered := func(output string) {
		t.Helper()
		if strings.Count(output, "extract backend: bubblewrap+seccomp") != 1 {
			t.Fatalf("expected one filtered extraction; got:\n%s", output)
		}
	}
	toc := filepath.Join(addons, "TestAddon", "TestAddon.toc")
	assertFiltered(run(true, "add", "direct:"+server.URL+"/addon.zip"))
	if got := readFile(t, toc); got != "## Version: 1" {
		t.Fatalf("installed TOC = %q", got)
	}

	version.Store(2)
	for _, args := range [][]string{{"check"}, {"update", "--all", "--dry-run"}} {
		if output := run(true, args...); strings.Contains(output, "extract backend:") {
			t.Fatalf("%v extracted an archive:\n%s", args, output)
		}
	}
	assertFiltered(run(true, "update", "--all"))
	if got := readFile(t, toc); got != "## Version: 2" {
		t.Fatalf("updated TOC = %q", got)
	}
	registryBefore := readFile(t, registry)
	treeBefore := snapshot(t, addons)
	version.Store(3)
	assertFiltered(run(false, "update", "--all"))
	if readFile(t, registry) != registryBefore || !reflect.DeepEqual(snapshot(t, addons), treeBefore) {
		t.Fatal("failed update changed registry or installed addon tree")
	}
	entries, err := os.ReadDir(iface)
	if err != nil || len(entries) != 1 || entries[0].Name() != "AddOns" {
		t.Fatalf("unexpected entries after extraction attempts: %v (%v)", entries, err)
	}
}

func makeZIP(t *testing.T, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("TestAddon/TestAddon.toc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(f, "## Version: "+version); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	err := fs.WalkDir(os.DirFS(root), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		switch {
		case entry.IsDir():
			files[path] = "directory"
		case entry.Type().IsRegular():
			files[path] = "file " + readFile(t, filepath.Join(root, path))
		default:
			return fmt.Errorf("unexpected file type at %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
