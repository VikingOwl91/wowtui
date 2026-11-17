package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"somegit.dev/vikingowl/wowtui/internal/wow"
)

const (
	releaseJSONName    = "release.json"
	maxReleaseJSONSize = 1 << 20
)

// The BigWigs packager writes its game_flavor names into release.json, with
// retail renamed to mainline. Its "classic" is vanilla (classic era).
var releaseJSONFlavors = map[string]wow.GameType{
	"mainline": wow.GameMainline,
	"classic":  wow.GameVanilla,
	"bcc":      wow.GameTBC,
	"wrath":    wow.GameWrath,
	"titan":    wow.GameTitan,
	"cata":     wow.GameCata,
	"mists":    wow.GameMists,
	"forever":  wow.GameForever,
}

type releaseJSON struct {
	Releases []struct {
		Filename string `json:"filename"`
		NoLib    bool   `json:"nolib"`
		Metadata []struct {
			Flavor string `json:"flavor"`
		} `json:"metadata"`
	} `json:"releases"`
}

// pickReleaseJSONAsset returns the asset a packager release.json names for
// gameType. ok is false when release.json gives no usable answer and the
// caller falls back to name heuristics. An error means release.json lists
// builds, but none for gameType.
func pickReleaseJSONAsset(data []byte, assets []string, gameType wow.GameType) (name string, ok bool, err error) {
	if gameType == "" {
		return "", false, nil
	}
	var manifest releaseJSON
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", false, nil
	}

	chosen := -1
	var supported []string
	for i, entry := range manifest.Releases {
		for _, meta := range entry.Metadata {
			metaType, known := releaseJSONFlavors[meta.Flavor]
			if !known {
				continue
			}
			if !slices.Contains(supported, string(metaType)) {
				supported = append(supported, string(metaType))
			}
			if metaType == gameType && (chosen < 0 || (manifest.Releases[chosen].NoLib && !entry.NoLib)) {
				chosen = i
			}
		}
	}
	if len(supported) == 0 {
		return "", false, nil
	}
	if chosen < 0 {
		return "", false, fmt.Errorf("release.json has no build for game type %s, it supports %s", gameType, strings.Join(supported, ", "))
	}

	filename := manifest.Releases[chosen].Filename
	if !strings.EqualFold(filepath.Ext(filename), ".zip") || !slices.Contains(assets, filename) {
		return "", false, nil
	}
	return filename, true, nil
}

func (g *GitHub) releaseJSONAsset(ctx context.Context, assets []ghAsset, names []string, flavor wow.GameFlavor) (string, bool, error) {
	for _, asset := range assets {
		if asset.Name != releaseJSONName {
			continue
		}
		data, err := g.fetchReleaseJSON(ctx, asset.BrowserDownloadURL)
		if err != nil {
			return "", false, nil
		}
		return pickReleaseJSONAsset(data, names, flavor.GameType())
	}
	return "", false, nil
}

func (g *GitHub) fetchReleaseJSON(ctx context.Context, url string) ([]byte, error) {
	body, err := g.Download(ctx, Release{AssetName: releaseJSONName, DownloadURL: url})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, maxReleaseJSONSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxReleaseJSONSize {
		return nil, errors.New("release.json exceeds 1 MiB")
	}
	return data, nil
}
