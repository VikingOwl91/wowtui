package addon

import (
	"os"
	"path/filepath"
	"testing"
)

func makeInstall(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	addons := filepath.Join(root, "Interface", "AddOns")

	wa := filepath.Join(addons, "WeakAuras")
	writeToc(t, wa, "WeakAuras.toc", "## Version: 5.18.1\n## Interface: 110200\n")

	details := filepath.Join(addons, "Details")
	writeToc(t, details, "Details.toc", "## Version: 11.0.8\n## Interface: 110200\n")
	writeToc(t, details, "Details_Classic.toc", "## Version: 11.0.8\n## Interface: 40402\n")

	return root
}

func TestScan(t *testing.T) {
	root := makeInstall(t)

	folders, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 2 {
		t.Fatalf("got %d folders, want 2", len(folders))
	}
	if folders[0].Name != "Details" || folders[1].Name != "WeakAuras" {
		t.Fatalf("expected sorted names, got %q and %q", folders[0].Name, folders[1].Name)
	}
	if len(folders[0].Tocs) != 2 {
		t.Errorf("Details should have 2 toc files, got %d", len(folders[0].Tocs))
	}
	if got := folders[1].Version(); got != "5.18.1" {
		t.Errorf("WeakAuras version = %q", got)
	}
}

func TestScanFolderInterfaces(t *testing.T) {
	root := makeInstall(t)

	folders, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	got := folders[0].Interfaces()
	if len(got) != 2 || got[0] != 40402 || got[1] != 110200 {
		t.Errorf("Details interfaces = %v, want [40402 110200]", got)
	}
}

func TestScanIgnoresStrayFilesAndToclessDirs(t *testing.T) {
	root := makeInstall(t)
	addons := filepath.Join(root, "Interface", "AddOns")
	if err := os.WriteFile(filepath.Join(addons, "stray.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(addons, "EmptyDir"), 0o755); err != nil {
		t.Fatal(err)
	}

	folders, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 2 {
		t.Fatalf("got %d folders, want 2", len(folders))
	}
}

func TestScanMissingAddOnsDir(t *testing.T) {
	folders, err := Scan(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 0 {
		t.Fatalf("got %d folders, want 0", len(folders))
	}
}

func TestScanCaseInsensitiveExtension(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Interface", "AddOns", "Upper")
	writeToc(t, dir, "Upper.TOC", "## Version: 1.0\n")

	folders, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 1 || folders[0].Name != "Upper" {
		t.Fatalf("got %+v, want one folder Upper", folders)
	}
}
