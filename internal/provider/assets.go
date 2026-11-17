package provider

import (
	"fmt"
	"path/filepath"
	"strings"

	"somegit.dev/vikingowl/wowtui/internal/wow"
)

var gameTypeTokens = []struct {
	gameType wow.GameType
	tokens   []string
}{
	{wow.GameVanilla, []string{"vanilla", "era", "classicera"}},
	{wow.GameTBC, []string{"bcc", "tbc"}},
	{wow.GameTitan, []string{"titan"}},
	{wow.GameWrath, []string{"wrath", "wotlkc"}},
	{wow.GameCata, []string{"cata", "cataclysm"}},
	{wow.GameMists, []string{"mists"}},
	{wow.GameForever, []string{"forever", "camelot"}},
	{wow.GameMainline, []string{"retail", "mainline"}},
}

// BigWigs packager names vanilla builds "-classic", so the bare token only
// counts when no more specific token is present.
const genericClassicToken = "classic"

func classifyAsset(name string) (gameType wow.GameType, generic bool) {
	tokens := map[string]bool{}
	for _, part := range strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}) {
		tokens[part] = true
	}

	for _, entry := range gameTypeTokens {
		for _, token := range entry.tokens {
			if tokens[token] {
				return entry.gameType, false
			}
		}
	}
	if tokens[genericClassicToken] {
		return wow.GameVanilla, true
	}
	return "", false
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

	gameType := flavor.GameType()
	var generic string
	for _, name := range zips {
		assetType, isGeneric := classifyAsset(name)
		if assetType != gameType {
			continue
		}
		if !isGeneric {
			return name, nil
		}
		if generic == "" {
			generic = name
		}
	}
	if generic != "" {
		return generic, nil
	}
	for _, name := range zips {
		if assetType, _ := classifyAsset(name); assetType == "" {
			return name, nil
		}
	}
	return "", fmt.Errorf("no zip asset matches flavor %q, available: %s", flavor, strings.Join(zips, ", "))
}
