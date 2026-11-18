package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/index"
	"somegit.dev/vikingowl/wowtui/internal/registry"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

func mapIndexEntries() []index.Entry {
	return []index.Entry{
		{
			ID:        "questie",
			Name:      "Questie",
			Source:    "gitlab:group/questie",
			Folders:   []string{"Questie_Data"},
			GameTypes: []wow.GameType{wow.GameVanilla},
			IDs:       index.ProviderIDs{CurseForge: "12345"},
			Reviewed:  true,
		},
		{
			ID:        "weakauras",
			Name:      "WeakAuras",
			Source:    "https://github.com/WeakAuras/WeakAuras2",
			Folders:   []string{"WeakAuras", "WeakAurasOptions"},
			GameTypes: []wow.GameType{wow.GameMainline},
		},
	}
}

// brokenIndexConfig returns an index config whose signature does not verify
// and whose content would also fail validation in two entries.
func brokenIndexConfig(t *testing.T) string {
	t.Helper()
	entries := mapIndexEntries()
	entries[0].Source = "direct:https://example.com/a.zip"
	entries[1].Source = "direct:https://example.com/b.zip"
	path, _ := writeSignedIndex(t, entries)
	otherPub, _, err := index.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return indexConfig(path, index.EncodePublicKey(otherPub))
}

func invalidContentIndexConfig(t *testing.T) string {
	t.Helper()
	entries := mapIndexEntries()
	entries[0].Source = "direct:https://example.com/a.zip"
	entries[1].Source = "direct:https://example.com/b.zip"
	return indexConfig(writeSignedIndex(t, entries))
}

func assertOneWarning(t *testing.T, stderr string) {
	t.Helper()
	lines := strings.Split(strings.TrimRight(stderr, "\n"), "\n")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "warning: index not loaded, continuing without it: ") {
		t.Errorf("stderr = %q, want one index warning line", stderr)
	}
}

func TestMapWithIndexAddsIndexCandidates(t *testing.T) {
	path, key := writeSignedIndex(t, mapIndexEntries())
	cfgPath, regPath, installPath := setupMapEnv(t, indexConfig(path, key))

	out := runCmd(t, "map", "--config", cfgPath, "--registry", regPath)

	want := [][]string{
		rowHub,
		{"Questie", "retail", "gitlab", "active", "gitlab:group/questie", "index:curseforge"},
		rowCurse,
		rowWowi,
		rowGit,
		{"WeakAuras", "retail", "github", "active", "github:WeakAuras/WeakAuras2", "index:folder,unreviewed,x-website"},
		rowWago,
	}
	if got := mapRows(t, out, installPath); !reflect.DeepEqual(got, want) {
		t.Errorf("rows =\n%q\nwant\n%q", got, want)
	}
}

func TestMapWithIndexThatFailsToLoadWarnsAndContinues(t *testing.T) {
	for name, cfg := range map[string]func(*testing.T) string{
		"bad signature":   brokenIndexConfig,
		"invalid content": invalidContentIndexConfig,
		"missing file": func(t *testing.T) string {
			_, key := writeSignedIndex(t, mapIndexEntries())
			return indexConfig(filepath.Join(t.TempDir(), "missing.json"), key)
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfgPath, regPath, installPath := setupMapEnv(t, cfg(t))

			out, errOut, err := runCmdSplit("map", "--config", cfgPath, "--registry", regPath)
			if err != nil {
				t.Fatalf("map: %v\nstderr: %s", err, errOut)
			}
			want := [][]string{rowHub, rowCurse, rowWowi, rowGit, rowGitHub, rowWago}
			if got := mapRows(t, out, installPath); !reflect.DeepEqual(got, want) {
				t.Errorf("rows =\n%q\nwant\n%q", got, want)
			}
			assertOneWarning(t, errOut)
		})
	}
}

func testAddonIndexConfig(t *testing.T, folders ...string) string {
	t.Helper()
	if folders == nil {
		folders = []string{"TestAddon"}
	}
	return indexConfig(writeSignedIndex(t, []index.Entry{{
		ID:        "testaddon",
		Name:      "Test Addon",
		Source:    "github:Owner/TestAddon",
		Folders:   folders,
		GameTypes: []wow.GameType{wow.GameMainline},
		Reviewed:  true,
	}}))
}

func appendConfig(t *testing.T, cfgPath, content string) {
	t.Helper()
	f, err := os.OpenFile(cfgPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString("\n" + content); err != nil {
		t.Fatal(err)
	}
}

func TestAddIndexReference(t *testing.T) {
	gh := newFakeGitHub(t, buildCmdZip(t, "1.0.0"))
	cfgPath, installPath := setupAddEnv(t, gh)
	appendConfig(t, cfgPath, testAddonIndexConfig(t))

	out := runCmd(t, "add", "index:testaddon", "--config", cfgPath)
	if !strings.Contains(out, "index:testaddon is github:Owner/TestAddon") || !strings.Contains(out, "installed") {
		t.Fatalf("add output: %s", out)
	}
	assertTocFile(t, filepath.Join(installPath, "Interface", "AddOns", "TestAddon", "TestAddon.toc"), "## Version: 1.0.0")
	reg, err := registry.Load(registry.DefaultPath())
	if err != nil {
		t.Fatal(err)
	}
	if addon := reg.FindAddon("Owner/TestAddon", installPath); addon == nil || addon.Provider != "github" {
		t.Errorf("addons = %+v, want github Owner/TestAddon", reg.Addons)
	}
}

func TestAddIndexReferenceFails(t *testing.T) {
	cases := []struct {
		name, ref string
		cfg       func(*testing.T) string
		want      []string
	}{
		{"no index configured", "index:testaddon", func(*testing.T) string { return "" }, []string{"no index configured", "wowtui index use"}},
		{"index fails to load", "index:testaddon", brokenIndexConfig, []string{"index not loaded", index.ErrInvalidSignature.Error()}},
		{"unknown id", "index:missing", func(t *testing.T) string { return testAddonIndexConfig(t) }, []string{`index has no entry "missing"`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gh := newFakeGitHub(t, buildCmdZip(t, "1.0.0"))
			cfgPath, installPath := setupAddEnv(t, gh)
			appendConfig(t, cfgPath, tc.cfg(t))

			out, errOut, err := runCmdSplit("add", tc.ref, "--config", cfgPath)
			if err == nil {
				t.Fatalf("expected an error\noutput: %s", out)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			if strings.Contains(errOut, "warning:") {
				t.Errorf("a required index must fail, not warn:\n%s", errOut)
			}
			if _, err := os.Stat(filepath.Join(installPath, "Interface", "AddOns", "TestAddon")); !os.IsNotExist(err) {
				t.Errorf("addon was installed: %v", err)
			}
		})
	}
}

func TestAddWithIndexThatFailsToLoadWarnsForOtherReferences(t *testing.T) {
	gh := newFakeGitHub(t, buildCmdZip(t, "1.0.0"))
	cfgPath, _ := setupAddEnv(t, gh)
	appendConfig(t, cfgPath, brokenIndexConfig(t))

	out, errOut, err := runCmdSplit("add", "github:Owner/TestAddon", "--config", cfgPath)
	if err != nil {
		t.Fatalf("add: %v\nstderr: %s", err, errOut)
	}
	if !strings.Contains(out, "installed") {
		t.Errorf("add output: %s", out)
	}
	assertOneWarning(t, errOut)
}

func seedAdoptRegistry(t *testing.T, installPath string) {
	t.Helper()
	reg := &registry.Registry{Version: 1, Installations: []registry.Installation{{
		Path:   installPath,
		Flavor: "retail",
		Folders: []registry.Folder{
			{Name: "TestAddon", Version: "1.0.0"},
			{Name: "TestAddon_Options", Version: "1.0.0"},
			{Name: "Unrelated", Version: "2.0"},
		},
	}}}
	if err := reg.Save(registry.DefaultPath()); err != nil {
		t.Fatal(err)
	}
}

func TestAdoptIndexReferenceAdoptsEntryFolders(t *testing.T) {
	gh := newFakeGitHub(t, buildCmdZip(t, "1.0.0"))
	cfgPath, installPath := setupAddEnv(t, gh)
	appendConfig(t, cfgPath, testAddonIndexConfig(t, "TestAddon", "TestAddon_Options", "TestAddon_Missing"))
	seedAdoptRegistry(t, installPath)

	out := runCmd(t, "adopt", installPath, "TestAddon_Options", "index:testaddon", "--config", cfgPath)
	if strings.TrimSpace(out) != "adopted TestAddon_Options from index:testaddon" {
		t.Errorf("output = %q", out)
	}
	reg, err := registry.Load(registry.DefaultPath())
	if err != nil {
		t.Fatal(err)
	}
	addon := reg.FindAddon("Owner/TestAddon", installPath)
	if addon == nil {
		t.Fatalf("addons = %+v", reg.Addons)
	}
	if want := []string{"TestAddon", "TestAddon_Options"}; addon.Provider != "github" || !reflect.DeepEqual(addon.Folders, want) {
		t.Errorf("addon = %+v, want github with folders %q", addon, want)
	}
}

func TestAdoptIndexReferenceWithoutIndexFails(t *testing.T) {
	gh := newFakeGitHub(t, buildCmdZip(t, "1.0.0"))
	cfgPath, installPath := setupAddEnv(t, gh)
	seedAdoptRegistry(t, installPath)
	before, err := os.ReadFile(registry.DefaultPath())
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = runCmdSplit("adopt", installPath, "TestAddon", "index:testaddon", "--config", cfgPath)
	if err == nil || !strings.Contains(err.Error(), "no index configured") {
		t.Fatalf("error = %v, want no index configured", err)
	}
	after, err := os.ReadFile(registry.DefaultPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("failed adopt changed the registry:\n%s", after)
	}
}

func TestAdoptWithIndexThatFailsToLoadWarnsForOtherReferences(t *testing.T) {
	gh := newFakeGitHub(t, buildCmdZip(t, "1.0.0"))
	cfgPath, installPath := setupAddEnv(t, gh)
	appendConfig(t, cfgPath, brokenIndexConfig(t))
	seedAdoptRegistry(t, installPath)

	out, errOut, err := runCmdSplit("adopt", installPath, "TestAddon", "github:Owner/TestAddon", "--config", cfgPath)
	if err != nil {
		t.Fatalf("adopt: %v\nstderr: %s", err, errOut)
	}
	if strings.TrimSpace(out) != "adopted TestAddon from github:Owner/TestAddon" {
		t.Errorf("output = %q", out)
	}
	assertOneWarning(t, errOut)
}
