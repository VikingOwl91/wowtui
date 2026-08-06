package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/addon"
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
}

type UpdateStatus struct {
	Addon     registry.Addon
	Latest    provider.Release
	HasUpdate bool
	Error     error
}

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
	for _, f := range folders {
		for _, num := range f.Interfaces() {
			if flavor := wow.FlavorFromInterface(num); flavor != "" {
				return flavor
			}
		}
	}
	return ""
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

	folders, sha, err := s.downloadAndInstall(ctx, p, release, install.Path, ref.Provider+"-"+ref.ID, ref.ID, 0, 0)
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
	for i := range reg.Addons {
		if err := s.updateAddon(ctx, reg, &reg.Addons[i], dryRun, i+1, total); err != nil {
			return err
		}
	}
	return nil
}

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
	folders, sha, err := s.downloadAndInstall(ctx, p, release, addon.InstallPath, addon.Provider+"-"+addon.Ref, addon.Ref, index, total)
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

func (s *Service) downloadAndInstall(ctx context.Context, p provider.Provider, release provider.Release, installPath, label, ref string, index, total int) ([]string, string, error) {
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

	workDir, err := os.MkdirTemp("", "wowtui-")
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

	ev(StageInstall, "installing")
	addonsDir := filepath.Join(installPath, "Interface", "AddOns")
	if err := os.MkdirAll(addonsDir, 0o755); err != nil {
		return nil, "", err
	}
	result, err := installer.Install(installer.InstallPlan{
		AddonsDir:  addonsDir,
		StagingDir: staging,
		BackupRoot: s.BackupRoot,
		Label:      label,
	})
	if err != nil {
		return nil, "", err
	}
	if result.BackupPath != "" {
		ev(StageBackup, "backup: "+result.BackupPath)
	}
	return result.Folders, sha, nil
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
