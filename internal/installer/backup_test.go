package installer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupCopiesFolders(t *testing.T) {
	addonsDir := filepath.Join(t.TempDir(), "AddOns")
	makeTree(t, filepath.Join(addonsDir, "MyAddon"))
	backupRoot := filepath.Join(t.TempDir(), "backups")

	path, err := Backup(addonsDir, []string{"MyAddon"}, backupRoot, "github-Owner-MyAddon")
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(path, "MyAddon", "sub", "b.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "bbb" {
		t.Errorf("backup content = %q", content)
	}
	if _, err := os.Stat(filepath.Join(addonsDir, "MyAddon")); err != nil {
		t.Error("backup must not touch the original folders")
	}
}

func TestBackupMissingFolderErrors(t *testing.T) {
	addonsDir := filepath.Join(t.TempDir(), "AddOns")
	if err := os.MkdirAll(addonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Backup(addonsDir, []string{"Nope"}, t.TempDir(), "label"); err == nil {
		t.Fatal("expected error for missing folder")
	}
}
