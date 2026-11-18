package main

import (
	"math"
	"os"
	"slices"
	"strings"
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/index"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestStripUIEscapes(t *testing.T) {
	cases := []struct{ in, want string }{
		{"WeakAuras", "WeakAuras"},
		{"|cff9482c9Weak|r|cFFFFFFFFAuras|r", "WeakAuras"},
		{"|cnIQ4:Epic Addon|r", "Epic Addon"},
		{`|TInterface\Icons\INV_Misc_QuestionMark:16|t Boss Mods`, "Boss Mods"},
		{`|A:questlog-questtypeicon-heroic:16:16|a Quests`, "Quests"},
		{"|Hitem:19019|h[Thunderfury]|h", "[Thunderfury]"},
		{"Line|nBreak", "Line Break"},
		{"Pipes || kept", "Pipes | kept"},
		{"  padded   name  ", "padded name"},
		{"tab\there\x01", "tab here"},
		{"|cff00ff00|r", ""},
	}
	for _, tc := range cases {
		if got := stripUIEscapes(tc.in); got != tc.want {
			t.Errorf("stripUIEscapes(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseTOC(t *testing.T) {
	data := []byte("\xef\xbb\xbf## Interface: 110200, 50503\r\n## Title: |cff9482c9Weak|rAuras\r\n##X-Curse-Project-ID:65387\n# comment\nWeakAuras.lua\n## Title-deDE: Schwach\n")
	fields := parseTOC(data)
	if fields["interface"] != "110200, 50503" || fields["title"] != "|cff9482c9Weak|rAuras" {
		t.Fatalf("fields = %v", fields)
	}
	if fields["x-curse-project-id"] != "65387" {
		t.Fatalf("fields = %v", fields)
	}
	if got := tocInterfaces(fields); !slices.Equal(got, []int{110200, 50503}) {
		t.Fatalf("interfaces = %v", got)
	}
	if got := tocInterfaces(map[string]string{"interface": "abc, 0, 30402"}); !slices.Equal(got, []int{30402}) {
		t.Fatalf("interfaces = %v", got)
	}
}

func TestTocMatchesFolderStripsClientSuffixes(t *testing.T) {
	folders := map[string]string{"questie": "Questie", "myaddon": "MyAddon", "weakauras": "WeakAuras"}
	cases := []struct {
		path   string
		want   string
		wantOK bool
	}{
		{"Questie.toc", "Questie", true},
		{"Questie_Forever.toc", "Questie", true},
		{"Questie/Questie-Forever.toc", "Questie", true},
		{"deep/path/MyAddon_Camelot.TOC", "MyAddon", true},
		{"WeakAuras/WeakAuras_Mainline.toc", "WeakAuras", true},
		{"weakauras_wrath.toc", "WeakAuras", true},
		{"Questie-335.toc", "", false},
		{"Questie_Mainline.enUS.toc", "", false},
		{"Other.toc", "", false},
		{"Questie", "", false},
	}
	for _, tc := range cases {
		got, ok := tocMatchesFolder(tc.path, folders)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("tocMatchesFolder(%q) = %q, %v; want %q, %v", tc.path, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestParsePkgmeta(t *testing.T) {
	meta := parsePkgmeta(readTestdata(t, "pkgmeta_weakauras.txt"))
	if meta.PackageAs != "WeakAuras" {
		t.Errorf("package-as = %q", meta.PackageAs)
	}
	want := []string{"WeakAuras", "WeakAurasOptions", "WeakAurasModelPaths", "WeakAurasTemplates", "WeakAurasArchive"}
	if !slices.Equal(meta.MoveFolders, want) {
		t.Errorf("move folders = %v, want %v", meta.MoveFolders, want)
	}
	if meta.SkippedMoves != 1 {
		t.Errorf("skipped moves = %d, want 1 (the nested library path)", meta.SkippedMoves)
	}

	odd := parsePkgmeta([]byte("package-as: \"Quoted\"  # note\nmove-folders:\n  # comment\n  'A/B': \"B\"\n  A/C: C\nother:\n  X/Y: Y\n"))
	if odd.PackageAs != "Quoted" || !slices.Equal(odd.MoveFolders, []string{"B", "C"}) {
		t.Errorf("odd = %+v", odd)
	}
	if empty := parsePkgmeta([]byte("")); empty.PackageAs != "" || len(empty.MoveFolders) != 0 {
		t.Errorf("empty = %+v", empty)
	}
}

func TestParseFunding(t *testing.T) {
	text := `# sponsors
github: [Alice, bob]
patreon: carol  # trailing comment
ko_fi: "dave"
custom: ["https://example.org/donate", 'https://example.net/x#frag']
liberapay:
open_collective: ~
buy_me_a_coffee: []
tidelift: npm/some-package
community_bridge:
  - nested-list-item
unknown_platform: someone
github: not a handle!
custom: ftp://example.org/pay
`
	got := parseFunding(text)
	want := []index.Funding{
		{Platform: "github", Handle: "Alice"},
		{Platform: "github", Handle: "bob"},
		{Platform: "patreon", Handle: "carol"},
		{Platform: "ko_fi", Handle: "dave"},
		{Platform: "custom", Handle: "https://example.org/donate"},
		{Platform: "custom", Handle: "https://example.net/x#frag"},
		{Platform: "tidelift", Handle: "npm/some-package"},
	}
	if !slices.Equal(got.Links, want) {
		t.Errorf("links = %+v\nwant   %+v", got.Links, want)
	}
	if len(got.Skipped) != 4 {
		t.Fatalf("skipped = %q, want 4 entries", got.Skipped)
	}
	for _, needle := range []string{"nested-list-item", "unknown_platform", "not a handle", "ftp://"} {
		found := false
		for _, line := range got.Skipped {
			found = found || strings.Contains(line, needle)
		}
		if !found {
			t.Errorf("no skip report mentions %q: %q", needle, got.Skipped)
		}
	}
	for _, link := range got.Links {
		if _, err := link.URL(); err != nil {
			t.Errorf("%+v does not validate: %v", link, err)
		}
	}
}

func TestParseFundingDeduplicatesAndHandlesEmpty(t *testing.T) {
	got := parseFunding("github: alice\ngithub: [alice]\n")
	if len(got.Links) != 1 || len(got.Skipped) != 0 {
		t.Fatalf("got %+v", got)
	}
	if got := parseFunding(""); len(got.Links) != 0 || len(got.Skipped) != 0 {
		t.Fatalf("empty = %+v", got)
	}
}

func TestReleaseJSONGameTypes(t *testing.T) {
	got, ok := releaseJSONGameTypes(readTestdata(t, "release_json_weakauras.json"))
	want := []wow.GameType{wow.GameMainline, wow.GameVanilla, wow.GameTitan, wow.GameCata, wow.GameMists}
	if !ok || !slices.Equal(got, want) {
		t.Fatalf("got %v, %v; want %v", got, ok, want)
	}
	for _, bad := range []string{`not json`, `{"releases":[]}`, `{"releases":[{"metadata":[{"flavor":"nope"}]}]}`} {
		if got, ok := releaseJSONGameTypes([]byte(bad)); ok || got != nil {
			t.Errorf("releaseJSONGameTypes(%q) = %v, %v", bad, got, ok)
		}
	}
}

func TestSimilarityRatio(t *testing.T) {
	if got := ratio("abcd", "abce"); math.Abs(got-0.75) > 1e-9 {
		t.Errorf("ratio = %v, want 0.75", got)
	}
	if ratio("", "") != 1 || ratio("a", "") != 0 {
		t.Error("empty strings")
	}
	if !nameSimilar("WeakAuras", "WeakAuras/WeakAuras2") || !nameSimilar("Pawn", "VgerMods/Pawn") {
		t.Error("expected similar names")
	}
	if nameSimilar("Bagnon", "tukui/ElvUI") {
		t.Error("unrelated names reported similar")
	}
}
