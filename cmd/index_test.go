package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/config"
	"somegit.dev/vikingowl/wowtui/internal/index"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

func fixtureEntries() []index.Entry {
	return []index.Entry{
		{
			ID:          "bossmods",
			Name:        "Boss Mods",
			Source:      "github:ExampleOrg/BossMods",
			Folders:     []string{"BossMods", "BossMods_Core", "BossMods_Options"},
			GameTypes:   []wow.GameType{wow.GameMainline, wow.GameMists, wow.GameVanilla},
			IDs:         index.ProviderIDs{CurseForge: "12345", Wago: "aB3dE5fG", WoWInterface: "6789"},
			ReleaseJSON: true,
			Funding:     []index.Funding{{Platform: "github", Handle: "exampleorg"}, {Platform: "ko_fi", Handle: "example"}},
			Reviewed:    true,
		},
		{
			ID:        "bossmods-classic-extras",
			Name:      "Boss Mods Classic Extras",
			Source:    "gitlab:example-group/addons/bossmods-extras",
			Folders:   []string{"BossMods_Extras", "bossmods_core"},
			GameTypes: []wow.GameType{wow.GameVanilla, wow.GameTBC},
			Reviewed:  true,
		},
		{
			ID:        "questhelper",
			Name:      "Quest Helper",
			Source:    "https://github.com/ExampleDev/QuestHelper",
			Folders:   []string{"QuestHelper"},
			GameTypes: []wow.GameType{wow.GameMainline},
			IDs:       index.ProviderIDs{CurseForge: "555"},
			Funding:   []index.Funding{{Platform: "custom", Handle: "https://example.com/donate"}},
		},
	}
}

func indexJSON(t *testing.T, entries []index.Entry) []byte {
	t.Helper()
	data, err := json.Marshal(index.File{
		Format:      index.FormatVersion,
		GeneratedAt: time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC),
		Entries:     entries,
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// writeSignedIndex writes an index and its signature into a temp dir and
// returns the index path and the encoded public key. The private key never
// leaves memory.
func writeSignedIndex(t *testing.T, entries []index.Entry) (path, publicKey string) {
	t.Helper()
	pub, priv, err := index.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	data := indexJSON(t, entries)
	path = filepath.Join(t.TempDir(), "index.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".sig", index.Sign(data, priv), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, index.EncodePublicKey(pub)
}

func fingerprintOf(t *testing.T, publicKey string) string {
	t.Helper()
	pub, err := index.DecodePublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(pub)
	return "sha256:" + hex.EncodeToString(sum[:])[:16]
}

func indexConfig(path, publicKey string) string {
	return "[index]\npath = " + quoteTOML(path) + "\npublic_key = " + quoteTOML(publicKey) + "\n"
}

func quoteTOML(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

func setupIndexEnv(t *testing.T, cfgContent string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("WOWTUI_GITHUB_TOKEN", "")
	t.Setenv("WOWTUI_WAGO_API_KEY", "")
	t.Setenv("WOWTUI_CURSEFORGE_API_KEY", "")
	cfgPath := filepath.Join(home, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfgPath
}

func runCmdSplit(args ...string) (stdout, stderr string, err error) {
	root := NewRootCmd()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs(args)
	err = root.Execute()
	return out.String(), errOut.String(), err
}

var columnSplit = regexp.MustCompile(`\s{2,}`)

func keyValues(out string) map[string]string {
	values := map[string]string{}
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		parts := columnSplit.Split(strings.TrimSpace(line), 2)
		if len(parts) == 2 {
			values[parts[0]] = parts[1]
		}
	}
	return values
}

func TestIndexStatusWithoutIndex(t *testing.T) {
	cfgPath := setupIndexEnv(t, "")

	out := runCmd(t, "index", "status", "--config", cfgPath)
	if !strings.Contains(out, "no index configured") || !strings.Contains(out, "wowtui index use") {
		t.Errorf("output = %q", out)
	}
}

func TestIndexStatusLoaded(t *testing.T) {
	path, key := writeSignedIndex(t, fixtureEntries())
	cfgPath := setupIndexEnv(t, indexConfig(path, key))

	out := runCmd(t, "index", "status", "--config", cfgPath)
	got := keyValues(out)
	want := map[string]string{
		"path":       path,
		"signature":  path + ".sig",
		"public key": fingerprintOf(t, key),
		"status":     "loaded",
		"entries":    "3",
		"generated":  "2026-11-18T10:00:00Z",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q\noutput:\n%s", k, got[k], v, out)
		}
	}
	if strings.Contains(out, key) {
		t.Errorf("status prints the full public key instead of a fingerprint:\n%s", out)
	}
}

func TestIndexStatusReportsLoadErrors(t *testing.T) {
	path, key := writeSignedIndex(t, fixtureEntries())
	otherPub, _, err := index.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	invalid := fixtureEntries()
	invalid[0].Source = "direct:https://example.com/addon.zip"
	invalidPath, invalidKey := writeSignedIndex(t, invalid)
	noSigPath, noSigKey := writeSignedIndex(t, fixtureEntries())
	if err := os.Remove(noSigPath + ".sig"); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, path, key, want string
	}{
		{"missing file", filepath.Join(t.TempDir(), "missing.json"), key, "no such file"},
		{"missing signature", noSigPath, noSigKey, "index signature"},
		{"wrong key", path, index.EncodePublicKey(otherPub), index.ErrInvalidSignature.Error()},
		{"invalid content", invalidPath, invalidKey, "want github or gitlab"},
		{"invalid public key", path, "not-a-key", "public key"},
		{"missing public key", path, "", "index.public_key is not set"},
		{"missing path", "", key, "index.path is not set"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfgPath := setupIndexEnv(t, indexConfig(tc.path, tc.key))

			out, errOut, err := runCmdSplit("index", "status", "--config", cfgPath)
			if err == nil {
				t.Fatalf("expected an error\noutput:\n%s", out)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err, tc.want)
			}
			if !strings.Contains(errOut, err.Error()) {
				t.Errorf("stderr does not show the load error:\n%s", errOut)
			}
			got := keyValues(out)
			if got["status"] != "not loaded" {
				t.Errorf("status = %q\noutput:\n%s", got["status"], out)
			}
			if tc.path != "" && got["path"] != tc.path {
				t.Errorf("path = %q, want %q", got["path"], tc.path)
			}
		})
	}
}

func searchRows(t *testing.T, out string) [][]string {
	t.Helper()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	header := []string{"ID", "NAME", "SOURCE", "FOLDERS", "GAME TYPES", "REVIEWED", "FUNDING"}
	if got := columnSplit.Split(strings.TrimSpace(lines[0]), -1); !reflect.DeepEqual(got, header) {
		t.Fatalf("header = %q\noutput:\n%s", lines[0], out)
	}
	var rows [][]string
	for _, line := range lines[1:] {
		fields := columnSplit.Split(strings.TrimSpace(line), -1)
		if len(fields) != len(header) {
			t.Fatalf("row %q has %d columns\noutput:\n%s", line, len(fields), out)
		}
		rows = append(rows, fields)
	}
	return rows
}

func TestIndexSearch(t *testing.T) {
	path, key := writeSignedIndex(t, fixtureEntries())
	cfgPath := setupIndexEnv(t, indexConfig(path, key))

	cases := []struct {
		query string
		want  [][]string
	}{
		{"boss", [][]string{
			{"bossmods", "Boss Mods", "github:ExampleOrg/BossMods", "BossMods,BossMods_Core,BossMods_Options", "mainline,mists,vanilla", "yes", "https://github.com/sponsors/exampleorg,https://ko-fi.com/example"},
			{"bossmods-classic-extras", "Boss Mods Classic Extras", "gitlab:example-group/addons/bossmods-extras", "BossMods_Extras,bossmods_core", "vanilla,tbc", "yes", "-"},
		}},
		{"QUEST", [][]string{
			{"questhelper", "Quest Helper", "github:ExampleDev/QuestHelper", "QuestHelper", "mainline", "no", "https://example.com/donate"},
		}},
	}
	for _, tc := range cases {
		out := runCmd(t, "index", "search", tc.query, "--config", cfgPath)
		if got := searchRows(t, out); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("search %q rows =\n%q\nwant\n%q", tc.query, got, tc.want)
		}
	}
}

func TestIndexSearchNoMatch(t *testing.T) {
	path, key := writeSignedIndex(t, fixtureEntries())
	cfgPath := setupIndexEnv(t, indexConfig(path, key))

	out := runCmd(t, "index", "search", "nothing-like-this", "--config", cfgPath)
	if strings.TrimSpace(out) != `no index entries match "nothing-like-this"` {
		t.Errorf("output = %q", out)
	}
}

func TestIndexSearchFailsWithoutLoadedIndex(t *testing.T) {
	path, _ := writeSignedIndex(t, fixtureEntries())
	otherPub, _, err := index.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, cfg, want string }{
		{"not configured", "", "no index configured"},
		{"bad signature", indexConfig(path, index.EncodePublicKey(otherPub)), index.ErrInvalidSignature.Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfgPath := setupIndexEnv(t, tc.cfg)
			out, _, err := runCmdSplit("index", "search", "boss", "--config", cfgPath)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want one containing %q", err, tc.want)
			}
			if strings.Contains(out, "bossmods") {
				t.Errorf("search printed entries of an index it could not load:\n%s", out)
			}
		})
	}
}

func TestIndexUseSavesVerifiedIndex(t *testing.T) {
	path, key := writeSignedIndex(t, fixtureEntries())
	cfgPath := setupIndexEnv(t, "[wow]\npaths = [\"/games/wow\"]\n")
	t.Setenv("WOWTUI_GITHUB_TOKEN", "dummy-env-token")

	t.Chdir(filepath.Dir(path))
	out := runCmd(t, "index", "use", "--path", "index.json", "--public-key", key, "--config", cfgPath)
	if !strings.Contains(out, path) || !strings.Contains(out, "3 entries") || !strings.Contains(out, fingerprintOf(t, key)) {
		t.Errorf("output = %q", out)
	}

	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "dummy-env-token") {
		t.Errorf("config file contains the env credential:\n%s", raw)
	}
	t.Setenv("WOWTUI_GITHUB_TOKEN", "")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Index != (config.Index{Path: path, PublicKey: key}) {
		t.Errorf("index = %+v, want absolute path %q and key", cfg.Index, path)
	}
	if len(cfg.WoW.Paths) != 1 || cfg.WoW.Paths[0] != "/games/wow" {
		t.Errorf("wow.paths = %v, want kept", cfg.WoW.Paths)
	}

	if got := keyValues(runCmd(t, "index", "status", "--config", cfgPath))["status"]; got != "loaded" {
		t.Errorf("status after use = %q", got)
	}
}

func TestIndexUseRejectsIndexThatDoesNotLoad(t *testing.T) {
	path, _ := writeSignedIndex(t, fixtureEntries())
	otherPub, _, err := index.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	const original = "[wow]\npaths = [\"/games/wow\"]\n"
	cases := []struct{ name, path, key, want string }{
		{"wrong key", path, index.EncodePublicKey(otherPub), index.ErrInvalidSignature.Error()},
		{"missing file", filepath.Join(t.TempDir(), "missing.json"), index.EncodePublicKey(otherPub), "no such file"},
		{"invalid key", path, "AAAA", "public key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfgPath := setupIndexEnv(t, original)
			_, _, err := runCmdSplit("index", "use", "--path", tc.path, "--public-key", tc.key, "--config", cfgPath)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want one containing %q", err, tc.want)
			}
			raw, err := os.ReadFile(cfgPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != original {
				t.Errorf("rejected index changed the config:\n%s", raw)
			}
		})
	}
}

func TestIndexUseRequiresBothFlags(t *testing.T) {
	path, key := writeSignedIndex(t, fixtureEntries())
	for _, args := range [][]string{
		{"index", "use", "--path", path},
		{"index", "use", "--public-key", key},
	} {
		cfgPath := setupIndexEnv(t, "")
		if _, _, err := runCmdSplit(append(args, "--config", cfgPath)...); err == nil {
			t.Errorf("%v: expected an error", args)
		}
	}
}
