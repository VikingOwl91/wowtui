package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"somegit.dev/vikingowl/wowtui/internal/fingerprint"
	"somegit.dev/vikingowl/wowtui/internal/index"
	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/registry"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

type releaseMatch struct {
	source  string
	release fingerprint.Release
	folders []string
	report  fingerprint.Report
}

// A configured index may publish a newer manifest for the same source/tag.
func releaseCatalog(idx *index.Index, flavor string) map[string][]fingerprint.Release {
	catalog := fingerprint.Builtin()
	if idx == nil {
		return catalog
	}
	for _, entry := range idx.Entries() {
		if len(entry.Fingerprints) == 0 || !slices.Contains(entry.GameTypes, wow.GameFlavor(flavor).GameType()) {
			continue
		}
		ref, err := provider.ParseReference(entry.Source)
		if err != nil {
			continue
		}
		key := provider.FormatReference(ref)
		for existing := range catalog {
			if sameReleaseSource(existing, key) {
				key = existing
				break
			}
		}
		for _, release := range entry.Fingerprints {
			releases := slices.DeleteFunc(catalog[key], func(r fingerprint.Release) bool { return r.Version == release.Version })
			catalog[key] = append(releases, release)
		}
	}
	return catalog
}

func sameReleaseSource(a, b string) bool {
	return a == b || (strings.HasPrefix(a, "github:") && strings.EqualFold(a, b))
}

// When files differ, a large main addon must not be able to establish the
// identity of an unrelated companion directory just because its name matches.
func corroboratesEveryFolder(release fingerprint.Release, report fingerprint.Report) bool {
	if report.Matched <= len(release.Files)/2 {
		return false
	}
	type counts struct{ expected, different int }
	folders := map[string]counts{}
	for _, file := range release.Files {
		name, _, _ := strings.Cut(file.Path, "/")
		n := folders[name]
		n.expected++
		folders[name] = n
	}
	for _, change := range report.Changes {
		if change.Kind == "added" {
			continue
		}
		name, _, _ := strings.Cut(change.Path, "/")
		n := folders[name]
		n.different++
		folders[name] = n
	}
	for _, n := range folders {
		if n.different*2 > n.expected {
			return false
		}
	}
	return true
}

// All declared roots must exist in the same scan. A TOC version narrows the
// search to a release; the actual file contents then establish the match.
func (s *Service) matchReleases(ctx context.Context, inst registry.Installation, onlySource, onlyFolder string) ([]releaseMatch, error) {
	var folders []string
	if onlyFolder != "" {
		folders = []string{onlyFolder}
	}
	return matchReleaseFolders(ctx, s.Index, inst, onlySource, folders)
}

func matchReleaseFolders(ctx context.Context, idx *index.Index, inst registry.Installation, onlySource string, onlyFolders []string) ([]releaseMatch, error) {
	versions := make(map[string]string, len(inst.Folders))
	counts := make(map[string]int, len(inst.Folders))
	for _, f := range inst.Folders {
		versions[f.Name] = f.Version
		counts[strings.ToLower(f.Name)]++
	}
	var matches []releaseMatch
	var problems []error
	for source, releases := range releaseCatalog(idx, inst.Flavor) {
		if onlySource != "" && !sameReleaseSource(source, onlySource) {
			continue
		}
		for _, release := range releases {
			folders := release.Folders()
			if len(onlyFolders) > 0 && !slices.ContainsFunc(onlyFolders, func(name string) bool { return slices.Contains(folders, name) }) {
				continue
			}
			allPresent, versionSeen := true, false
			for _, name := range folders {
				version, present := versions[name]
				allPresent = allPresent && present && counts[strings.ToLower(name)] == 1
				versionSeen = versionSeen || version == release.Version
			}
			if !allPresent || !versionSeen {
				continue
			}
			report, err := fingerprint.Compare(ctx, filepath.Join(inst.Path, "Interface", "AddOns"), release)
			if err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				problems = append(problems, fmt.Errorf("verify %s %s: %w", source, release.Version, err))
				continue
			}
			if corroboratesEveryFolder(release, report) {
				matches = append(matches, releaseMatch{source, release, folders, report})
			}
		}
	}
	// A mixed installation can resemble two different releases from the same
	// repository. Do not attach either version to their shared folder.
	type owner struct{ source, folder string }
	claims := make(map[owner]int)
	for _, m := range matches {
		for _, folder := range m.folders {
			claims[owner{m.source, folder}]++
		}
	}
	matches = slices.DeleteFunc(matches, func(m releaseMatch) bool {
		for _, folder := range m.folders {
			if claims[owner{m.source, folder}] > 1 {
				problems = append(problems, fmt.Errorf("multiple releases of %s match folder %s", m.source, folder))
				return true
			}
		}
		return false
	})
	return matches, errors.Join(problems...)
}

// Restrict legacy baselines to a known provider, version and owned folder.
// The update guard must also catch installations that have diverged too far
// for discovery to recommend them as a new package.
func (s *Service) knownBaseline(ctx context.Context, reg *registry.Registry, addon *registry.Addon) ([]fingerprint.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	idx := s.Index
	if idx == nil && s.LoadFingerprintIndex != nil {
		var err error
		idx, err = s.LoadFingerprintIndex()
		if err != nil {
			return nil, fmt.Errorf("load release fingerprints: %w", err)
		}
	}
	ref := provider.FormatReference(provider.Reference{Provider: addon.Provider, ID: addon.Ref})
	for source, releases := range releaseCatalog(idx, addon.Flavor) {
		if !sameReleaseSource(ref, source) {
			continue
		}
		for _, release := range releases {
			if release.Version != addon.Version {
				continue
			}
			folders := release.Folders()
			if slices.ContainsFunc(addon.Folders, func(name string) bool { return slices.Contains(folders, name) }) {
				// Older versions could register one root of a larger package.
				return scopedBaseline([]releaseMatch{{release: release}}, addon.Folders)
			}
		}
	}
	for _, inst := range reg.Installations {
		if inst.Path == addon.InstallPath {
			return alternateBaseline(ctx, idx, inst, addon.Folders)
		}
	}
	return nil, nil
}

// Merge the stronger file evidence into an existing TOC or index suggestion
// for the same source, keeping alternative providers separately selectable.
func addReleaseCandidates(candidates []MappingCandidate, inst registry.Installation, f registry.Folder, matches []releaseMatch) []MappingCandidate {
	for _, m := range matches {
		if !slices.Contains(m.folders, f.Name) {
			continue
		}
		i := slices.IndexFunc(candidates, func(c MappingCandidate) bool { return sameReleaseSource(c.Reference, m.source) })
		if i < 0 {
			ref, _ := provider.ParseReference(m.source)
			i = len(candidates)
			candidates = append(candidates, MappingCandidate{InstallPath: inst.Path, Flavor: inst.Flavor, Folder: f.Name, Version: f.Version, Provider: ref.Provider, Reference: m.source})
		}
		c := &candidates[i]
		if c.Reason != "" {
			c.Reason += ","
		}
		c.Reason += "release:fingerprint"
		c.ReleaseVersion = m.release.Version
		c.PackageFolders = m.folders
		c.FileChanges = m.report.Changes
	}
	return candidates
}
