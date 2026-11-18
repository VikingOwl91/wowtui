package service

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"somegit.dev/vikingowl/wowtui/internal/fingerprint"
	"somegit.dev/vikingowl/wowtui/internal/index"
	"somegit.dev/vikingowl/wowtui/internal/registry"
)

// The bytes of an independently identified release can protect owned files
// even when the user chooses a different provider for future updates.
func alternateBaseline(ctx context.Context, idx *index.Index, inst registry.Installation, owned []string) ([]fingerprint.File, error) {
	if len(owned) == 0 {
		return nil, nil
	}
	matches, err := matchReleaseFolders(ctx, idx, inst, "", owned)
	if err != nil {
		return nil, err
	}
	return scopedBaseline(matches, owned)
}

func scopedBaseline(matches []releaseMatch, owned []string) ([]fingerprint.File, error) {
	byFolder := make(map[string][]fingerprint.File)
	for _, match := range matches {
		files := make(map[string][]fingerprint.File)
		for _, f := range match.release.Files {
			folder, _, _ := strings.Cut(f.Path, "/")
			if slices.Contains(owned, folder) {
				files[folder] = append(files[folder], f)
			}
		}
		for folder, candidate := range files {
			slices.SortFunc(candidate, func(a, b fingerprint.File) int { return cmp.Compare(a.Path, b.Path) })
			if previous, found := byFolder[folder]; found && !slices.Equal(previous, candidate) {
				return nil, fmt.Errorf("different release fingerprints match folder %q", folder)
			}
			byFolder[folder] = candidate
		}
	}
	if len(byFolder) == 0 {
		return nil, nil
	}
	var files []fingerprint.File
	for _, folder := range owned {
		if len(byFolder[folder]) == 0 {
			return nil, fmt.Errorf("the release fingerprint does not cover selected folder %q", folder)
		}
		files = append(files, byFolder[folder]...)
	}
	return files, nil
}
