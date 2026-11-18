package service

import (
	"cmp"
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/addon"
	"somegit.dev/vikingowl/wowtui/internal/index"
	"somegit.dev/vikingowl/wowtui/internal/installer"
	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/registry"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

type Service struct {
	Providers    map[string]provider.Provider
	RegistryPath string
	BackupRoot   string
	MaxDownload  int64
	OnEvent      EmitFunc
	// Index is the verified addon index, nil when none is loaded.
	Index *index.Index
}

type UpdateStatus struct {
	Addon     registry.Addon
	Latest    provider.Release
	HasUpdate bool
	Error     error
}

type MappingCandidate struct {
	InstallPath  string
	Flavor       string
	Folder       string
	Version      string
	Provider     string
	Reference    string
	Reason       string
	Availability Availability
}

type Availability string

const (
	AvailabilityActive      Availability = "active"
	AvailabilityPaused      Availability = "paused"
	AvailabilityNeedsAPIKey Availability = "needs-api-key"
	AvailabilityDisabled    Availability = "disabled"
	AvailabilityUnsupported Availability = "unsupported"
)

var extractLimits = installer.Limits{
	MaxFiles:     20000,
	MaxTotalSize: 512 << 20,
	MaxFileSize:  256 << 20,
}

const defaultMaxDownload int64 = 512 << 20

func (s *Service) Scan(ctx context.Context, configPaths []string) ([]registry.Installation, error) {
	installs, err := wow.Discover(configPaths)
	if err != nil {
		return nil, err
	}

	reg, err := registry.Load(s.RegistryPath)
	if err != nil {
		return nil, err
	}
	oldByPath := map[string]registry.Installation{}
	for _, inst := range reg.Installations {
		oldByPath[inst.Path] = inst
	}
	reg.Version = 1
	reg.Installations = nil

	for _, inst := range installs {
		folders, err := addon.Scan(inst.Path)
		if err != nil {
			return nil, err
		}
		flavor := inst.Flavor
		if flavor == "" {
			flavor = inferFlavor(folders)
		}

		rec := registry.Installation{Path: inst.Path, Flavor: string(flavor)}
		if old, ok := oldByPath[inst.Path]; ok {
			rec.Title = old.Title
		}
		for _, f := range folders {
			rec.Folders = append(rec.Folders, registry.Folder{
				Name:       f.Name,
				Version:    f.Version(),
				Interfaces: f.Interfaces(),
				TocFiles:   tocNames(f.Tocs),
				Hints:      f.Hints(),
			})
		}
		reg.Installations = append(reg.Installations, rec)

		label := string(flavor)
		if label == "" {
			label = "unknown"
		}
		s.emit(Event{Stage: StageInfo, Message: fmt.Sprintf("%s  %s  %d addons", label, inst.Path, len(folders))})
	}

	if err := reg.Save(s.RegistryPath); err != nil {
		return nil, err
	}
	return reg.Installations, nil
}

func inferFlavor(folders []*addon.Folder) wow.GameFlavor {
	votes := map[wow.GameFlavor]int{}
	for _, f := range folders {
		var folderType wow.GameType
		ambiguous := false
		for _, num := range f.Interfaces() {
			gameType := wow.GameTypeFromInterface(num)
			if gameType == "" {
				continue
			}
			if folderType != "" && folderType != gameType {
				ambiguous = true
				break
			}
			folderType = gameType
		}
		if ambiguous {
			continue
		}
		if flavor := wow.FlavorForGameType(folderType); flavor != "" {
			votes[flavor]++
		}
	}

	var best wow.GameFlavor
	bestVotes, tied := 0, false
	for flavor, n := range votes {
		switch {
		case n > bestVotes:
			best, bestVotes, tied = flavor, n, false
		case n == bestVotes:
			tied = true
		}
	}
	if tied {
		return ""
	}
	return best
}

func tocNames(tocs []*addon.Toc) []string {
	names := make([]string, 0, len(tocs))
	for _, toc := range tocs {
		names = append(names, filepath.Base(toc.Path))
	}
	return names
}

func (s *Service) SetInstallTitle(path, title string) error {
	reg, err := registry.Load(s.RegistryPath)
	if err != nil {
		return err
	}
	for i := range reg.Installations {
		if reg.Installations[i].Path == path {
			reg.Installations[i].Title = title
			return reg.Save(s.RegistryPath)
		}
	}
	return fmt.Errorf("installation %s not found in registry", path)
}

// MapUnmanaged marks a candidate active when s.Providers has its provider.
// inactive gives the availability of any other provider; nil marks them
// unsupported.
func (s *Service) MapUnmanaged(inactive func(provider string) Availability) ([]MappingCandidate, error) {
	reg, err := registry.Load(s.RegistryPath)
	if err != nil {
		return nil, err
	}
	var candidates []MappingCandidate
	for _, installation := range reg.Installations {
		for _, folder := range installation.Folders {
			if folderOwner(reg, installation.Path, "", folder.Name) != nil {
				continue
			}
			for _, candidate := range mapFolderHints(installation, folder, s.Index) {
				candidate.Availability = s.mappingAvailability(candidate.Provider, inactive)
				candidates = append(candidates, candidate)
			}
		}
	}
	slices.SortFunc(candidates, func(a, b MappingCandidate) int {
		return cmp.Or(
			cmp.Compare(a.InstallPath, b.InstallPath),
			cmp.Compare(a.Folder, b.Folder),
			cmp.Compare(indexRank(a), indexRank(b)),
			cmp.Compare(a.Provider, b.Provider),
			cmp.Compare(a.Reference, b.Reference),
		)
	})
	return candidates, nil
}

func (s *Service) mappingAvailability(name string, inactive func(string) Availability) Availability {
	if _, ok := s.Providers[name]; ok {
		return AvailabilityActive
	}
	if inactive == nil {
		return AvailabilityUnsupported
	}
	return inactive(name)
}

func (s *Service) Adopt(ctx context.Context, installPath, folderName string, ref provider.Reference) error {
	reg, err := registry.Load(s.RegistryPath)
	if err != nil {
		return err
	}
	var installation registry.Installation
	var folder registry.Folder
	found := false
	for _, item := range reg.Installations {
		if item.Path != installPath {
			continue
		}
		installation = item
		for _, candidate := range item.Folders {
			if candidate.Name == folderName {
				folder = candidate
				found = true
				break
			}
		}
	}
	if !found {
		return fmt.Errorf("unmanaged folder %q not found in %s", folderName, installPath)
	}
	if owner := folderOwner(reg, installPath, ref.ID, folder.Name); owner != nil {
		return fmt.Errorf("refusing to adopt %s in %s: folder %s is managed by %s", ref.ID, installPath, folder.Name, owner.Ref)
	}
	p, ok := s.Providers[ref.Provider]
	if !ok {
		return providerUnavailable(ref.Provider)
	}
	remote, err := p.Resolve(ctx, ref)
	if err != nil {
		return err
	}
	if reg.FindAddon(ref.ID, installPath) != nil {
		return fmt.Errorf("%s is already managed in %s", ref.ID, installPath)
	}
	reg.UpsertAddon(registry.Addon{
		Provider:    ref.Provider,
		Ref:         ref.ID,
		Name:        remote.Name,
		Author:      remote.Author,
		Flavor:      installation.Flavor,
		InstallPath: installPath,
		Folders:     []string{folder.Name},
		Version:     folder.Version,
		Source:      "auto",
		InstalledAt: time.Now(),
	})
	return reg.Save(s.RegistryPath)
}

// indexRank sorts index candidates first. Index reasons are added before any
// hint reason, so an index candidate's reason starts with one.
func indexRank(c MappingCandidate) int {
	if strings.HasPrefix(c.Reason, "index:") {
		return 0
	}
	return 1
}

func mapFolderHints(installation registry.Installation, folder registry.Folder, idx *index.Index) []MappingCandidate {
	var candidates []MappingCandidate
	add := func(providerName, ref, reason string) {
		for i := range candidates {
			if candidates[i].Provider == providerName && candidates[i].Reference == ref {
				if !slices.Contains(strings.Split(candidates[i].Reason, ","), reason) {
					candidates[i].Reason += "," + reason
				}
				return
			}
		}
		candidates = append(candidates, MappingCandidate{InstallPath: installation.Path, Flavor: installation.Flavor, Folder: folder.Name, Version: folder.Version, Provider: providerName, Reference: ref, Reason: reason})
	}
	for _, match := range indexMatches(idx, folder) {
		ref, err := provider.ParseReference(match.entry.Source)
		if err != nil {
			continue
		}
		for _, reason := range match.reasons {
			add(ref.Provider, provider.FormatReference(ref), reason)
		}
	}
	for _, key := range []string{"x-source-repo", "x-website"} {
		if value := folder.Hints[key]; value != "" {
			if ref, ok := hintReference(key, value); ok {
				add(ref.Provider, provider.FormatReference(ref), key)
			}
		}
	}
	if id := folder.Hints["x-wowi-id"]; id != "" {
		add("wowinterface", "wowinterface:"+id, "x-wowi-id")
	}
	if id := folder.Hints["x-wago-id"]; id != "" {
		add("wago", "wago:"+id, "x-wago-id")
	}
	if id := folder.Hints["x-curse-project-id"]; id != "" {
		add("curseforge", "curseforge:"+id, "x-curse-project-id")
	}
	return candidates
}

type indexMatch struct {
	entry   index.Entry
	reasons []string
}

// indexMatches returns the index entries that install folder or carry one of
// its provider ID hints, in the order first matched, each with its match
// reasons and "unreviewed" when no human reviewed the entry.
func indexMatches(idx *index.Index, folder registry.Folder) []indexMatch {
	if idx == nil {
		return nil
	}
	var matches []indexMatch
	add := func(entry index.Entry, reason string) {
		for i := range matches {
			if matches[i].entry.ID == entry.ID {
				matches[i].reasons = append(matches[i].reasons, reason)
				return
			}
		}
		matches = append(matches, indexMatch{entry: entry, reasons: []string{reason}})
	}
	for _, entry := range idx.ByFolder(folder.Name) {
		add(entry, "index:folder")
	}
	for _, hint := range []struct{ key, kind string }{
		{"x-curse-project-id", index.KindCurseForge},
		{"x-wago-id", index.KindWago},
		{"x-wowi-id", index.KindWoWInterface},
	} {
		if id := folder.Hints[hint.key]; id != "" {
			if entry, ok := idx.ByProviderID(hint.kind, id); ok {
				add(entry, "index:"+hint.kind)
			}
		}
	}
	for i := range matches {
		if !matches[i].entry.Reviewed {
			matches[i].reasons = append(matches[i].reasons, "unreviewed")
		}
	}
	return matches
}

func hintReference(key, value string) (provider.Reference, bool) {
	value = strings.TrimSpace(value)
	ref, err := provider.ParseReference(value)
	if err == nil {
		switch ref.Provider {
		case "github", "gitlab", "wowuphub":
			return ref, true
		case "curseforge":
			_, err := provider.ParseReference("curseforge:" + ref.ID)
			return ref, err == nil
		}
	}
	if key != "x-source-repo" {
		return provider.Reference{}, false
	}
	gitURL := value
	if err == nil {
		gitURL = ref.ID
	} else if u, parseErr := url.Parse(value); parseErr != nil || (u.Scheme != "ssh" && u.Scheme != "git") {
		// ParseReference has no syntax for bare ssh and git URLs. Any other
		// URL it rejects is malformed, plain http, or names no addon on a
		// catalog host.
		return provider.Reference{}, false
	}
	return gitRepositoryReference(gitURL)
}

func gitRepositoryReference(rawURL string) (provider.Reference, bool) {
	ref, err := provider.ParseReference("git:" + rawURL)
	if err != nil {
		return provider.Reference{}, false
	}
	u, err := url.Parse(ref.ID)
	if err != nil || len(strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })) < 2 {
		return provider.Reference{}, false
	}
	return ref, true
}

func (s *Service) Add(ctx context.Context, ref provider.Reference, flavor wow.GameFlavor, install wow.Installation, source string) error {
	p, ok := s.Providers[ref.Provider]
	if !ok {
		return providerUnavailable(ref.Provider)
	}

	reg, err := registry.Load(s.RegistryPath)
	if err != nil {
		return err
	}
	if reg.FindAddon(ref.ID, install.Path) != nil {
		return fmt.Errorf("%s is already installed in %s", ref.ID, install.Path)
	}

	s.emit(Event{Ref: ref.ID, Stage: StageResolve, Message: "resolving " + ref.ID})
	remote, err := p.Resolve(ctx, ref)
	if err != nil {
		return err
	}

	s.emit(Event{Ref: ref.ID, Stage: StageResolve, Message: fmt.Sprintf("fetching latest version for flavor %s", flavor)})
	release, err := p.Latest(ctx, remote, flavor, source)
	if err != nil {
		return err
	}
	s.emit(Event{Ref: ref.ID, Stage: StageInfo, Message: fmt.Sprintf("release %s (%s, %s)", versionLabel(release), release.AssetName, humanSize(release.Size))})

	folders, sha, err := s.downloadAndInstall(ctx, p, release, reg, install.Path, ref.Provider+"-"+ref.ID, ref.ID, nil, 0, 0)
	if err != nil {
		return err
	}

	pin := source
	if pin == "" {
		pin = "auto"
	}
	reg.UpsertAddon(registry.Addon{
		Provider:    ref.Provider,
		Ref:         ref.ID,
		Name:        remote.Name,
		Author:      remote.Author,
		Flavor:      string(flavor),
		InstallPath: install.Path,
		Folders:     folders,
		Version:     versionFor(release),
		Source:      pin,
		ReleaseID:   releaseIDFor(release),
		AssetName:   release.AssetName,
		SHA256:      sha,
		InstalledAt: time.Now(),
	})
	if err := reg.Save(s.RegistryPath); err != nil {
		return err
	}
	s.emit(Event{Ref: ref.ID, Stage: StageDone, Message: fmt.Sprintf("installed %s (%s)", ref.ID, strings.Join(folders, ", "))})
	return nil
}

func (s *Service) Check(ctx context.Context) ([]UpdateStatus, error) {
	reg, err := registry.Load(s.RegistryPath)
	if err != nil {
		return nil, err
	}

	statuses := make([]UpdateStatus, 0, len(reg.Addons))
	total := len(reg.Addons)
	for i, addon := range reg.Addons {
		status := UpdateStatus{Addon: addon}
		s.emit(Event{Ref: addon.Ref, Stage: StageResolve, Message: "checking " + addon.Ref, Index: i + 1, Total: total})

		p, ok := s.Providers[addon.Provider]
		if !ok {
			status.Error = providerUnavailable(addon.Provider)
			statuses = append(statuses, status)
			continue
		}

		remote := provider.RemoteAddon{Provider: addon.Provider, ID: addon.Ref, Name: addon.Name}
		release, err := p.Latest(ctx, remote, wow.GameFlavor(addon.Flavor), addon.Source)
		if err != nil {
			status.Error = err
			statuses = append(statuses, status)
			continue
		}

		status.Latest = release
		status.HasUpdate = !sameVersion(addon, release)
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func (s *Service) Update(ctx context.Context, refOrName string, dryRun bool) error {
	reg, err := registry.Load(s.RegistryPath)
	if err != nil {
		return err
	}
	addon, err := findAddon(reg, refOrName)
	if err != nil {
		return err
	}
	return s.updateAddon(ctx, reg, addon, dryRun, 0, 0)
}

func (s *Service) UpdateAll(ctx context.Context, dryRun bool) error {
	reg, err := registry.Load(s.RegistryPath)
	if err != nil {
		return err
	}
	total := len(reg.Addons)
	var failures updateFailures
	for i := range reg.Addons {
		addon := &reg.Addons[i]
		if err := s.updateAddon(ctx, reg, addon, dryRun, i+1, total); err != nil {
			s.emit(Event{Ref: addon.Ref, Stage: StageError, Message: fmt.Sprintf("failed to update %s: %v", addon.Ref, err), Index: i + 1, Total: total, Err: err})
			failures = append(failures, fmt.Errorf("%s: %w", addon.Ref, err))
		}
	}
	if len(failures) > 0 {
		return failures
	}
	return nil
}

type updateFailures []error

func (f updateFailures) Error() string {
	msgs := make([]string, 0, len(f))
	for _, err := range f {
		msgs = append(msgs, err.Error())
	}
	return "update failed: " + strings.Join(msgs, "; ")
}

func (f updateFailures) Unwrap() []error { return f }

func (s *Service) updateAddon(ctx context.Context, reg *registry.Registry, addon *registry.Addon, dryRun bool, index, total int) error {
	ev := func(stage Stage, msg string) {
		s.emit(Event{Ref: addon.Ref, Stage: stage, Message: msg, Index: index, Total: total})
	}

	p, ok := s.Providers[addon.Provider]
	if !ok {
		return providerUnavailable(addon.Provider)
	}

	remote := provider.RemoteAddon{Provider: addon.Provider, ID: addon.Ref, Name: addon.Name}
	ev(StageResolve, "resolving "+addon.Ref)
	release, err := p.Latest(ctx, remote, wow.GameFlavor(addon.Flavor), addon.Source)
	if err != nil {
		return err
	}

	if sameVersion(*addon, release) {
		ev(StageCurrent, fmt.Sprintf("%s is up to date (%s)", addon.Ref, addon.Version))
		return nil
	}
	if dryRun {
		ev(StageInfo, fmt.Sprintf("%s: %s -> %s (dry run)", addon.Ref, addon.Version, versionLabel(release)))
		return nil
	}

	ev(StageInstall, fmt.Sprintf("updating %s: %s -> %s", addon.Ref, addon.Version, versionLabel(release)))
	folders, sha, err := s.downloadAndInstall(ctx, p, release, reg, addon.InstallPath, addon.Provider+"-"+addon.Ref, addon.Ref, addon.Folders, index, total)
	if err != nil {
		return err
	}

	addon.Folders = folders
	addon.Version = versionFor(release)
	addon.ReleaseID = releaseIDFor(release)
	addon.AssetName = release.AssetName
	addon.SHA256 = sha
	addon.InstalledAt = time.Now()
	return reg.Save(s.RegistryPath)
}

func sameVersion(addon registry.Addon, release provider.Release) bool {
	switch release.Kind {
	case provider.KindRelease:
		if addon.Source == "tag" || addon.Source == "branch" {
			return false
		}
		if addon.ReleaseID != 0 && release.ID != 0 {
			return addon.ReleaseID == release.ID
		}
		return addon.Version != "" && strings.EqualFold(addon.Version, release.Tag)
	case provider.KindBranch:
		return addon.Version != "" && addon.Version == release.Commit
	default:
		return addon.Version != "" && strings.EqualFold(addon.Version, release.Tag)
	}
}

func versionFor(release provider.Release) string {
	if release.Kind == provider.KindBranch {
		return release.Commit
	}
	return release.Tag
}

func releaseIDFor(release provider.Release) int64 {
	if release.Kind == provider.KindRelease {
		return release.ID
	}
	return 0
}

func versionLabel(release provider.Release) string {
	if release.Kind == provider.KindBranch && release.Commit != "" {
		return release.Tag + "@" + shortHash(release.Commit)
	}
	if release.Kind == provider.KindDirect && len(release.Tag) == 64 {
		return shortHash(release.Tag)
	}
	return release.Tag
}

func shortHash(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func (s *Service) downloadAndInstall(ctx context.Context, p provider.Provider, release provider.Release, reg *registry.Registry, installPath, label, ref string, previous []string, index, total int) ([]string, string, error) {
	ev := func(stage Stage, msg string) {
		s.emit(Event{Ref: ref, Stage: stage, Message: msg, Index: index, Total: total})
	}

	ev(StageDownload, "downloading "+release.AssetName)
	body, err := p.Download(ctx, release)
	if err != nil {
		return nil, "", err
	}
	defer body.Close()

	maxDownload := s.MaxDownload
	if maxDownload == 0 {
		maxDownload = defaultMaxDownload
	}

	addonsDir := filepath.Join(installPath, "Interface", "AddOns")
	if err := os.MkdirAll(addonsDir, 0o755); err != nil {
		return nil, "", err
	}
	workDir, err := os.MkdirTemp(filepath.Dir(addonsDir), ".wowtui-work-")
	if err != nil {
		return nil, "", err
	}
	defer os.RemoveAll(workDir)

	zipPath := filepath.Join(workDir, "archive.zip")
	sha, size, err := installer.WriteArchive(body, zipPath, maxDownload)
	if err != nil {
		return nil, "", err
	}
	ev(StageDownload, fmt.Sprintf("downloaded %s (sha256 %s)", humanSize(size), sha[:12]))

	ev(StageExtract, "extracting")
	staging := filepath.Join(workDir, "staging")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return nil, "", err
	}
	if err := installer.ExtractZip(zipPath, staging, extractLimits); err != nil {
		return nil, "", err
	}

	roots, err := installer.FindAddonRoots(staging)
	if err != nil {
		return nil, "", err
	}
	var conflicts []string
	for _, root := range roots {
		if containsFold(previous, root.Name) {
			continue
		}
		if owner := folderOwner(reg, installPath, ref, root.Name); owner != nil {
			conflicts = append(conflicts, fmt.Sprintf("folder %s is managed by %s", root.Name, owner.Ref))
		}
	}
	if len(conflicts) > 0 {
		return nil, "", fmt.Errorf("refusing to install %s in %s: %s", ref, installPath, strings.Join(conflicts, "; "))
	}
	if release.Kind == provider.KindTag || release.Kind == provider.KindBranch {
		for _, msg := range packagerWarnings(staging, roots) {
			ev(StageWarning, fmt.Sprintf("warning: %s: %s", ref, msg))
		}
	}

	var dropped []string
	for _, name := range previous {
		shipped := false
		for _, root := range roots {
			if strings.EqualFold(root.Name, name) {
				shipped = true
				break
			}
		}
		if !shipped && folderOwner(reg, installPath, ref, name) == nil {
			dropped = append(dropped, name)
		}
	}

	ev(StageInstall, "installing")
	result, err := installer.Install(installer.InstallPlan{
		AddonsDir:  addonsDir,
		StagingDir: staging,
		BackupRoot: s.BackupRoot,
		Label:      label,
		Roots:      roots,
		Remove:     dropped,
	})
	if err != nil {
		return nil, "", err
	}
	if result.BackupPath != "" {
		ev(StageBackup, "backup: "+result.BackupPath)
	}
	var unmanaged []string
	for _, name := range result.Replaced {
		if !containsFold(previous, name) {
			unmanaged = append(unmanaged, name)
		}
	}
	if len(unmanaged) > 0 {
		ev(StageWarning, fmt.Sprintf("warning: %s replaced unmanaged folders %s (backup: %s)", ref, strings.Join(unmanaged, ", "), result.BackupPath))
	}
	if len(result.Removed) > 0 {
		ev(StageInfo, fmt.Sprintf("%s removed folders no longer shipped: %s (backup: %s)", ref, strings.Join(result.Removed, ", "), result.BackupPath))
	}
	return result.Folders, sha, nil
}

func folderOwner(reg *registry.Registry, installPath, ref, folder string) *registry.Addon {
	for i := range reg.Addons {
		a := &reg.Addons[i]
		if a.InstallPath != installPath || a.Ref == ref {
			continue
		}
		if containsFold(a.Folders, folder) {
			return a
		}
	}
	return nil
}

func containsFold(names []string, name string) bool {
	for _, n := range names {
		if strings.EqualFold(n, name) {
			return true
		}
	}
	return false
}

func findAddon(reg *registry.Registry, refOrName string) (*registry.Addon, error) {
	var matches []*registry.Addon
	for i := range reg.Addons {
		addon := &reg.Addons[i]
		if addon.Ref == refOrName {
			return addon, nil
		}
		short := addon.Ref
		if idx := strings.LastIndex(addon.Ref, "/"); idx >= 0 {
			short = addon.Ref[idx+1:]
		}
		if strings.EqualFold(short, refOrName) {
			matches = append(matches, addon)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		names := make([]string, 0, len(matches))
		for _, m := range matches {
			names = append(names, m.Ref)
		}
		return nil, fmt.Errorf("ambiguous addon %q, matches: %s", refOrName, strings.Join(names, ", "))
	}
	return nil, fmt.Errorf("addon %q not found in registry", refOrName)
}

func (s *Service) emit(e Event) {
	if s.OnEvent != nil {
		s.OnEvent(e)
	}
}

func providerUnavailable(name string) error {
	if name == "wowuphub" {
		return fmt.Errorf("provider %q is paused pending upstream approval", name)
	}
	if name == "curseforge" {
		return fmt.Errorf("provider %q requires WOWTUI_CURSEFORGE_API_KEY or providers.curseforge.api_key", name)
	}
	if name == "wago" {
		return fmt.Errorf("provider %q is paused pending approval", name)
	}
	return fmt.Errorf("provider %q is disabled or not configured; enable it with: wowtui providers enable %s", name, name)
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
