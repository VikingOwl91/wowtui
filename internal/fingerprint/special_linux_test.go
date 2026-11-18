package fingerprint

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestCompareRejectsFIFO(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "Core/file.lua", "core")
	if err := syscall.Mkfifo(filepath.Join(dir, "Core", "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Compare(t.Context(), dir, Release{"1", []File{{"Core/file.lua", digest("core")}}}); err == nil {
		t.Fatal("accepted a named pipe")
	}
}

func TestCompareRejectsUnsafeLocalName(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "Core/file.lua", "core")
	put(t, dir, "Core/local\x1b.lua", "edit")
	_, err := Compare(t.Context(), dir, Release{"1", []File{{"Core/file.lua", digest("core")}}})
	if err == nil || strings.ContainsRune(err.Error(), '\x1b') {
		t.Fatalf("unsafe path handling returned %q", err)
	}
}
