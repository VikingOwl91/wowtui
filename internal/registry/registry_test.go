package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func sampleRegistry() *Registry {
	return &Registry{
		Version: 1,
		Installations: []Installation{
			{
				Path:   "/games/wow/_retail_",
				Flavor: "retail",
				Folders: []Folder{
					{
						Name:       "WeakAuras",
						Version:    "5.18.1",
						Interfaces: []int{110200},
						TocFiles:   []string{"WeakAuras.toc"},
					},
				},
			},
		},
	}
}

func TestLoadMissingFileReturnsEmptyRegistry(t *testing.T) {
	reg, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatal(err)
	}
	if reg == nil || reg.Version != 0 || len(reg.Installations) != 0 {
		t.Errorf("expected empty registry, got %+v", reg)
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	want := sampleRegistry()

	if err := want.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != want.Version || len(got.Installations) != 1 {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	folder := got.Installations[0].Folders[0]
	if folder.Name != "WeakAuras" || folder.Version != "5.18.1" || folder.Interfaces[0] != 110200 {
		t.Errorf("folder roundtrip mismatch: %+v", folder)
	}
}

func TestSaveOverwritesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	if err := sampleRegistry().Save(path); err != nil {
		t.Fatal(err)
	}

	reg := &Registry{Version: 1}
	if err := reg.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Installations) != 0 {
		t.Errorf("expected overwritten registry, got %+v", got)
	}
}

func TestSaveCreatesDirAndRestrictsPerms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "registry.json")
	if err := sampleRegistry().Save(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file perms = %o, want 600", perm)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestDefaultPathUsesXDGDataHome(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/custom/data")
	if got := DefaultPath(); got != "/custom/data/wowtui/registry.json" {
		t.Errorf("DefaultPath = %q", got)
	}
}

func TestDefaultPathFallsBackToHome(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "/tmp/fakehome")
	if got := DefaultPath(); got != "/tmp/fakehome/.local/share/wowtui/registry.json" {
		t.Errorf("DefaultPath = %q", got)
	}
}
