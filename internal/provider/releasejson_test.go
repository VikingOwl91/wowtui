package provider

import (
	"strings"
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/wow"
)

func TestReleaseJSONFlavorGameTypes(t *testing.T) {
	want := map[string]wow.GameType{
		"mainline": wow.GameMainline,
		"classic":  wow.GameVanilla,
		"bcc":      wow.GameTBC,
		"wrath":    wow.GameWrath,
		"titan":    wow.GameTitan,
		"cata":     wow.GameCata,
		"mists":    wow.GameMists,
		"forever":  wow.GameForever,
	}
	for flavor, gameType := range want {
		data := `{"releases":[{"filename":"Addon.zip","nolib":false,"metadata":[{"flavor":"` + flavor + `","interface":1}]}]}`
		name, ok, err := pickReleaseJSONAsset([]byte(data), []string{"Addon.zip"}, gameType)
		if err != nil || !ok || name != "Addon.zip" {
			t.Errorf("flavor %q: got %q, %v, %v; want Addon.zip for %s", flavor, name, ok, err, gameType)
		}
	}
}

func TestReleaseJSONIgnoresUnknownFlavors(t *testing.T) {
	data := `{"releases":[{"filename":"Addon.zip","nolib":false,"metadata":[{"flavor":"retail","interface":1},{"flavor":"vanilla","interface":1}]}]}`
	name, ok, err := pickReleaseJSONAsset([]byte(data), []string{"Addon.zip"}, wow.GameMainline)
	if err != nil || ok || name != "" {
		t.Errorf("got %q, %v, %v; want a fallback for a release.json without known flavors", name, ok, err)
	}
}

func TestReleaseJSONAcceptsNolibWhenOnlyBuild(t *testing.T) {
	data := `{"releases":[{"filename":"Addon-nolib.zip","nolib":true,"metadata":[{"flavor":"mainline","interface":1}]}]}`
	name, ok, err := pickReleaseJSONAsset([]byte(data), []string{"Addon-nolib.zip"}, wow.GameMainline)
	if err != nil || !ok || name != "Addon-nolib.zip" {
		t.Errorf("got %q, %v, %v; want Addon-nolib.zip", name, ok, err)
	}
}

func TestReleaseJSONSingleZipForManyFlavors(t *testing.T) {
	data := `{"releases":[{"name":"BigWigs","version":"v1","filename":"BigWigs-v1.zip","nolib":false,"metadata":[` +
		`{"flavor":"mists","interface":50503},{"flavor":"titan","interface":38002},{"flavor":"mainline","interface":120100},` +
		`{"flavor":"classic","interface":11508},{"flavor":"bcc","interface":20505},{"flavor":"forever","interface":16001}]}]}`
	for _, gameType := range []wow.GameType{wow.GameMists, wow.GameTitan, wow.GameMainline, wow.GameVanilla, wow.GameTBC, wow.GameForever} {
		name, ok, err := pickReleaseJSONAsset([]byte(data), []string{"BigWigs-v1.zip", "release.json"}, gameType)
		if err != nil || !ok || name != "BigWigs-v1.zip" {
			t.Errorf("%s: got %q, %v, %v", gameType, name, ok, err)
		}
	}
}

func TestReleaseJSONMissingGameTypeNamesSupported(t *testing.T) {
	data := `{"releases":[
{"filename":"Addon.zip","nolib":false,"metadata":[{"flavor":"mainline","interface":1},{"flavor":"future","interface":1}]},
{"filename":"Addon-classic.zip","nolib":false,"metadata":[{"flavor":"classic","interface":1},{"flavor":"mainline","interface":1}]},
{"filename":"Addon-cata.zip","nolib":false,"metadata":[{"flavor":"cata","interface":1}]}
]}`
	_, ok, err := pickReleaseJSONAsset([]byte(data), []string{"Addon.zip", "Addon-classic.zip", "Addon-cata.zip"}, wow.GameWrath)
	if err == nil || ok {
		t.Fatalf("got ok=%v err=%v, want an authoritative error", ok, err)
	}
	if !strings.Contains(err.Error(), "wrath") || !strings.Contains(err.Error(), "mainline, vanilla, cata") {
		t.Errorf("error = %q, want the requested game type and the supported ones in file order", err)
	}
}

func TestReleaseJSONFallsBack(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		gameType wow.GameType
	}{
		{"malformed", `{"releases":`, wow.GameMainline},
		{"not an object", `[]`, wow.GameMainline},
		{"no entries", `{"releases":[]}`, wow.GameMainline},
		{"unknown filename", `{"releases":[{"filename":"Other.zip","nolib":false,"metadata":[{"flavor":"mainline","interface":1}]}]}`, wow.GameMainline},
		{"empty filename", `{"releases":[{"nolib":false,"metadata":[{"flavor":"mainline","interface":1}]}]}`, wow.GameMainline},
		{"not a zip", `{"releases":[{"filename":"Addon.tar.gz","nolib":false,"metadata":[{"flavor":"mainline","interface":1}]}]}`, wow.GameMainline},
		{"empty game type", `{"releases":[{"filename":"Addon.zip","nolib":false,"metadata":[{"flavor":"mainline","interface":1}]}]}`, ""},
	}
	for _, tt := range tests {
		name, ok, err := pickReleaseJSONAsset([]byte(tt.data), []string{"Addon.zip", "Addon.tar.gz"}, tt.gameType)
		if err != nil || ok || name != "" {
			t.Errorf("%s: got %q, %v, %v; want a fallback", tt.name, name, ok, err)
		}
	}
}
