package wow

import "testing"

func TestFlavorFromDirName(t *testing.T) {
	tests := []struct {
		dir   string
		want  GameFlavor
		found bool
	}{
		{"_retail_", Retail, true},
		{"_classic_", Classic, true},
		{"_classic_era_", ClassicEra, true},
		{"_beta_", Beta, true},
		{"_ptr_", PTR, true},
		{"_unknown_", "", false},
		{"retail", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, found := FlavorFromDirName(tt.dir)
		if got != tt.want || found != tt.found {
			t.Errorf("FlavorFromDirName(%q) = %q, %v; want %q, %v",
				tt.dir, got, found, tt.want, tt.found)
		}
	}
}

func TestFlavorFromInterface(t *testing.T) {
	tests := []struct {
		num  int
		want GameFlavor
	}{
		{120107, Retail},
		{120105, Retail},
		{120000, Retail},
		{110200, Retail},
		{90005, Retail},
		{80300, Retail},
		{50504, Classic},
		{50000, Classic},
		{59999, Classic},
		{40402, Classic},
		{38002, Classic},
		{30405, Classic},
		{30403, Classic},
		{11509, ClassicEra},
		{11503, ClassicEra},
		{20504, ClassicEra},
		{0, ""},
		{5000, ""},
		{60000, ""},
		{79999, ""},
	}
	for _, tt := range tests {
		if got := FlavorFromInterface(tt.num); got != tt.want {
			t.Errorf("FlavorFromInterface(%d) = %q; want %q", tt.num, got, tt.want)
		}
	}
}
