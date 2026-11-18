package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFingerprintCommand(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
		valid bool
	}{
		{"release", []string{"Core/z.lua", "Core/a.lua"}, true},
		{"traversal", []string{"Core/a", "Core/../other"}, false},
		{"duplicate", []string{"Core/A", "Core/a"}, false},
		{"symlink", []string{"Core/link"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, "release.zip")
			out := filepath.Join(dir, "manifest.json")
			f, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			zw := zip.NewWriter(f)
			for _, name := range tc.files {
				h := &zip.FileHeader{Name: name}
				if tc.name == "symlink" {
					h.SetMode(os.ModeSymlink | 0777)
				}
				w, err := zw.CreateHeader(h)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = fmt.Fprint(w, name); err != nil {
					t.Fatal(err)
				}
			}
			if err := zw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			code, _, stderr := runCLI(t, deps{}, "fingerprint", "--version", "1.0", "--zip", archive, "--out", out)
			if !tc.valid {
				if code != 1 {
					t.Fatalf("code %d: %s", code, stderr)
				}
				if _, err := os.Stat(out); !os.IsNotExist(err) {
					t.Fatalf("output after error: %v", err)
				}
				return
			}
			if code != 0 {
				t.Fatalf("code %d: %s", code, stderr)
			}
			data, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			var release struct {
				Version string
				Files   []struct{ Path, SHA256 string }
			}
			if err := json.Unmarshal(data, &release); err != nil {
				t.Fatal(err)
			}
			if release.Version != "1.0" || len(release.Files) != 2 || release.Files[0].Path != "Core/a.lua" || release.Files[0].SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte("Core/a.lua"))) || release.Files[1].Path != "Core/z.lua" {
				t.Fatalf("manifest = %s", data)
			}
		})
	}
}

func TestFingerprintBoundsZIPDirectory(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "metadata.zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	comment := strings.Repeat("a", 60000)
	for i := 0; i < 150; i++ {
		_, err := w.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("Core/%d", i), Comment: comment})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := manifestFromZIP(archive, "1"); err == nil || !strings.Contains(err.Error(), "metadata") {
		t.Fatalf("oversized ZIP directory: %v", err)
	}
}
