package installer

import (
	"os"
	"path/filepath"
	"testing"
)

func makeTree(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("aaa"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "b.txt"), []byte("bbb"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCopyDir(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	makeTree(t, src)
	dst := filepath.Join(t.TempDir(), "dst")

	if err := CopyDir(src, dst); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(dst, "sub", "b.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "bbb" {
		t.Errorf("content = %q", content)
	}
	info, err := os.Stat(filepath.Join(dst, "sub", "b.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o, want 600", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(src, "a.txt")); err != nil {
		t.Error("source must stay intact after copy")
	}
}

func TestCopyDirRejectsExistingDestination(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	makeTree(t, src)
	dst := filepath.Join(t.TempDir(), "dst")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := CopyDir(src, dst); err == nil {
		t.Fatal("expected error for existing destination")
	}
}

func TestMoveDir(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	makeTree(t, src)
	dst := filepath.Join(t.TempDir(), "dst")

	if err := MoveDir(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "a.txt")); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("source must be gone after move")
	}
}
