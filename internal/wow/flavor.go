package wow

type GameFlavor string

const (
	Retail     GameFlavor = "retail"
	Classic    GameFlavor = "classic"
	ClassicEra GameFlavor = "classic-era"
	Beta       GameFlavor = "beta"
	PTR        GameFlavor = "ptr"
)

var flavorByDirName = map[string]GameFlavor{
	"_retail_":      Retail,
	"_classic_":     Classic,
	"_classic_era_": ClassicEra,
	"_beta_":        Beta,
	"_ptr_":         PTR,
}

func FlavorFromDirName(name string) (GameFlavor, bool) {
	flavor, ok := flavorByDirName[name]
	return flavor, ok
}

func FlavorFromInterface(num int) GameFlavor {
	switch {
	case num >= 80000:
		return Retail
	case num >= 30000 && num < 60000:
		return Classic
	case num >= 10000 && num < 30000:
		return ClassicEra
	default:
		return ""
	}
}
