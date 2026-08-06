package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	Out          io.Writer
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

func (s *Service) Add(ctx context.Context, ref provider.Reference, flavor wow.GameFlavor, install wow.Installation) error {
	p, ok := s.Providers[ref.Provider]
	if !ok {
		return fmt.Errorf("provider %q is not configured", ref.Provider)
	}

	reg, err := registry.Load(s.RegistryPath)
	if err != nil {
		return err
	}
	if reg.FindAddon(ref.ID, install.Path) != nil {
		return fmt.Errorf("%s is already installed in %s", ref.ID, install.Path)
	}

	s.printf("resolving %s", ref.ID)
	remote, err := p.Resolve(ctx, ref)
	if err != nil {
		return err
	}

	s.printf("fetching latest release for flavor %s", flavor)
	release, err := p.LatestRelease(ctx, remote, flavor)
	if err != nil {
		return err
	}
	s.printf("release %s (%s, %s)", release.Tag, release.AssetName, humanSize(release.Size))

	folders, sha, err := s.downloadAndInstall(ctx, p, release, install.Path, ref.Provider+"-"+ref.ID)
	if err != nil {
		return err
	}

	reg.UpsertAddon(registry.Addon{
		Provider:    ref.Provider,
		Ref:         ref.ID,
		Name:        remote.Name,
		Flavor:      string(flavor),
		InstallPath: install.Path,
		Folders:     folders,
		Version:     release.Tag,
		ReleaseID:   release.ID,
		AssetName:   release.AssetName,
		SHA256:      sha,
		InstalledAt: time.Now(),
	})
	if err := reg.Save(s.RegistryPath); err != nil {
		return err
	}
	s.printf("installed %s (%s)", ref.ID, strings.Join(folders, ", "))
	return nil
}

func (s *Service) Check(ctx context.Context) ([]UpdateStatus, error) {
	reg, err := registry.Load(s.RegistryPath)
	if err != nil {
		return nil, err
	}

	statuses := make([]UpdateStatus, 0, len(reg.Addons))
	for _, addon := range reg.Addons {
		status := UpdateStatus{Addon: addon}

		p, ok := s.Providers[addon.Provider]
		if !ok {
			status.Error = fmt.Errorf("provider %q is not configured", addon.Provider)
			statuses = append(statuses, status)
			continue
		}

		remote := provider.RemoteAddon{Provider: addon.Provider, ID: addon.Ref, Name: addon.Name}
		release, err := p.LatestRelease(ctx, remote, wow.GameFlavor(addon.Flavor))
		if err != nil {
			status.Error = err
			statuses = append(statuses, status)
			continue
		}

		status.Latest = release
		status.HasUpdate = release.ID != addon.ReleaseID
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
	return s.updateAddon(ctx, reg, addon, dryRun)
}

func (s *Service) UpdateAll(ctx context.Context, dryRun bool) error {
	reg, err := registry.Load(s.RegistryPath)
	if err != nil {
		return err
	}
	for i := range reg.Addons {
		if err := s.updateAddon(ctx, reg, &reg.Addons[i], dryRun); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) updateAddon(ctx context.Context, reg *registry.Registry, addon *registry.Addon, dryRun bool) error {
	p, ok := s.Providers[addon.Provider]
	if !ok {
		return fmt.Errorf("provider %q is not configured", addon.Provider)
	}

	remote := provider.RemoteAddon{Provider: addon.Provider, ID: addon.Ref, Name: addon.Name}
	release, err := p.LatestRelease(ctx, remote, wow.GameFlavor(addon.Flavor))
	if err != nil {
		return err
	}

	if release.ID == addon.ReleaseID {
		s.printf("%s is up to date (%s)", addon.Ref, addon.Version)
		return nil
	}
	if dryRun {
		s.printf("%s: %s -> %s (dry run)", addon.Ref, addon.Version, release.Tag)
		return nil
	}

	s.printf("updating %s: %s -> %s", addon.Ref, addon.Version, release.Tag)
	folders, sha, err := s.downloadAndInstall(ctx, p, release, addon.InstallPath, addon.Provider+"-"+addon.Ref)
	if err != nil {
		return err
	}

	addon.Folders = folders
	addon.Version = release.Tag
	addon.ReleaseID = release.ID
	addon.AssetName = release.AssetName
	addon.SHA256 = sha
	addon.InstalledAt = time.Now()
	return reg.Save(s.RegistryPath)
}

func (s *Service) downloadAndInstall(ctx context.Context, p provider.Provider, release provider.Release, installPath, label string) ([]string, string, error) {
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
	s.printf("downloaded %s (sha256 %s)", humanSize(size), sha[:12])

	staging := filepath.Join(workDir, "staging")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return nil, "", err
	}
	if err := installer.ExtractZip(zipPath, staging, extractLimits); err != nil {
		return nil, "", err
	}

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
		s.printf("backup: %s", result.BackupPath)
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

func (s *Service) printf(format string, args ...any) {
	if s.Out != nil {
		fmt.Fprintf(s.Out, format+"\n", args...)
	}
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
