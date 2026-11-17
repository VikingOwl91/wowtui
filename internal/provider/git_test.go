package provider

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestGitRepoName(t *testing.T) {
	tests := map[string]string{
		"https://github.com/Owner/Repo.git":        "Repo",
		"https://git.example.org/team/addon":       "addon",
		"ssh://git@git.example.org/team/addon.git": "addon",
	}
	for input, want := range tests {
		if got := gitRepoName(input); got != want {
			t.Errorf("gitRepoName(%q) = %q, want %q", input, got, want)
		}
	}
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	base := []string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false"}
	cmd := exec.Command("git", append(base, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}

func initGitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-b", "main")
	for name, content := range files {
		path := filepath.Join(repoDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return repoDir
}

func isolateTempDir(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	return tmp
}

func assertEmptyDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		t.Errorf("leaked temp entry %s", entry.Name())
	}
}

func TestGitDownloadZipsCheckout(t *testing.T) {
	requireGit(t)
	repoDir := initGitRepo(t, map[string]string{
		"Addon.toc":     "## Interface: 110000\n",
		"sub/Core.lua":  "print('core')\n",
		".gitignore":    "*.bak\n",
		"Libs/Lib.xml":  "<Ui/>\n",
		"README.md":     "readme\n",
		"Media/one.tga": "\x00\x01\x02",
	})
	runGit(t, repoDir, "add", "-A")
	runGit(t, repoDir, "commit", "-m", "init")
	tmp := isolateTempDir(t)

	reader, err := NewGit().Download(context.Background(), Release{Tag: "main", DownloadURL: "file://" + repoDir})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	assertEmptyDir(t, tmp)

	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	prefix := filepath.Base(repoDir) + "-main/"
	want := map[string]string{
		prefix + "Addon.toc":     "## Interface: 110000\n",
		prefix + "sub/Core.lua":  "print('core')\n",
		prefix + "Libs/Lib.xml":  "<Ui/>\n",
		prefix + "README.md":     "readme\n",
		prefix + "Media/one.tga": "\x00\x01\x02",
	}
	var names []string
	for _, file := range archive.File {
		names = append(names, file.Name)
		wantContent, ok := want[file.Name]
		if !ok {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != wantContent {
			t.Errorf("%s content = %q, want %q", file.Name, got, wantContent)
		}
	}
	var wantNames []string
	for name := range want {
		wantNames = append(wantNames, name)
	}
	sort.Strings(names)
	sort.Strings(wantNames)
	if strings.Join(names, ",") != strings.Join(wantNames, ",") {
		t.Errorf("archive entries = %v, want %v", names, wantNames)
	}
}

func TestGitDownloadRejectsSymlink(t *testing.T) {
	requireGit(t)
	if runtime.GOOS == "windows" {
		t.Skip("git for Windows checks symlinks out as plain files by default")
	}
	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("outside secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	repoDir := initGitRepo(t, map[string]string{"Addon.toc": "## Interface: 110000\n"})
	if err := os.Symlink(secret, filepath.Join(repoDir, "Data.lua")); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoDir, "add", "-A")
	runGit(t, repoDir, "commit", "-m", "init")
	tmp := isolateTempDir(t)

	reader, err := NewGit().Download(context.Background(), Release{Tag: "main", DownloadURL: "file://" + repoDir})
	if err == nil {
		data, _ := io.ReadAll(reader)
		reader.Close()
		t.Fatalf("Download succeeded with %d byte archive, want symlink error", len(data))
	}
	if !strings.Contains(err.Error(), "Data.lua") || !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error = %q, want it to name symlink Data.lua", err)
	}
	assertEmptyDir(t, tmp)
}

func TestZipDirectoryRejectsNonRegularFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Addon.toc"), []byte("## Interface: 110000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(root, "sock"))
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	defer listener.Close()

	err = zipDirectory(root, filepath.Join(t.TempDir(), "addon.zip"), "addon-main")
	if err == nil {
		t.Fatal("zipDirectory succeeded, want error for socket entry")
	}
	if !strings.Contains(err.Error(), `"sock"`) {
		t.Errorf("error = %q, want it to name sock", err)
	}
}
