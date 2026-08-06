package provider

import (
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/wow"
)

func TestClassifyAsset(t *testing.T) {
	tests := []struct {
		name string
		want wow.GameFlavor
	}{
		{"Addon-1.0.zip", ""},
		{"Addon-1.0-retail.zip", wow.Retail},
		{"Addon-1.0-mainline.zip", wow.Retail},
		{"Addon-1.0-classic.zip", wow.Classic},
		{"Addon-1.0-cata.zip", wow.Classic},
		{"Addon-1.0-cataclysm.zip", wow.Classic},
		{"Addon-1.0-vanilla.zip", wow.ClassicEra},
		{"Addon-1.0-classic_era.zip", wow.ClassicEra},
		{"Addon-1.0-classicera.zip", wow.ClassicEra},
		{"Addon_1.0_Retail.zip", wow.Retail},
		{"Addon-1.0-beta.zip", ""},
	}
	for _, tt := range tests {
		if got := classifyAsset(tt.name); got != tt.want {
			t.Errorf("classifyAsset(%q) = %q, want %q", tt.name, got, tt.want)
		}
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
