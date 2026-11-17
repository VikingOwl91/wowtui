package wow

import "testing"

var clientDirTests = []struct {
	dir      string
	flavor   GameFlavor
	gameType GameType
}{
	{"_retail_", Retail, GameMainline},
	{"_ptr_", PTR, GameMainline},
	{"_xptr_", XPTR, GameMainline},
	{"_beta_", Beta, GameMainline},
	{"_classic_", Classic, GameMists},
	{"_classic_ptr_", ClassicPTR, GameMists},
	{"_classic_beta_", ClassicBeta, GameForever},
	{"_classic_era_", ClassicEra, GameVanilla},
	{"_classic_era_ptr_", ClassicEraPTR, GameTBC},
	{"_anniversary_", Anniversary, GameTBC},
	{"_classic_titan_", ClassicTitan, GameTitan},
	{"_forever_", Forever, GameForever},
}

func TestFlavorIDsKeepRegistryValues(t *testing.T) {
	tests := map[GameFlavor]string{
		Retail:        "retail",
		Classic:       "classic",
		ClassicEra:    "classic-era",
		Beta:          "beta",
		PTR:           "ptr",
		XPTR:          "xptr",
		ClassicPTR:    "classic-ptr",
		ClassicBeta:   "classic-beta",
		ClassicEraPTR: "classic-era-ptr",
		Anniversary:   "anniversary",
		ClassicTitan:  "classic-titan",
		Forever:       "forever",
	}
	for flavor, want := range tests {
		if string(flavor) != want {
			t.Errorf("flavor %q, want %q", flavor, want)
		}
	}
}

func TestFlavorFromDirName(t *testing.T) {
	for _, tt := range clientDirTests {
		got, found := FlavorFromDirName(tt.dir)
		if got != tt.flavor || !found {
			t.Errorf("FlavorFromDirName(%q) = %q, %v; want %q, true", tt.dir, got, found, tt.flavor)
		}
	}
	for _, dir := range []string{"_unknown_", "retail", "", "_Retail_", "_classic_era_beta_", "_cn_beta_"} {
		if got, found := FlavorFromDirName(dir); got != "" || found {
			t.Errorf("FlavorFromDirName(%q) = %q, %v; want \"\", false", dir, got, found)
		}
	}
}

func TestFlavorsListsEveryClientDir(t *testing.T) {
	got := Flavors()
	if len(got) != len(clientDirTests) {
		t.Fatalf("Flavors() = %v, want %d flavors", got, len(clientDirTests))
	}
	for i, tt := range clientDirTests {
		if got[i] != tt.flavor {
			t.Errorf("Flavors()[%d] = %q, want %q", i, got[i], tt.flavor)
		}
	}
	got[0] = "mutated"
	if Flavors()[0] != Retail {
		t.Error("Flavors() exposes internal state")
	}
}

func TestGameFlavorGameType(t *testing.T) {
	for _, tt := range clientDirTests {
		if got := tt.flavor.GameType(); got != tt.gameType {
			t.Errorf("%q.GameType() = %q, want %q", tt.flavor, got, tt.gameType)
		}
	}
	for _, flavor := range []GameFlavor{"", "unknown", "Retail"} {
		if got := flavor.GameType(); got != "" {
			t.Errorf("%q.GameType() = %q, want empty", flavor, got)
		}
	}
}

func TestTestClientsShareBaseGameType(t *testing.T) {
	tests := []struct{ test, base GameFlavor }{
		{PTR, Retail},
		{XPTR, Retail},
		{Beta, Retail},
		{ClassicPTR, Classic},
		{ClassicBeta, Forever},
		{ClassicEraPTR, Anniversary},
	}
	for _, tt := range tests {
		if tt.test.GameType() != tt.base.GameType() {
			t.Errorf("%q game type %q differs from base %q game type %q",
				tt.test, tt.test.GameType(), tt.base, tt.base.GameType())
		}
	}
}

func TestGameTypeValues(t *testing.T) {
	want := []GameType{GameMainline, GameVanilla, GameTBC, GameWrath, GameTitan, GameCata, GameMists, GameForever}
	values := []string{"mainline", "vanilla", "tbc", "wrath", "titan", "cata", "mists", "forever"}
	for i, gameType := range want {
		if string(gameType) != values[i] {
			t.Errorf("game type %q, want %q", gameType, values[i])
		}
	}
}

func TestGameTypeFromInterface(t *testing.T) {
	tests := []struct {
		num  int
		want GameType
	}{
		{120107, GameMainline},
		{120105, GameMainline},
		{110200, GameMainline},
		{80000, GameMainline},
		{79999, ""},
		{60000, ""},
		{59999, GameMists},
		{50504, GameMists},
		{50000, GameMists},
		{49999, GameCata},
		{40402, GameCata},
		{40000, GameCata},
		{39999, GameTitan},
		{38002, GameTitan},
		{38000, GameTitan},
		{37999, GameWrath},
		{30405, GameWrath},
		{30000, GameWrath},
		{29999, GameTBC},
		{20506, GameTBC},
		{20000, GameTBC},
		{19999, GameForever},
		{16001, GameForever},
		{16000, GameForever},
		{15999, GameVanilla},
		{11509, GameVanilla},
		{10000, GameVanilla},
		{9999, ""},
		{0, ""},
		{-1, ""},
	}
	for _, tt := range tests {
		if got := GameTypeFromInterface(tt.num); got != tt.want {
			t.Errorf("GameTypeFromInterface(%d) = %q; want %q", tt.num, got, tt.want)
		}
	}
}

func TestFlavorForGameType(t *testing.T) {
	tests := []struct {
		gameType GameType
		want     GameFlavor
	}{
		{GameMainline, Retail},
		{GameMists, Classic},
		{GameVanilla, ClassicEra},
		{GameTBC, Anniversary},
		{GameTitan, ClassicTitan},
		{GameForever, Forever},
		{GameWrath, ""},
		{GameCata, ""},
		{"", ""},
		{"unknown", ""},
	}
	for _, tt := range tests {
		if got := FlavorForGameType(tt.gameType); got != tt.want {
			t.Errorf("FlavorForGameType(%q) = %q; want %q", tt.gameType, got, tt.want)
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
		{40402, ""},
		{38002, ClassicTitan},
		{30405, ""},
		{30403, ""},
		{20506, Anniversary},
		{20504, Anniversary},
		{16001, Forever},
		{11509, ClassicEra},
		{11503, ClassicEra},
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
