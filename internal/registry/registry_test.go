package registry

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func sampleAddon() Addon {
	return Addon{
		Provider:    "github",
		Ref:         "Owner/Repo",
		Name:        "Owner/Repo",
		Flavor:      "retail",
		InstallPath: "/games/wow/_retail_",
		Folders:     []string{"Repo"},
		Version:     "v1.2.3",
		Source:      "auto",
		ReleaseID:   4242,
		AssetName:   "Repo-1.2.3.zip",
		SHA256:      "abc123",
		InstalledAt: time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC),
	}
}

func TestAddonRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	reg := &Registry{Version: 1, Addons: []Addon{sampleAddon()}}
	if err := reg.Save(path); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Addons) != 1 {
		t.Fatalf("got %d addons, want 1", len(got.Addons))
	}
	addon := got.Addons[0]
	if addon.Ref != "Owner/Repo" || addon.ReleaseID != 4242 || addon.Version != "v1.2.3" {
		t.Errorf("addon roundtrip mismatch: %+v", addon)
	}
	if addon.Source != "auto" {
		t.Errorf("source = %q, want auto", addon.Source)
	}
	if !addon.InstalledAt.Equal(time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("installed_at = %v", addon.InstalledAt)
	}
}

func TestFindAddon(t *testing.T) {
	reg := &Registry{Version: 1, Addons: []Addon{sampleAddon()}}

	if got := reg.FindAddon("Owner/Repo", "/games/wow/_retail_"); got == nil || got.Ref != "Owner/Repo" {
		t.Errorf("FindAddon = %+v", got)
	}
	if got := reg.FindAddon("Owner/Repo", "/other/path"); got != nil {
		t.Errorf("FindAddon should miss on different install path, got %+v", got)
	}
	if got := reg.FindAddon("Other/Repo", "/games/wow/_retail_"); got != nil {
		t.Errorf("FindAddon should miss on different ref, got %+v", got)
	}
}

func TestUpsertAddon(t *testing.T) {
	reg := &Registry{Version: 1}

	reg.UpsertAddon(sampleAddon())
	if len(reg.Addons) != 1 {
		t.Fatalf("got %d addons after insert, want 1", len(reg.Addons))
	}

	updated := sampleAddon()
	updated.Version = "v2.0.0"
	updated.ReleaseID = 9999
	reg.UpsertAddon(updated)
	if len(reg.Addons) != 1 {
		t.Fatalf("got %d addons after update, want 1", len(reg.Addons))
	}
	if reg.Addons[0].Version != "v2.0.0" || reg.Addons[0].ReleaseID != 9999 {
		t.Errorf("upsert did not update: %+v", reg.Addons[0])
	}
}
