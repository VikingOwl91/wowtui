package main

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/index"
	"somegit.dev/vikingowl/wowtui/internal/installer"
	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

type BuildResult struct {
	File     index.File
	Warnings []string
}

type BuildSummary struct {
	Entries  int
	Warnings []string
}

var (
	slugInvalid  = regexp.MustCompile(`[^a-z0-9._-]+`)
	slugDashes   = regexp.MustCompile(`-{2,}`)
	providerID   = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	slugFallback = "addon"
)

func slugify(s string) string {
	s = slugInvalid.ReplaceAllString(strings.ToLower(s), "-")
	s = slugDashes.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-._")
	if s == "" {
		return slugFallback
	}
	return s
}

// assignSlugs derives one unique index id per repository. The result only
// depends on the set of repositories, never on their order: a name used by
// several repositories becomes owner-name for all of them, and names that
// still collide get a numeric suffix in sorted repository order.
func assignSlugs(pkgs []Package) map[string]string {
	repos := make([]string, 0, len(pkgs))
	for _, pkg := range pkgs {
		repos = append(repos, pkg.Repo)
	}
	slices.SortFunc(repos, func(a, b string) int {
		if c := strings.Compare(strings.ToLower(a), strings.ToLower(b)); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	})
	repos = slices.Compact(repos)

	base := map[string]string{}
	count := map[string]int{}
	for _, repo := range repos {
		base[repo] = slugify(repoNamePart(repo))
		count[base[repo]]++
	}
	slugs := map[string]string{}
	used := map[string]bool{}
	for _, repo := range repos {
		slug := base[repo]
		if count[slug] > 1 {
			slug = slugify(strings.ReplaceAll(repo, "/", "-"))
		}
		for n := 2; used[slug]; n++ {
			slug = fmt.Sprintf("%s-%d", slugify(strings.ReplaceAll(repo, "/", "-")), n)
		}
		used[slug] = true
		slugs[repo] = slug
	}
	return slugs
}

// Build turns candidates into an index document. Packages the index could
// not hold (no safe folder, no game type, a bad source) are skipped and
// reported as warnings. The result is the same for any package order.
func Build(cands *Candidates) (BuildResult, error) {
	if cands.Format != candidatesFormat {
		return BuildResult{}, fmt.Errorf("candidates have format %d, want %d", cands.Format, candidatesFormat)
	}
	if cands.GeneratedAt.IsZero() {
		return BuildResult{}, fmt.Errorf("candidates have no generated_at")
	}
	var warnings []string
	slugs := assignSlugs(cands.Packages)
	var entries []index.Entry
	seenRepos := map[string]bool{}
	for _, pkg := range cands.Packages {
		if seenRepos[pkg.Repo] {
			warnings = append(warnings, fmt.Sprintf("%s: listed twice, the repeat is ignored", pkg.Repo))
			continue
		}
		seenRepos[pkg.Repo] = true
		entry, warns, ok := entryFor(pkg, slugs[pkg.Repo])
		warnings = append(warnings, warns...)
		if ok {
			entries = append(entries, entry)
		}
	}
	slices.SortFunc(entries, func(a, b index.Entry) int { return strings.Compare(a.ID, b.ID) })

	taken := map[[2]string]string{}
	for i := range entries {
		entries[i].IDs, warnings = claimIDs(entries[i], taken, warnings)
	}
	if entries == nil {
		entries = []index.Entry{}
	}
	slices.Sort(warnings)
	return BuildResult{
		File:     index.File{Format: index.FormatVersion, GeneratedAt: cands.GeneratedAt.UTC().Truncate(time.Second), Entries: entries},
		Warnings: warnings,
	}, nil
}

// claimIDs keeps each provider ID on one entry, the first in id order.
func claimIDs(entry index.Entry, taken map[[2]string]string, warnings []string) (index.ProviderIDs, []string) {
	ids := entry.IDs
	for _, field := range []struct {
		kind string
		ptr  *string
	}{
		{index.KindCurseForge, &ids.CurseForge},
		{index.KindWago, &ids.Wago},
		{index.KindWoWInterface, &ids.WoWInterface},
	} {
		if *field.ptr == "" {
			continue
		}
		key := [2]string{field.kind, *field.ptr}
		if owner, ok := taken[key]; ok {
			warnings = append(warnings, fmt.Sprintf("%s: %s id %s is already used by %s, dropped", entry.Source, field.kind, *field.ptr, owner))
			*field.ptr = ""
			continue
		}
		taken[key] = entry.ID
	}
	return ids, warnings
}

func entryFor(pkg Package, slug string) (index.Entry, []string, bool) {
	var warnings []string
	warn := func(format string, args ...any) {
		warnings = append(warnings, pkg.Repo+": "+fmt.Sprintf(format, args...))
	}
	skip := func(format string, args ...any) (index.Entry, []string, bool) {
		warn("skipped, "+format, args...)
		return index.Entry{}, warnings, false
	}

	ref, err := provider.ParseReference(pkg.Source)
	if err != nil || ref.Provider != "github" {
		return skip("source %q is not a github reference", pkg.Source)
	}
	entry := index.Entry{ID: slug, Source: provider.FormatReference(ref), ReleaseJSON: pkg.Release != nil && pkg.Release.ReleaseJSON}

	entry.Name = stripUIEscapes(pkg.Title)
	if entry.Name == "" {
		entry.Name = repoNamePart(pkg.Repo)
	}
	if strings.TrimSpace(entry.Name) == "" {
		return skip("no usable name")
	}

	seen := map[string]bool{}
	for _, folder := range pkg.Folders {
		switch {
		case !installer.SafeFolderName(folder):
			warn("folder %q is not a safe folder name, left out", folder)
		case seen[strings.ToLower(folder)]:
		default:
			seen[strings.ToLower(folder)] = true
			entry.Folders = append(entry.Folders, folder)
		}
	}
	slices.SortFunc(entry.Folders, compareFold)
	if len(entry.Folders) == 0 {
		return skip("no safe folder names")
	}

	entry.GameTypes = gameTypesFor(pkg)
	if len(entry.GameTypes) == 0 {
		return skip("no game type from release.json or the TOC interface numbers")
	}

	for _, field := range []struct {
		kind string
		ptr  *string
	}{
		{index.KindCurseForge, &entry.IDs.CurseForge},
		{index.KindWago, &entry.IDs.Wago},
		{index.KindWoWInterface, &entry.IDs.WoWInterface},
	} {
		for i, id := range pkg.IDs[field.kind] {
			switch {
			case !providerID.MatchString(id):
				warn("%s id %q is not usable, left out", field.kind, id)
			case i > 0 || *field.ptr != "":
				warn("%s id %s is not recorded, the entry holds one id per provider", field.kind, id)
			default:
				*field.ptr = id
			}
		}
	}

	funding := parseFunding(pkg.FundingYML)
	for _, line := range funding.Skipped {
		warn("FUNDING.yml %s", line)
	}
	entry.Funding = funding.Links
	slices.SortFunc(entry.Funding, func(a, b index.Funding) int {
		if c := strings.Compare(a.Platform, b.Platform); c != 0 {
			return c
		}
		return strings.Compare(a.Handle, b.Handle)
	})
	return entry, warnings, true
}

// gameTypesFor prefers the flavors a release.json lists and falls back to the
// TOC interface numbers.
func gameTypesFor(pkg Package) []wow.GameType {
	if pkg.Release != nil && len(pkg.Release.ReleaseJSONGameTypes) > 0 {
		return sortGameTypes(pkg.Release.ReleaseJSONGameTypes)
	}
	var types []wow.GameType
	for _, num := range pkg.Interfaces {
		if gameType := wow.GameTypeFromInterface(num); gameType != "" {
			types = append(types, gameType)
		}
	}
	return sortGameTypes(types)
}

func encodeIndex(file index.File) ([]byte, error) {
	return marshalJSON(file)
}

func readPrivateKey(path string) (ed25519.PrivateKey, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("private key %s has permissions %04o, group and others must have no access (chmod 600)", path, info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return index.DecodePrivateKey(string(data))
}

// buildFiles builds, signs and verifies the index in memory and only then
// writes index.json and index.json.sig. load is index.Load in production.
func buildFiles(candPath, keyPath, out string, load func([]byte, []byte, ed25519.PublicKey) (*index.Index, error)) (BuildSummary, error) {
	priv, err := readPrivateKey(keyPath)
	if err != nil {
		return BuildSummary{}, err
	}
	cands, err := readCandidates(candPath)
	if err != nil {
		return BuildSummary{}, err
	}
	res, err := Build(cands)
	if err != nil {
		return BuildSummary{}, err
	}
	data, err := encodeIndex(res.File)
	if err != nil {
		return BuildSummary{}, err
	}
	sig := index.Sign(data, priv)
	pub, _ := priv.Public().(ed25519.PublicKey)
	if _, err := load(data, sig, pub); err != nil {
		return BuildSummary{}, fmt.Errorf("the built index does not verify with its own public key, nothing written: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return BuildSummary{}, err
	}
	if err := writeFileAtomic(out, data, 0o644); err != nil {
		return BuildSummary{}, err
	}
	if err := writeFileAtomic(out+".sig", sig, 0o644); err != nil {
		os.Remove(out)
		return BuildSummary{}, err
	}
	return BuildSummary{Entries: len(res.File.Entries), Warnings: res.Warnings}, nil
}

// keygen writes signing.key (0600) and signing.pub into dir (0700 when it is
// created) and never overwrites either file.
func keygen(dir string) error {
	keyPath := filepath.Join(dir, "signing.key")
	pubPath := filepath.Join(dir, "signing.pub")
	for _, p := range []string{keyPath, pubPath} {
		if _, err := os.Lstat(p); err == nil {
			return fmt.Errorf("%s already exists, refusing to overwrite", p)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	pub, priv, err := index.GenerateKey()
	if err != nil {
		return err
	}
	if err := writeNew(keyPath, []byte(index.EncodePrivateKey(priv)+"\n"), 0o600); err != nil {
		return err
	}
	if err := writeNew(pubPath, []byte(index.EncodePublicKey(pub)+"\n"), 0o644); err != nil {
		os.Remove(keyPath)
		return err
	}
	return nil
}

func writeNew(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return err
	}
	return nil
}
