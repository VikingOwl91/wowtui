package installer

import (
	"archive/zip"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type zipEntry struct {
	name    string
	content string
	mode    fs.FileMode
}

func writeZip(t *testing.T, path string, entries []zipEntry) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	for _, e := range entries {
		hdr := &zip.FileHeader{Name: e.name}
		if e.mode != 0 {
			hdr.SetMode(e.mode)
		} else if strings.HasSuffix(e.name, "/") {
			hdr.SetMode(fs.ModeDir | 0o755)
		} else {
			hdr.SetMode(0o644)
		}
		fw, err := w.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(e.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func defaultLimits() Limits {
	return Limits{MaxFiles: 100, MaxTotalSize: 1 << 20, MaxFileSize: 1 << 20}
}

func TestExtractZipHappyPath(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "addon.zip")
	writeZip(t, zipPath, []zipEntry{
		{name: "MyAddon/"},
		{name: "MyAddon/MyAddon.toc", content: "## Version: 1.0"},
		{name: "MyAddon/sub/lib.lua", content: "print('hi')"},
	})
	dest := filepath.Join(t.TempDir(), "out")

	if err := ExtractZip(zipPath, dest, defaultLimits()); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(dest, "MyAddon", "MyAddon.toc"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "## Version: 1.0" {
		t.Errorf("toc content = %q", content)
	}
	if _, err := os.Stat(filepath.Join(dest, "MyAddon", "sub", "lib.lua")); err != nil {
		t.Error(err)
	}
}

func TestExtractZipRejectsZipSlip(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "evil.zip")
	writeZip(t, zipPath, []zipEntry{
		{name: "../evil.txt", content: "pwned"},
	})
	dest := filepath.Join(t.TempDir(), "out")

	if err := ExtractZip(zipPath, dest, defaultLimits()); err == nil {
		t.Fatal("expected error for zip-slip entry")
	}
	if _, err := os.Stat(filepath.Join(t.TempDir(), "evil.txt")); !os.IsNotExist(err) {
		t.Error("zip-slip file escaped destination")
	}
}

func TestExtractZipRejectsNestedZipSlip(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "evil.zip")
	writeZip(t, zipPath, []zipEntry{
		{name: "foo/../../evil.txt", content: "pwned"},
	})
	dest := filepath.Join(t.TempDir(), "out")

	if err := ExtractZip(zipPath, dest, defaultLimits()); err == nil {
		t.Fatal("expected error for nested zip-slip entry")
	}
}

func TestExtractZipRejectsAbsolutePaths(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "evil.zip")
	writeZip(t, zipPath, []zipEntry{
		{name: "/etc/passwd", content: "pwned"},
	})
	dest := filepath.Join(t.TempDir(), "out")

	if err := ExtractZip(zipPath, dest, defaultLimits()); err == nil {
		t.Fatal("expected error for absolute path entry")
	}
}

func TestExtractZipRejectsSymlinks(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "evil.zip")
	writeZip(t, zipPath, []zipEntry{
		{name: "link", content: "/etc/passwd", mode: fs.ModeSymlink | 0o777},
	})
	dest := filepath.Join(t.TempDir(), "out")

	if err := ExtractZip(zipPath, dest, defaultLimits()); err == nil {
		t.Fatal("expected error for symlink entry")
	}
}

func TestExtractZipEnforcesMaxFiles(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "many.zip")
	writeZip(t, zipPath, []zipEntry{
		{name: "a.txt", content: "a"},
		{name: "b.txt", content: "b"},
		{name: "c.txt", content: "c"},
	})
	dest := filepath.Join(t.TempDir(), "out")
	limits := defaultLimits()
	limits.MaxFiles = 2

	if err := ExtractZip(zipPath, dest, limits); err == nil {
		t.Fatal("expected error for file count limit")
	}
}

func TestExtractZipEnforcesMaxFileSize(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "big.zip")
	writeZip(t, zipPath, []zipEntry{
		{name: "big.bin", content: strings.Repeat("x", 100)},
	})
	dest := filepath.Join(t.TempDir(), "out")
	limits := defaultLimits()
	limits.MaxFileSize = 10

	if err := ExtractZip(zipPath, dest, limits); err == nil {
		t.Fatal("expected error for file size limit")
	}
}

func TestExtractZipEnforcesMaxTotalSize(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "sum.zip")
	writeZip(t, zipPath, []zipEntry{
		{name: "a.txt", content: strings.Repeat("x", 30)},
		{name: "b.txt", content: strings.Repeat("y", 30)},
	})
	dest := filepath.Join(t.TempDir(), "out")
	limits := defaultLimits()
	limits.MaxTotalSize = 50

	if err := ExtractZip(zipPath, dest, limits); err == nil {
		t.Fatal("expected error for total size limit")
	}
}

func TestExtractZipMissingArchive(t *testing.T) {
	if err := ExtractZip(filepath.Join(t.TempDir(), "nope.zip"), t.TempDir(), defaultLimits()); err == nil {
		t.Fatal("expected error for missing archive")
	}
}
