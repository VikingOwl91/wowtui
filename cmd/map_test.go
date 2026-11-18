package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/registry"
)

func setupMapEnv(t *testing.T, cfgContent string) (cfgPath, regPath, installPath string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("WOWTUI_CURSEFORGE_API_KEY", "")

	installPath = filepath.Join(home, "wow", "_retail_")
	folder := func(name string, hints map[string]string) registry.Folder {
		return registry.Folder{Name: name, Version: "1.0", Hints: hints}
	}
	reg := &registry.Registry{Version: 1, Installations: []registry.Installation{{
		Path:   installPath,
		Flavor: "retail",
		Folders: []registry.Folder{
			folder("WeakAuras", map[string]string{"x-website": "https://github.com/WeakAuras/WeakAuras2", "x-wago-id": "abc"}),
			folder("SelfHosted", map[string]string{"x-source-repo": "https://git.example.org/team/addon.git"}),
			folder("Questie", map[string]string{"x-curse-project-id": "12345", "x-wowi-id": "6789"}),
			folder("Hub", map[string]string{"x-website": "https://wowup.io/addons/123"}),
			folder("RaiderIO", map[string]string{"x-website": "https://raider.io"}),
			folder("RaiderIO_DB_EU_M", map[string]string{"x-website": "https://raider.io"}),
			folder("ForeverDB", map[string]string{"x-website": "https://foreverdb.net"}),
			folder("ForeverDBHelper", map[string]string{"x-website": "https://foreverdb.net/foreverdb-helper"}),
			folder("TidyPlates_ThreatPlates", map[string]string{"x-website": "https://wow.curseforge.com/projects/tidy-plates-threat-plates"}),
			folder("BigWigs", map[string]string{"x-website": "https://github.com/BigWigsMods"}),
			folder("WowUpCompanion", map[string]string{"x-source-repo": "https://wowup.io"}),
		},
	}}}
	regPath = filepath.Join(home, "registry.json")
	if err := reg.Save(regPath); err != nil {
		t.Fatal(err)
	}
	cfgPath = filepath.Join(home, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfgPath, regPath, installPath
}

func mapRows(t *testing.T, out, installPath string) [][]string {
	t.Helper()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if got := strings.Fields(lines[0]); !reflect.DeepEqual(got, []string{"FOLDER", "FLAVOR", "PROVIDER", "AVAILABILITY", "REFERENCE", "REASON", "INSTALL"}) {
		t.Fatalf("header = %q\noutput:\n%s", lines[0], out)
	}
	var rows [][]string
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) != 7 || fields[6] != installPath {
			t.Fatalf("unexpected row %q\noutput:\n%s", line, out)
		}
		rows = append(rows, fields[:6])
	}
	return rows
}

var (
	rowHub        = []string{"Hub", "retail", "wowuphub", "paused", "123", "x-website"}
	rowCurse      = []string{"Questie", "retail", "curseforge", "needs-api-key", "curseforge:12345", "x-curse-project-id"}
	rowWowi       = []string{"Questie", "retail", "wowinterface", "unsupported", "wowinterface:6789", "x-wowi-id"}
	rowGit        = []string{"SelfHosted", "retail", "git", "active", "https://git.example.org/team/addon.git", "x-source-repo"}
	rowGitHub     = []string{"WeakAuras", "retail", "github", "active", "WeakAuras/WeakAuras2", "x-website"}
	rowGitHubOff  = []string{"WeakAuras", "retail", "github", "disabled", "WeakAuras/WeakAuras2", "x-website"}
	rowWago       = []string{"WeakAuras", "retail", "wago", "paused", "wago:abc", "x-wago-id"}
	junkMapValues = []string{"raider.io", "foreverdb.net", "curseforge.com", "BigWigsMods", "wowup.io", "direct"}
)

func assertNoJunk(t *testing.T, out string) {
	t.Helper()
	for _, junk := range junkMapValues {
		if strings.Contains(out, junk) {
			t.Errorf("map output contains junk %q:\n%s", junk, out)
		}
	}
}

func TestMapShowsAvailability(t *testing.T) {
	cfgPath, regPath, installPath := setupMapEnv(t, "")

	out := runCmd(t, "map", "--config", cfgPath, "--registry", regPath)

	want := [][]string{rowHub, rowCurse, rowWowi, rowGit, rowGitHub, rowWago}
	if got := mapRows(t, out, installPath); !reflect.DeepEqual(got, want) {
		t.Errorf("rows =\n%q\nwant\n%q", got, want)
	}
	assertNoJunk(t, out)
}

func TestMapAdoptableShowsOnlyActiveProviders(t *testing.T) {
	cfgPath, regPath, installPath := setupMapEnv(t, "")

	out := runCmd(t, "map", "--adoptable", "--config", cfgPath, "--registry", regPath)

	want := [][]string{rowGit, rowGitHub}
	if got := mapRows(t, out, installPath); !reflect.DeepEqual(got, want) {
		t.Errorf("rows =\n%q\nwant\n%q", got, want)
	}
	assertNoJunk(t, out)
}

func TestMapMarksDisabledProvider(t *testing.T) {
	cfgPath, regPath, installPath := setupMapEnv(t, "[providers.github]\nenabled = false\n")

	out := runCmd(t, "map", "--config", cfgPath, "--registry", regPath)
	want := [][]string{rowHub, rowCurse, rowWowi, rowGit, rowGitHubOff, rowWago}
	if got := mapRows(t, out, installPath); !reflect.DeepEqual(got, want) {
		t.Errorf("rows =\n%q\nwant\n%q", got, want)
	}

	out = runCmd(t, "map", "--adoptable", "--config", cfgPath, "--registry", regPath)
	if got := mapRows(t, out, installPath); !reflect.DeepEqual(got, [][]string{rowGit}) {
		t.Errorf("adoptable rows = %q, want only the git row", got)
	}
}

func TestMapAdoptableWithoutActiveCandidates(t *testing.T) {
	cfgPath, regPath, _ := setupMapEnv(t, "[providers.github]\nenabled = false\n[providers.git]\nenabled = false\n")

	out := runCmd(t, "map", "--adoptable", "--config", cfgPath, "--registry", regPath)
	if strings.TrimSpace(out) != "no adoptable provider candidates found" {
		t.Errorf("output = %q", out)
	}
}
