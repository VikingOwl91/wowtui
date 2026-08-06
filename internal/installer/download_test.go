package installer

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteArchiveHappyPath(t *testing.T) {
	content := strings.Repeat("addon-bytes", 100)
	dest := filepath.Join(t.TempDir(), "download.zip")

	sha, size, err := WriteArchive(strings.NewReader(content), dest, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len(content)) {
		t.Errorf("size = %d, want %d", size, len(content))
	}

	sum := sha256.Sum256([]byte(content))
	if sha != hex.EncodeToString(sum[:]) {
		t.Errorf("sha = %q", sha)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Error("file content mismatch")
	}
}

func TestWriteArchiveEnforcesMaxSize(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "download.zip")
	_, _, err := WriteArchive(strings.NewReader(strings.Repeat("x", 100)), dest, 10)
	if err == nil {
		t.Fatal("expected error for size limit")
	}
}
