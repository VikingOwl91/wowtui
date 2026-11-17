package provider

import (
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/wow"
)

func TestClassifyAsset(t *testing.T) {
	tests := []struct {
		name    string
		want    wow.GameType
		generic bool
	}{
		{"Addon-1.0.zip", "", false},
		{"Addon-1.0-retail.zip", wow.GameMainline, false},
		{"Addon-1.0-mainline.zip", wow.GameMainline, false},
		{"Addon-1.0-classic.zip", wow.GameVanilla, true},
		{"Addon-1.0-cata.zip", wow.GameCata, false},
		{"Addon-1.0-cataclysm.zip", wow.GameCata, false},
		{"Addon-1.0-vanilla.zip", wow.GameVanilla, false},
		{"Addon-1.0-classic_era.zip", wow.GameVanilla, false},
		{"Addon-1.0-classicera.zip", wow.GameVanilla, false},
		{"Addon_1.0_Retail.zip", wow.GameMainline, false},
		{"Addon-1.0-beta.zip", "", false},
		{"Addon-1.0-bcc.zip", wow.GameTBC, false},
		{"Addon-1.0-tbc.zip", wow.GameTBC, false},
		{"Addon-1.0-wrath.zip", wow.GameWrath, false},
		{"Addon-1.0-wotlkc.zip", wow.GameWrath, false},
		{"Addon-1.0-titan.zip", wow.GameTitan, false},
		{"Addon-1.0-mists.zip", wow.GameMists, false},
		{"Addon-1.0-forever.zip", wow.GameForever, false},
		{"Addon-1.0-camelot.zip", wow.GameForever, false},
		{"Addon-1.0-mists-classic.zip", wow.GameMists, false},
	}
	for _, tt := range tests {
		got, generic := classifyAsset(tt.name)
		if got != tt.want || generic != tt.generic {
			t.Errorf("classifyAsset(%q) = %q, %v; want %q, %v", tt.name, got, generic, tt.want, tt.generic)
		}
	}
}

func TestPickAssetPerGameType(t *testing.T) {
	assets := []string{
		"Addon-1.0.zip",
		"Addon-1.0-classic.zip",
		"Addon-1.0-bcc.zip",
		"Addon-1.0-wrath.zip",
		"Addon-1.0-titan.zip",
		"Addon-1.0-cata.zip",
		"Addon-1.0-mists.zip",
		"Addon-1.0-forever.zip",
	}
	want := map[wow.GameFlavor]string{
		wow.Retail:        "Addon-1.0.zip",
		wow.PTR:           "Addon-1.0.zip",
		wow.XPTR:          "Addon-1.0.zip",
		wow.Beta:          "Addon-1.0.zip",
		wow.Classic:       "Addon-1.0-mists.zip",
		wow.ClassicPTR:    "Addon-1.0-mists.zip",
		wow.ClassicBeta:   "Addon-1.0-forever.zip",
		wow.ClassicEra:    "Addon-1.0-classic.zip",
		wow.ClassicEraPTR: "Addon-1.0-bcc.zip",
		wow.Anniversary:   "Addon-1.0-bcc.zip",
		wow.ClassicTitan:  "Addon-1.0-titan.zip",
		wow.Forever:       "Addon-1.0-forever.zip",
	}
	for _, flavor := range wow.Flavors() {
		got, err := PickAsset(assets, flavor)
		if err != nil {
			t.Errorf("PickAsset(%q): %v", flavor, err)
			continue
		}
		if got != want[flavor] {
			t.Errorf("PickAsset(%q) = %q, want %q", flavor, got, want[flavor])
		}
	}
}

func TestPickAssetTestClientsMatchBaseClient(t *testing.T) {
	assets := []string{"Addon-1.0-retail.zip", "Addon-1.0-vanilla.zip", "Addon-1.0-tbc.zip", "Addon-1.0-mists.zip"}
	pairs := []struct{ test, base wow.GameFlavor }{
		{wow.PTR, wow.Retail},
		{wow.XPTR, wow.Retail},
		{wow.Beta, wow.Retail},
		{wow.ClassicPTR, wow.Classic},
		{wow.ClassicEraPTR, wow.Anniversary},
	}
	for _, pair := range pairs {
		base, err := PickAsset(assets, pair.base)
		if err != nil {
			t.Fatalf("PickAsset(%q): %v", pair.base, err)
		}
		got, err := PickAsset(assets, pair.test)
		if err != nil {
			t.Fatalf("PickAsset(%q): %v", pair.test, err)
		}
		if got != base {
			t.Errorf("PickAsset(%q) = %q, want base client asset %q", pair.test, got, base)
		}
	}
}

func TestPickAssetMainlineToken(t *testing.T) {
	got, err := PickAsset([]string{"Addon-1.0-classic.zip", "Addon-1.0-mainline.zip"}, wow.XPTR)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Addon-1.0-mainline.zip" {
		t.Errorf("got %q", got)
	}
}

func TestPickAssetSingleZipWins(t *testing.T) {
	got, err := PickAsset([]string{"Addon-1.0.zip", "README.md"}, wow.Classic)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Addon-1.0.zip" {
		t.Errorf("got %q", got)
	}
}

func TestPickAssetFlavorMatch(t *testing.T) {
	assets := []string{"Addon-1.0-classic.zip", "Addon-1.0-vanilla.zip", "Addon-1.0-retail.zip"}
	got, err := PickAsset(assets, wow.ClassicEra)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Addon-1.0-vanilla.zip" {
		t.Errorf("got %q", got)
	}
}

func TestPickAssetFallsBackToUniversal(t *testing.T) {
	assets := []string{"Addon-1.0-classic.zip", "Addon-1.0.zip"}
	got, err := PickAsset(assets, wow.Retail)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Addon-1.0.zip" {
		t.Errorf("got %q", got)
	}
}

func TestPickAssetNoZip(t *testing.T) {
	if _, err := PickAsset([]string{"README.md", "source.tar.gz"}, wow.Retail); err == nil {
		t.Fatal("expected error for release without zip assets")
	}
}

func TestPickAssetNoFlavorMatch(t *testing.T) {
	assets := []string{"Addon-1.0-classic.zip", "Addon-1.0-vanilla.zip"}
	if _, err := PickAsset(assets, wow.Retail); err == nil {
		t.Fatal("expected error when no asset matches flavor")
	}
}

func TestPickAssetGenericClassicIsNotProgression(t *testing.T) {
	assets := []string{"Addon-1.0-classic.zip", "Addon-1.0-retail.zip"}
	if got, err := PickAsset(assets, wow.Classic); err == nil {
		t.Fatalf("PickAsset(classic) = %q, want error: a -classic asset is a vanilla build", got)
	}
}
