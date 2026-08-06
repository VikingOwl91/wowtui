package provider

import (
	"fmt"
	"path/filepath"
	"strings"

	"somegit.dev/vikingowl/wowtui/internal/wow"
)

var flavorTokens = map[wow.GameFlavor][]string{
	wow.ClassicEra: {"vanilla", "era", "classicera"},
	wow.Classic:    {"classic", "cata", "cataclysm"},
	wow.Retail:     {"retail", "mainline"},
}

func classifyAsset(name string) wow.GameFlavor {
	tokens := map[string]bool{}
	for _, part := range strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}) {
		tokens[part] = true
	}

	for _, flavor := range []wow.GameFlavor{wow.ClassicEra, wow.Classic, wow.Retail} {
		for _, token := range flavorTokens[flavor] {
			if tokens[token] {
				return flavor
			}
		}
	}
	return ""
}

func PickAsset(names []string, flavor wow.GameFlavor) (string, error) {
	var zips []string
	for _, name := range names {
		if strings.EqualFold(filepath.Ext(name), ".zip") {
			zips = append(zips, name)
		}
	}
	if len(zips) == 0 {
		return "", fmt.Errorf("release has no zip assets")
	}
	if len(zips) == 1 {
		return zips[0], nil
	}

	for _, name := range zips {
		if classifyAsset(name) == flavor {
			return name, nil
		}
	}
	for _, name := range zips {
		if classifyAsset(name) == "" {
			return name, nil
		}
	}
	return "", fmt.Errorf("no zip asset matches flavor %q, available: %s", flavor, strings.Join(zips, ", "))
}
