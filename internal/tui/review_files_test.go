package tui

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"somegit.dev/vikingowl/wowtui/internal/fingerprint"
	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/registry"
	"somegit.dev/vikingowl/wowtui/internal/sandbox/worker"
	"somegit.dev/vikingowl/wowtui/internal/service"
	"somegit.dev/vikingowl/wowtui/internal/wow"
)

func TestMain(m *testing.M) {
	if handled, code := worker.Dispatch(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); handled {
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func TestImportPackageSelectionAndFileReview(t *testing.T) {
	m := loadedModel(t, 80, 24)
	s := &m.importing
	s.selected = make(map[importFolder]service.MappingCandidate)
	s.choices = make(map[importFolder]int)
	m.screen = screenImport
	folders := []string{"Core", "Options", "Archive", "Models", "Templates"}
	var changes []fingerprint.Change
	for i := range 33 {
		changes = append(changes, fingerprint.Change{Path: fmt.Sprintf("Core/file%02d.lua", i), Kind: "modified"})
	}
	var candidates []service.MappingCandidate
	for _, folder := range folders {
		candidates = append(candidates, service.MappingCandidate{
			InstallPath: "/retail", Folder: folder, Reference: "github:Owner/Core", ReleaseVersion: "1.2",
			PackageFolders: folders, FileChanges: changes, Availability: service.AvailabilityActive,
		})
	}
	candidates = append(candidates, service.MappingCandidate{
		InstallPath: "/retail", Folder: "Options", Reference: "github:Alt/Options", Availability: service.AvailabilityActive,
	})
	m.finishImportMap(opResult{candidates: candidates})
	m.Update(keyMsg("a"))
	if len(s.selected) != 0 {
		t.Fatal("bulk selection chose a package with an ambiguous folder")
	}
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(s.reviewed) != 5 || !strings.Contains(m.View(), "release 1.2") || !strings.Contains(m.View(), "33 local changes") || !strings.Contains(m.View(), "Core/file00.lua") {
		t.Fatalf("incorrect package review: %s", m.View())
	}
	m.Update(keyMsg("d"))
	if !strings.Contains(m.View(), "modified  Core/file00.lua") {
		t.Fatalf("missing initial file: %s", m.View())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if !strings.Contains(m.View(), "modified  Core/file32.lua") {
		t.Fatalf("last file inaccessible: %s", m.View())
	}
	m.Update(keyMsg("y"))
	if m.busy {
		t.Fatal("file preview authorized an import")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if len(s.selected) != 1 || s.selected[importKey(candidates[5])].Reference != candidates[5].Reference {
		t.Fatalf("switching one member's source retained partial package: %v", s.selected)
	}
}

func TestUpdateFileReviewDefaultsToKeep(t *testing.T) {
	m := loadedModel(t, 80, 24)
	err := &service.LocalChangesError{InstallPath: "/retail", Ref: "Owner/MyAddon", Changes: []fingerprint.Change{{Kind: "added", Path: "MyAddon/private.lua"}}}
	finish := func(kind opKind) {
		t.Helper()
		startTestOp(t, m, err)
		m.opKind = kind
		m.Update(opClosedMsg{})
	}
	finish(opUpdateAll)
	if m.screen != screenAddons || !strings.Contains(m.View(), "press u") {
		t.Fatalf("bulk update requested replacement or hid next step: %s", m.View())
	}
	finish(opUpdate)
	if !strings.Contains(m.View(), "added  MyAddon/private.lua") || !strings.Contains(m.View(), "/retail") {
		t.Fatalf("single-addon review missing context: %s", m.View())
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || m.busy || m.screen != screenAddons {
		t.Fatal("default response did not keep local changes")
	}
	finish(opUpdate)
	_, cmd = m.Update(keyMsg("y"))
	if cmd == nil || m.opKind != opUpdate {
		t.Fatal("explicit confirmation should start the reviewed update")
	}
	driveServiceOp(t, m, cmd)
}

type repairProvider struct {
	provider.Provider
	archive []byte
}

func (p repairProvider) Latest(context.Context, provider.RemoteAddon, wow.GameFlavor, string) (provider.Release, error) {
	return provider.Release{Kind: provider.KindRelease, ID: 1, Tag: "v1", AssetName: "Core.zip"}, nil
}

func (p repairProvider) Download(context.Context, provider.Release) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(p.archive)), nil
}

func TestTUIRepairOnlyReviewedInstallationAndBytes(t *testing.T) {
	const toc, original = "Core/Core.toc", "## Version: v1"
	var archive bytes.Buffer
	w := zip.NewWriter(&archive)
	f, err := w.Create(toc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(f, original); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	m := testModel(t)
	base := t.TempDir()
	m.deps.RegistryPath = filepath.Join(base, "registry.json")
	m.deps.BackupRoot = filepath.Join(base, "backups")
	m.deps.Providers["github"] = repairProvider{archive: archive.Bytes()}
	reg := &registry.Registry{Version: 1}
	file := func(flavor string) string { return filepath.Join(base, flavor, "Interface", "AddOns", toc) }
	for _, flavor := range []string{"classic", "retail"} {
		install := filepath.Join(base, flavor)
		reg.Installations = append(reg.Installations, registry.Installation{Path: install, Flavor: flavor, Folders: []registry.Folder{{Name: "Core"}}})
		reg.Addons = append(reg.Addons, registry.Addon{
			Ref: "Owner/Core", Provider: "github", Version: "v1", Flavor: flavor, InstallPath: install, Folders: []string{"Core"},
			ReleaseFiles: []fingerprint.File{{Path: toc, SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(original)))}},
		})
		if err := os.MkdirAll(filepath.Dir(file(flavor)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file(flavor), []byte(flavor+" edit"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := reg.Save(m.deps.RegistryPath); err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(registryLoadedMsg{reg: reg})
	m.table.SetCursor(1)
	_, cmd := m.Update(keyMsg("u"))
	driveServiceOp(t, m, cmd)
	if m.screen != screenFileReview || m.files.approval.InstallPath != filepath.Join(base, "retail") {
		t.Fatal("u did not open the selected retail package")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_, cmd = m.Update(keyMsg("u"))
	driveServiceOp(t, m, cmd)
	m.table.SetCursor(0)
	if err := os.WriteFile(file("retail"), []byte("second edit"), 0600); err != nil {
		t.Fatal(err)
	}
	_, cmd = m.Update(keyMsg("y"))
	driveServiceOp(t, m, cmd)
	if m.screen != screenFileReview || m.files.approval.InstallPath != filepath.Join(base, "retail") {
		t.Fatal("stale approval did not redisplay the reviewed retail package")
	}
	assertContent := func(flavor, want string) {
		t.Helper()
		b, err := os.ReadFile(file(flavor))
		if err != nil || string(b) != want {
			t.Fatalf("%s files = %q, %v; want %q", flavor, b, err, want)
		}
	}
	assertContent("retail", "second edit")
	_, cmd = m.Update(keyMsg("y"))
	driveServiceOp(t, m, cmd)
	assertContent("retail", original)
	assertContent("classic", "classic edit")
}
