package wow

type GameFlavor string

const (
	Retail        GameFlavor = "retail"
	PTR           GameFlavor = "ptr"
	XPTR          GameFlavor = "xptr"
	Beta          GameFlavor = "beta"
	Classic       GameFlavor = "classic"
	ClassicPTR    GameFlavor = "classic-ptr"
	ClassicBeta   GameFlavor = "classic-beta"
	ClassicEra    GameFlavor = "classic-era"
	ClassicEraPTR GameFlavor = "classic-era-ptr"
	Anniversary   GameFlavor = "anniversary"
	ClassicTitan  GameFlavor = "classic-titan"
	Forever       GameFlavor = "forever"
)

type GameType string

const (
	GameMainline GameType = "mainline"
	GameVanilla  GameType = "vanilla"
	GameTBC      GameType = "tbc"
	GameWrath    GameType = "wrath"
	GameTitan    GameType = "titan"
	GameCata     GameType = "cata"
	GameMists    GameType = "mists"
	GameForever  GameType = "forever"
)

// Directory names are Blizzard's shared_container_default_subfolder per
// product. The game types of _classic_, _classic_ptr_, _classic_beta_,
// _classic_era_ptr_ and _anniversary_ follow the content Blizzard ships there
// now and must be revisited when those clients move to another expansion.
var clientDirs = []struct {
	dir        string
	flavor     GameFlavor
	gameType   GameType
	testClient bool
}{
	{"_retail_", Retail, GameMainline, false},
	{"_ptr_", PTR, GameMainline, true},
	{"_xptr_", XPTR, GameMainline, true},
	{"_beta_", Beta, GameMainline, true},
	{"_classic_", Classic, GameMists, false},
	{"_classic_ptr_", ClassicPTR, GameMists, true},
	{"_classic_beta_", ClassicBeta, GameForever, true},
	{"_classic_era_", ClassicEra, GameVanilla, false},
	{"_classic_era_ptr_", ClassicEraPTR, GameTBC, true},
	{"_anniversary_", Anniversary, GameTBC, false},
	{"_classic_titan_", ClassicTitan, GameTitan, false},
	{"_forever_", Forever, GameForever, false},
}

func Flavors() []GameFlavor {
	flavors := make([]GameFlavor, 0, len(clientDirs))
	for _, client := range clientDirs {
		flavors = append(flavors, client.flavor)
	}
	return flavors
}

func (f GameFlavor) GameType() GameType {
	for _, client := range clientDirs {
		if client.flavor == f {
			return client.gameType
		}
	}
	return ""
}

func FlavorFromDirName(name string) (GameFlavor, bool) {
	for _, client := range clientDirs {
		if client.dir == name {
			return client.flavor, true
		}
	}
	return "", false
}

func GameTypeFromInterface(num int) GameType {
	switch {
	case num >= 80000:
		return GameMainline
	case num >= 60000:
		return ""
	case num >= 50000:
		return GameMists
	case num >= 40000:
		return GameCata
	case num >= 38000:
		return GameTitan
	case num >= 30000:
		return GameWrath
	case num >= 20000:
		return GameTBC
	case num >= 16000:
		return GameForever
	case num >= 10000:
		return GameVanilla
	default:
		return ""
	}
}

// FlavorForGameType returns the only non-test client running gameType, or ""
// when no such client exists.
func FlavorForGameType(gameType GameType) GameFlavor {
	var found GameFlavor
	for _, client := range clientDirs {
		if client.testClient || client.gameType != gameType {
			continue
		}
		if found != "" {
			return ""
		}
		found = client.flavor
	}
	return found
}

func FlavorFromInterface(num int) GameFlavor {
	return FlavorForGameType(GameTypeFromInterface(num))
}
