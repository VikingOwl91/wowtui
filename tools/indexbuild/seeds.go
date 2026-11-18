package main

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"somegit.dev/vikingowl/wowtui/internal/index"
	"somegit.dev/vikingowl/wowtui/internal/installer"
	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/registry"
)

// idHints maps the TOC hint keys the registry stores to the index kind and
// to the key spelling used in code search queries.
var idHints = []struct{ key, kind, queryKey string }{
	{"x-curse-project-id", index.KindCurseForge, "X-Curse-Project-ID"},
	{"x-wago-id", index.KindWago, "X-Wago-ID"},
	{"x-wowi-id", index.KindWoWInterface, "X-WoWI-ID"},
}

var idValuePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// githubOwnersNotRepos are github.com path prefixes that are not user or
// organization names.
var githubOwnersNotRepos = []string{"sponsors", "orgs", "topics", "settings", "marketplace", "features", "apps", "login", "about", "pricing", "collections", "trending"}

// seed is one installed folder name with the hints that identify its addon.
type seed struct {
	Folder   string
	IDs      map[string][]string
	Repos    []string
	BadHints []string
}

type seedBuilder struct {
	names map[string]bool
	seed  seed
}

// loadRegistryFile reads a wowtui registry. Unlike registry.Load, a missing
// file is an error: an empty seed set would silently produce an empty index.
func loadRegistryFile(path string) (*registry.Registry, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	return registry.Load(path)
}

// loadSeeds collects every folder name of every installation. Folder names
// that differ only in case are one folder. Only names and hints are read;
// installation paths never leave this function. seeds holds the folders with
// at least one usable ID or GitHub URL hint, folders all distinct names.
func loadSeeds(reg *registry.Registry) (seeds []*seed, folders []string) {
	builders := map[string]*seedBuilder{}
	for _, install := range reg.Installations {
		for _, folder := range install.Folders {
			if !installer.SafeFolderName(folder.Name) {
				continue
			}
			key := strings.ToLower(folder.Name)
			b := builders[key]
			if b == nil {
				b = &seedBuilder{names: map[string]bool{}, seed: seed{IDs: map[string][]string{}}}
				builders[key] = b
			}
			b.names[folder.Name] = true
			b.add(folder.Hints)
		}
	}
	keys := make([]string, 0, len(builders))
	for key := range builders {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		b := builders[key]
		names := make([]string, 0, len(b.names))
		for name := range b.names {
			names = append(names, name)
		}
		slices.Sort(names)
		b.seed.Folder = names[0]
		folders = append(folders, names[0])
		for kind := range b.seed.IDs {
			slices.Sort(b.seed.IDs[kind])
		}
		slices.Sort(b.seed.Repos)
		slices.Sort(b.seed.BadHints)
		if len(b.seed.IDs) > 0 || len(b.seed.Repos) > 0 {
			s := b.seed
			seeds = append(seeds, &s)
		}
	}
	return seeds, folders
}

func (b *seedBuilder) add(hints map[string]string) {
	for _, hint := range idHints {
		value := strings.TrimSpace(hints[hint.key])
		if value == "" {
			continue
		}
		if !idValuePattern.MatchString(value) {
			bad := fmt.Sprintf("%s=%q", hint.key, value)
			if !slices.Contains(b.seed.BadHints, bad) {
				b.seed.BadHints = append(b.seed.BadHints, bad)
			}
			continue
		}
		if !slices.Contains(b.seed.IDs[hint.kind], value) {
			b.seed.IDs[hint.kind] = append(b.seed.IDs[hint.kind], value)
		}
	}
	for _, key := range []string{"x-website", "x-source-repo"} {
		if repo, ok := githubRepoHint(hints[key]); ok && !slices.Contains(b.seed.Repos, repo) {
			b.seed.Repos = append(b.seed.Repos, repo)
		}
	}
}

// githubRepoHint returns Owner/Repo for a github.com URL hint.
func githubRepoHint(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "https://") && !strings.HasPrefix(value, "http://") {
		return "", false
	}
	ref, err := provider.ParseReference(value)
	if err != nil || ref.Provider != "github" {
		return "", false
	}
	owner, _, _ := strings.Cut(ref.ID, "/")
	if slices.Contains(githubOwnersNotRepos, strings.ToLower(owner)) {
		return "", false
	}
	return ref.ID, true
}
