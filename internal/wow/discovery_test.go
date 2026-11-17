package wow

import (
	"os"
	"path/filepath"
	"testing"
)

func makeFlavorDir(t *testing.T, root, name string, withInterface bool) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if withInterface {
		dir = filepath.Join(dir, "Interface")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if withInterface {
		return filepath.Dir(dir)
	}
	return dir
}

func TestDiscoverConfiguredFlavorDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	retail := makeFlavorDir(t, root, "_retail_", true)

	got, err := Discover([]string{retail})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d installations, want 1", len(got))
	}
	if got[0].Path != retail || got[0].Flavor != Retail {
		t.Errorf("got %+v, want path %q flavor %q", got[0], retail, Retail)
	}
}

func TestDiscoverConfiguredParentDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	makeFlavorDir(t, root, "_retail_", true)
	makeFlavorDir(t, root, "_classic_", true)

	got, err := Discover([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d installations, want 2", len(got))
	}
	if got[0].Flavor != Classic || got[1].Flavor != Retail {
		t.Errorf("expected sorted [classic retail], got %+v", got)
	}
}

func TestDiscoverConfiguredDirWithoutInterface(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	retail := makeFlavorDir(t, root, "_retail_", false)

	got, err := Discover([]string{retail})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d installations, want 1 (explicit config wins)", len(got))
	}
}

func TestDiscoverConfiguredMissingPathErrors(t *testing.T) {
	_, err := Discover([]string{filepath.Join(t.TempDir(), "nope")})
	if err == nil {
		t.Fatal("expected error for missing configured path")
	}
}

func TestDiscoverDefaultWinePrefix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	wowDir := filepath.Join(home, ".wine", "drive_c", "Program Files", "World of Warcraft")
	makeFlavorDir(t, wowDir, "_retail_", true)

	got, err := Discover(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Flavor != Retail {
		t.Fatalf("got %+v, want one retail installation", got)
	}
}

func TestDiscoverDefaultLutrisPrefix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	wowDir := filepath.Join(home, "Games", "world-of-warcraft", "drive_c", "Program Files", "World of Warcraft")
	makeFlavorDir(t, wowDir, "_classic_", true)

	got, err := Discover(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Flavor != Classic {
		t.Fatalf("got %+v, want one classic installation", got)
	}
}

func TestDiscoverDefaultLutrisProgramFilesX86(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	wowDir := filepath.Join(home, "Games", "battlenet-new", "drive_c", "Program Files (x86)", "World of Warcraft")
	makeFlavorDir(t, wowDir, "_retail_", true)

	got, err := Discover(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Flavor != Retail {
		t.Fatalf("got %+v, want one retail installation", got)
	}
}

func TestDiscoverDefaultSkipsEmptyFlavorDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	wowDir := filepath.Join(home, ".wine", "drive_c", "Program Files", "World of Warcraft")
	makeFlavorDir(t, wowDir, "_retail_", false)

	got, err := Discover(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v, want no installations without Interface dir", got)
	}
}

func TestDiscoverDedupesConfiguredAndDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	wowDir := filepath.Join(home, ".wine", "drive_c", "Program Files", "World of Warcraft")
	retail := makeFlavorDir(t, wowDir, "_retail_", true)

	got, err := Discover([]string{retail})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d installations, want dedup to 1", len(got))
	}
}

func assertEveryClientDir(t *testing.T, got []Installation, root string) {
	t.Helper()
	if len(got) != len(clientDirTests) {
		t.Fatalf("got %d installations, want %d: %+v", len(got), len(clientDirTests), got)
	}
	byFlavor := map[GameFlavor]string{}
	for _, inst := range got {
		byFlavor[inst.Flavor] = inst.Path
	}
	for _, tt := range clientDirTests {
		want := filepath.Join(root, tt.dir)
		if byFlavor[tt.flavor] != want {
			t.Errorf("flavor %q path = %q, want %q", tt.flavor, byFlavor[tt.flavor], want)
		}
	}
}

func TestDiscoverConfiguredParentFindsEveryClientDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	for _, tt := range clientDirTests {
		makeFlavorDir(t, root, tt.dir, true)
	}
	makeFlavorDir(t, root, "_cn_beta_", true)

	got, err := Discover([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	assertEveryClientDir(t, got, root)
}

func TestDiscoverDefaultFindsEveryClientDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	wowDir := filepath.Join(home, "Games", "battlenet", "drive_c", "Program Files (x86)", "World of Warcraft")
	for _, tt := range clientDirTests {
		makeFlavorDir(t, wowDir, tt.dir, true)
	}

	got, err := Discover(nil)
	if err != nil {
		t.Fatal(err)
	}
	assertEveryClientDir(t, got, wowDir)
}

func TestDiscoverConfiguredNewFlavorDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	eraPTR := makeFlavorDir(t, root, "_classic_era_ptr_", true)

	got, err := Discover([]string{eraPTR})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != eraPTR || got[0].Flavor != ClassicEraPTR {
		t.Fatalf("got %+v, want one classic-era-ptr installation at %q", got, eraPTR)
	}
}
