package service

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/registry"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

type fakeProvider struct {
	release    provider.Release
	zipContent []byte
	resolveErr error
	releaseErr error
	downloads  int
	lastSource string
}

func (f *fakeProvider) Resolve(_ context.Context, ref provider.Reference) (provider.RemoteAddon, error) {
	if f.resolveErr != nil {
		return provider.RemoteAddon{}, f.resolveErr
	}
	return provider.RemoteAddon{Provider: ref.Provider, ID: ref.ID, Name: ref.ID}, nil
}

func (f *fakeProvider) Latest(_ context.Context, _ provider.RemoteAddon, _ wow.GameFlavor, source string) (provider.Release, error) {
	f.lastSource = source
	if f.releaseErr != nil {
		return provider.Release{}, f.releaseErr
	}
	return f.release, nil
}

func (f *fakeProvider) Download(_ context.Context, _ provider.Release) (io.ReadCloser, error) {
	f.downloads++
	return io.NopCloser(bytes.NewReader(f.zipContent)), nil
}

func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newTestService(t *testing.T, fp *fakeProvider) (*Service, wow.Installation, *captured) {
	t.Helper()
	base := t.TempDir()
	install := wow.Installation{
		Path:   filepath.Join(base, "_retail_"),
		Flavor: wow.Retail,
	}
	if err := os.MkdirAll(filepath.Join(install.Path, "Interface", "AddOns"), 0o755); err != nil {
		t.Fatal(err)
	}

	out := &captured{}
	svc := &Service{
		Providers:    map[string]provider.Provider{"github": fp},
		RegistryPath: filepath.Join(base, "registry.json"),
		BackupRoot:   filepath.Join(base, "backups"),
		OnEvent:      out.emit,
	}
	return svc, install, out
}

type captured struct {
	events []Event
	text   strings.Builder
}

func (c *captured) emit(e Event) {
	c.events = append(c.events, e)
	if e.Message != "" {
		c.text.WriteString(e.Message + "\n")
	}
}

func (c *captured) String() string { return c.text.String() }

func (c *captured) firstIndex(stage Stage) int {
	for i, e := range c.events {
		if e.Stage == stage {
			return i
		}
	}
	return -1
}

func addonZip(t *testing.T, version string) []byte {
	return buildZip(t, map[string]string{
		"MyAddon/MyAddon.toc": "## Version: " + version,
		"MyAddon/core.lua":    "local x = 1",
	})
}

func TestAddInstallsAddonAndWritesRegistry(t *testing.T) {
	fp := &fakeProvider{
		release:    provider.Release{Kind: provider.KindRelease, ID: 100, Tag: "v1.0.0", AssetName: "MyAddon-1.0.0.zip", DownloadURL: "http://fake/zip", Size: 10},
		zipContent: addonZip(t, "1.0.0"),
	}
	svc, install, out := newTestService(t, fp)

	ref := provider.Reference{Provider: "github", ID: "Owner/MyAddon"}
	if err := svc.Add(context.Background(), ref, wow.Retail, install, ""); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(install.Path, "Interface", "AddOns", "MyAddon", "MyAddon.toc"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "## Version: 1.0.0" {
		t.Errorf("installed toc = %q", content)
	}

	reg, err := registry.Load(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	addon := reg.FindAddon("Owner/MyAddon", install.Path)
	if addon == nil {
		t.Fatal("registry entry missing")
	}
	if addon.ReleaseID != 100 || addon.Version != "v1.0.0" || addon.SHA256 == "" {
		t.Errorf("registry entry = %+v", addon)
	}
	if addon.Source != "auto" {
		t.Errorf("source = %q, want auto", addon.Source)
	}
	if len(addon.Folders) != 1 || addon.Folders[0] != "MyAddon" {
		t.Errorf("folders = %v", addon.Folders)
	}
	if !strings.Contains(out.String(), "v1.0.0") {
		t.Errorf("output missing release info: %s", out.String())
	}
}

func TestAddPinsSource(t *testing.T) {
	fp := &fakeProvider{
		release:    provider.Release{Kind: provider.KindTag, Tag: "v1.0.0", AssetName: "a.zip"},
		zipContent: addonZip(t, "1.0.0"),
	}
	svc, install, _ := newTestService(t, fp)
	ref := provider.Reference{Provider: "github", ID: "Owner/MyAddon"}

	if err := svc.Add(context.Background(), ref, wow.Retail, install, "tag"); err != nil {
		t.Fatal(err)
	}
	if fp.lastSource != "tag" {
		t.Errorf("provider source = %q, want tag", fp.lastSource)
	}

	reg, err := registry.Load(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	addon := reg.FindAddon("Owner/MyAddon", install.Path)
	if addon == nil || addon.Source != "tag" {
		t.Errorf("registry entry = %+v, want source tag", addon)
	}
	if addon.ReleaseID != 0 {
		t.Errorf("tag install must not store release id, got %d", addon.ReleaseID)
	}
}

func TestAddRejectsDuplicate(t *testing.T) {
	fp := &fakeProvider{
		release:    provider.Release{Kind: provider.KindRelease, ID: 100, Tag: "v1.0.0", AssetName: "a.zip"},
		zipContent: addonZip(t, "1.0.0"),
	}
	svc, install, _ := newTestService(t, fp)
	ref := provider.Reference{Provider: "github", ID: "Owner/MyAddon"}

	if err := svc.Add(context.Background(), ref, wow.Retail, install, ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.Add(context.Background(), ref, wow.Retail, install, ""); err == nil {
		t.Fatal("expected error for duplicate addon")
	}
}

func TestAddUnknownProvider(t *testing.T) {
	svc, install, _ := newTestService(t, &fakeProvider{})
	ref := provider.Reference{Provider: "direct", ID: "https://example.com/x.zip"}
	if err := svc.Add(context.Background(), ref, wow.Retail, install, ""); err == nil {
		t.Fatal("expected error for unconfigured provider")
	}
}

func TestAddResolveError(t *testing.T) {
	fp := &fakeProvider{resolveErr: errors.New("not found")}
	svc, install, _ := newTestService(t, fp)
	ref := provider.Reference{Provider: "github", ID: "Owner/Missing"}
	if err := svc.Add(context.Background(), ref, wow.Retail, install, ""); err == nil {
		t.Fatal("expected resolve error to surface")
	}
}

func seedAddon(t *testing.T, svc *Service, install wow.Installation, releaseID int64, version string) {
	t.Helper()
	reg := &registry.Registry{Version: 1}
	reg.UpsertAddon(registry.Addon{
		Provider:    "github",
		Ref:         "Owner/MyAddon",
		Name:        "Owner/MyAddon",
		Flavor:      "retail",
		InstallPath: install.Path,
		Folders:     []string{"MyAddon"},
		Version:     version,
		Source:      "auto",
		ReleaseID:   releaseID,
		InstalledAt: time.Now(),
	})
	if err := reg.Save(svc.RegistryPath); err != nil {
		t.Fatal(err)
	}
}

func TestCheckDetectsUpdate(t *testing.T) {
	fp := &fakeProvider{release: provider.Release{Kind: provider.KindRelease, ID: 200, Tag: "v2.0.0", AssetName: "a.zip"}}
	svc, install, _ := newTestService(t, fp)
	seedAddon(t, svc, install, 100, "v1.0.0")

	statuses, err := svc.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || !statuses[0].HasUpdate {
		t.Fatalf("statuses = %+v", statuses)
	}
	if statuses[0].Latest.Tag != "v2.0.0" {
		t.Errorf("latest = %+v", statuses[0].Latest)
	}
	if fp.lastSource != "auto" {
		t.Errorf("provider source = %q, want auto", fp.lastSource)
	}
}

func TestCheckNoUpdate(t *testing.T) {
	fp := &fakeProvider{release: provider.Release{Kind: provider.KindRelease, ID: 100, Tag: "v1.0.0", AssetName: "a.zip"}}
	svc, install, _ := newTestService(t, fp)
	seedAddon(t, svc, install, 100, "v1.0.0")

	statuses, err := svc.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || statuses[0].HasUpdate {
		t.Fatalf("statuses = %+v", statuses)
	}
}

func TestCheckTagBased(t *testing.T) {
	fp := &fakeProvider{release: provider.Release{Kind: provider.KindTag, Tag: "v1.1.0", AssetName: "a.zip"}}
	svc, install, _ := newTestService(t, fp)

	reg := &registry.Registry{Version: 1}
	reg.UpsertAddon(registry.Addon{
		Provider: "github", Ref: "Owner/MyAddon", Flavor: "retail",
		InstallPath: install.Path, Version: "v1.0.0", Source: "tag",
	})
	if err := reg.Save(svc.RegistryPath); err != nil {
		t.Fatal(err)
	}

	statuses, err := svc.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || !statuses[0].HasUpdate {
		t.Fatalf("statuses = %+v", statuses)
	}
	if fp.lastSource != "tag" {
		t.Errorf("provider source = %q, want pinned tag", fp.lastSource)
	}
}

func TestCheckBranchBasedSameCommitIsCurrent(t *testing.T) {
	sha := "abc123def456abc123def456abc123def456abcd"
	fp := &fakeProvider{release: provider.Release{Kind: provider.KindBranch, Tag: "main", Commit: sha, AssetName: "a.zip"}}
	svc, install, _ := newTestService(t, fp)

	reg := &registry.Registry{Version: 1}
	reg.UpsertAddon(registry.Addon{
		Provider: "github", Ref: "Owner/MyAddon", Flavor: "retail",
		InstallPath: install.Path, Version: sha, Source: "branch",
	})
	if err := reg.Save(svc.RegistryPath); err != nil {
		t.Fatal(err)
	}

	statuses, err := svc.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || statuses[0].HasUpdate {
		t.Fatalf("statuses = %+v, want current for same commit", statuses)
	}
}

func TestCheckDetectsTransitionFromTagToRelease(t *testing.T) {
	fp := &fakeProvider{release: provider.Release{Kind: provider.KindRelease, ID: 55, Tag: "v1.0.0", AssetName: "a.zip"}}
	svc, install, _ := newTestService(t, fp)

	reg := &registry.Registry{Version: 1}
	reg.UpsertAddon(registry.Addon{
		Provider: "github", Ref: "Owner/MyAddon", Flavor: "retail",
		InstallPath: install.Path, Version: "v1.0.0", Source: "tag",
	})
	if err := reg.Save(svc.RegistryPath); err != nil {
		t.Fatal(err)
	}

	statuses, err := svc.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || !statuses[0].HasUpdate {
		t.Fatalf("statuses = %+v, want update when a release appears", statuses)
	}
}

func TestCheckSurfacesPerAddonError(t *testing.T) {
	fp := &fakeProvider{releaseErr: errors.New("api down")}
	svc, install, _ := newTestService(t, fp)
	seedAddon(t, svc, install, 100, "v1.0.0")

	statuses, err := svc.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || statuses[0].Error == nil {
		t.Fatalf("statuses = %+v", statuses)
	}
}

func TestUpdateInstallsNewVersion(t *testing.T) {
	fp := &fakeProvider{
		release:    provider.Release{Kind: provider.KindRelease, ID: 200, Tag: "v2.0.0", AssetName: "MyAddon-2.0.0.zip"},
		zipContent: addonZip(t, "2.0.0"),
	}
	svc, install, _ := newTestService(t, fp)
	seedAddon(t, svc, install, 100, "v1.0.0")

	addonsDir := filepath.Join(install.Path, "Interface", "AddOns")
	if err := os.MkdirAll(filepath.Join(addonsDir, "MyAddon"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(addonsDir, "MyAddon", "MyAddon.toc"), []byte("## Version: 1.0.0"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := svc.Update(context.Background(), "Owner/MyAddon", false); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(addonsDir, "MyAddon", "MyAddon.toc"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "## Version: 2.0.0" {
		t.Errorf("updated toc = %q", content)
	}

	reg, err := registry.Load(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	addon := reg.FindAddon("Owner/MyAddon", install.Path)
	if addon == nil || addon.ReleaseID != 200 || addon.Version != "v2.0.0" {
		t.Errorf("registry entry = %+v", addon)
	}

	backups, err := os.ReadDir(svc.BackupRoot)
	if err != nil || len(backups) == 0 {
		t.Error("expected a backup of the old version")
	}
}

func TestUpdateDryRunChangesNothing(t *testing.T) {
	fp := &fakeProvider{
		release:    provider.Release{Kind: provider.KindRelease, ID: 200, Tag: "v2.0.0", AssetName: "a.zip"},
		zipContent: addonZip(t, "2.0.0"),
	}
	svc, install, out := newTestService(t, fp)
	seedAddon(t, svc, install, 100, "v1.0.0")

	addonsDir := filepath.Join(install.Path, "Interface", "AddOns")
	if err := os.MkdirAll(filepath.Join(addonsDir, "MyAddon"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(addonsDir, "MyAddon", "MyAddon.toc"), []byte("## Version: 1.0.0"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := svc.Update(context.Background(), "Owner/MyAddon", true); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(addonsDir, "MyAddon", "MyAddon.toc"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "## Version: 1.0.0" {
		t.Errorf("dry run must not install, toc = %q", content)
	}
	if fp.downloads != 0 {
		t.Error("dry run must not download")
	}
	if !strings.Contains(out.String(), "v2.0.0") {
		t.Errorf("dry run output missing target version: %s", out.String())
	}
}

func TestUpdateAlreadyCurrent(t *testing.T) {
	fp := &fakeProvider{release: provider.Release{Kind: provider.KindRelease, ID: 100, Tag: "v1.0.0", AssetName: "a.zip"}}
	svc, install, out := newTestService(t, fp)
	seedAddon(t, svc, install, 100, "v1.0.0")

	if err := svc.Update(context.Background(), "Owner/MyAddon", false); err != nil {
		t.Fatal(err)
	}
	if fp.downloads != 0 {
		t.Error("no download expected when up to date")
	}
	if !strings.Contains(out.String(), "up to date") {
		t.Errorf("output = %s", out.String())
	}
}

func TestUpdateMatchesByRepoShortName(t *testing.T) {
	fp := &fakeProvider{release: provider.Release{Kind: provider.KindRelease, ID: 100, Tag: "v1.0.0", AssetName: "a.zip"}}
	svc, install, out := newTestService(t, fp)
	seedAddon(t, svc, install, 100, "v1.0.0")

	if err := svc.Update(context.Background(), "MyAddon", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "up to date") {
		t.Errorf("output = %s", out.String())
	}
}

func TestUpdateUnknownAddon(t *testing.T) {
	svc, _, _ := newTestService(t, &fakeProvider{})
	if err := svc.Update(context.Background(), "Nope", false); err == nil {
		t.Fatal("expected error for unknown addon")
	}
}

func TestUpdateAll(t *testing.T) {
	fp := &fakeProvider{
		release:    provider.Release{Kind: provider.KindRelease, ID: 200, Tag: "v2.0.0", AssetName: "MyAddon-2.0.0.zip"},
		zipContent: addonZip(t, "2.0.0"),
	}
	svc, install, _ := newTestService(t, fp)
	seedAddon(t, svc, install, 100, "v1.0.0")

	addonsDir := filepath.Join(install.Path, "Interface", "AddOns")
	if err := os.MkdirAll(filepath.Join(addonsDir, "MyAddon"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(addonsDir, "MyAddon", "MyAddon.toc"), []byte("## Version: 1.0.0"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := svc.UpdateAll(context.Background(), false); err != nil {
		t.Fatal(err)
	}

	reg, err := registry.Load(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	addon := reg.FindAddon("Owner/MyAddon", install.Path)
	if addon == nil || addon.ReleaseID != 200 {
		t.Errorf("registry entry = %+v", addon)
	}
}

func TestAddEmitsStageSequence(t *testing.T) {
	fp := &fakeProvider{
		release:    provider.Release{Kind: provider.KindRelease, ID: 100, Tag: "v1.0.0", AssetName: "MyAddon-1.0.0.zip", DownloadURL: "http://fake/zip", Size: 10},
		zipContent: addonZip(t, "1.0.0"),
	}
	svc, install, out := newTestService(t, fp)
	ref := provider.Reference{Provider: "github", ID: "Owner/MyAddon"}
	if err := svc.Add(context.Background(), ref, wow.Retail, install, ""); err != nil {
		t.Fatal(err)
	}

	download := out.firstIndex(StageDownload)
	extract := out.firstIndex(StageExtract)
	install_ := out.firstIndex(StageInstall)
	done := out.firstIndex(StageDone)
	if download < 0 || extract < 0 || install_ < 0 || done < 0 {
		t.Fatalf("missing stages, events = %+v", out.events)
	}
	if !(download < extract && extract < install_ && install_ < done) {
		t.Errorf("stage order wrong: download=%d extract=%d install=%d done=%d", download, extract, install_, done)
	}
	if out.events[done].Ref != "Owner/MyAddon" {
		t.Errorf("done event ref = %q", out.events[done].Ref)
	}
}

func TestUpdateAllEmitsBatchProgress(t *testing.T) {
	fp := &fakeProvider{release: provider.Release{Kind: provider.KindRelease, ID: 100, Tag: "v1.0.0", AssetName: "a.zip"}}
	svc, install, out := newTestService(t, fp)

	reg := &registry.Registry{Version: 1}
	reg.UpsertAddon(registry.Addon{Provider: "github", Ref: "Owner/One", Flavor: "retail", InstallPath: install.Path, Version: "v1.0.0", Source: "auto", ReleaseID: 100})
	reg.UpsertAddon(registry.Addon{Provider: "github", Ref: "Owner/Two", Flavor: "retail", InstallPath: install.Path, Version: "v1.0.0", Source: "auto", ReleaseID: 100})
	if err := reg.Save(svc.RegistryPath); err != nil {
		t.Fatal(err)
	}

	if err := svc.UpdateAll(context.Background(), false); err != nil {
		t.Fatal(err)
	}

	var currents []Event
	for _, e := range out.events {
		if e.Stage == StageCurrent {
			currents = append(currents, e)
		}
	}
	if len(currents) != 2 {
		t.Fatalf("got %d up-to-date events, want 2: %+v", len(currents), out.events)
	}
	if currents[0].Index != 1 || currents[0].Total != 2 || currents[1].Index != 2 || currents[1].Total != 2 {
		t.Errorf("batch indices wrong: %+v", currents)
	}
}

func TestScanPreservesTitle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc, install, _ := newTestService(t, &fakeProvider{})

	addonDir := filepath.Join(install.Path, "Interface", "AddOns", "MyAddon")
	if err := os.MkdirAll(addonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(addonDir, "MyAddon.toc"), []byte("## Version: 1.0"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &registry.Registry{Version: 1, Installations: []registry.Installation{{Path: install.Path, Flavor: "retail", Title: "Mein Retail"}}}
	if err := reg.Save(svc.RegistryPath); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Scan(context.Background(), []string{install.Path}); err != nil {
		t.Fatal(err)
	}

	reg2, err := registry.Load(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(reg2.Installations) != 1 || reg2.Installations[0].Title != "Mein Retail" {
		t.Fatalf("title lost by scan: %+v", reg2.Installations)
	}
}

func TestSetInstallTitle(t *testing.T) {
	svc, install, _ := newTestService(t, &fakeProvider{})
	reg := &registry.Registry{Version: 1, Installations: []registry.Installation{{Path: install.Path, Flavor: "retail"}}}
	if err := reg.Save(svc.RegistryPath); err != nil {
		t.Fatal(err)
	}

	if err := svc.SetInstallTitle(install.Path, "Classic Alt"); err != nil {
		t.Fatal(err)
	}
	reg2, err := registry.Load(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	if reg2.Installations[0].Title != "Classic Alt" {
		t.Errorf("title = %q", reg2.Installations[0].Title)
	}

	if err := svc.SetInstallTitle("/does/not/exist", "x"); err == nil {
		t.Fatal("expected error for unknown installation")
	}
}

func TestMapAndAdoptUnmanagedAddon(t *testing.T) {
	fake := &fakeProvider{release: provider.Release{Kind: provider.KindRelease, ID: 1, Tag: "v1"}}
	svc, install, _ := newTestService(t, fake)
	reg := &registry.Registry{Version: 1, Installations: []registry.Installation{{
		Path:   install.Path,
		Flavor: "retail",
		Folders: []registry.Folder{{Name: "MyAddon", Version: "1.0", Hints: map[string]string{
			"x-source-repo": "https://github.com/Owner/MyAddon",
		}}},
	}}}
	if err := reg.Save(svc.RegistryPath); err != nil {
		t.Fatal(err)
	}

	candidates, err := svc.MapUnmanaged()
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Provider != "github" || candidates[0].Reference != "Owner/MyAddon" {
		t.Errorf("candidates = %+v", candidates)
	}

	if err := svc.Adopt(context.Background(), install.Path, "MyAddon", provider.Reference{Provider: "github", ID: "Owner/MyAddon"}); err != nil {
		t.Fatal(err)
	}
	reg, err = registry.Load(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	addon := reg.FindAddon("Owner/MyAddon", install.Path)
	if addon == nil || addon.Version != "1.0" || addon.Folders[0] != "MyAddon" {
		t.Errorf("adopted addon = %+v", addon)
	}
}

func TestScanPreservesManagedAddons(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc, install, _ := newTestService(t, &fakeProvider{})

	addonDir := filepath.Join(install.Path, "Interface", "AddOns", "MyAddon")
	if err := os.MkdirAll(addonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(addonDir, "MyAddon.toc"), []byte("## Version: 1.0\n## Interface: 120007"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &registry.Registry{Version: 1}
	reg.UpsertAddon(registry.Addon{Provider: "github", Ref: "Owner/MyAddon", Flavor: "retail", InstallPath: install.Path, Folders: []string{"MyAddon"}, Version: "v1.0.0", Source: "auto", ReleaseID: 100})
	if err := reg.Save(svc.RegistryPath); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Scan(context.Background(), []string{install.Path}); err != nil {
		t.Fatal(err)
	}

	reg2, err := registry.Load(svc.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(reg2.Addons) != 1 || reg2.Addons[0].Ref != "Owner/MyAddon" {
		t.Fatalf("managed addons lost by scan: %+v", reg2.Addons)
	}
	if len(reg2.Installations) != 1 {
		t.Fatalf("got %d installations, want 1", len(reg2.Installations))
	}
	if reg2.Installations[0].Flavor != "retail" {
		t.Errorf("flavor = %q", reg2.Installations[0].Flavor)
	}
	if len(reg2.Installations[0].Folders) != 1 || reg2.Installations[0].Folders[0].Name != "MyAddon" {
		t.Errorf("folders = %+v", reg2.Installations[0].Folders)
	}
}
