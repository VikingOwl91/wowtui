package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"somegit.dev/vikingowl/wowtui/internal/index"
	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/registry"
)

type importTestProvider struct{ provider.Provider }

func (importTestProvider) Resolve(_ context.Context, ref provider.Reference) (provider.RemoteAddon, error) {
	if ref.ID == "Owner/B" {
		return provider.RemoteAddon{}, errors.New("source unavailable")
	}
	return provider.RemoteAddon{Name: ref.ID, ID: ref.ID}, nil
}

func TestBulkImportReviewAndConfirm(t *testing.T) {
	m := testModel(t)
	m.deps.RegistryPath = filepath.Join(t.TempDir(), "registry.json")
	m.deps.Providers = map[string]provider.Provider{"github": importTestProvider{}, "curseforge": importTestProvider{}}
	reg := &registry.Registry{Version: 1, Installations: []registry.Installation{
		{Path: "/retail", Flavor: "retail", Folders: []registry.Folder{
			{Name: "A", Hints: map[string]string{"x-source-repo": "https://github.com/Owner/A", "x-curse-project-id": "42"}},
			{Name: "B", Hints: map[string]string{"x-source-repo": "https://github.com/Owner/B"}},
			{Name: "NoSource"},
		}},
		{Path: "/classic", Flavor: "classic", Folders: []registry.Folder{{Name: "Other", Hints: map[string]string{"x-source-repo": "https://github.com/Owner/Other"}}}},
	}}
	if err := reg.Save(m.deps.RegistryPath); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(m.deps.RegistryPath)
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 32})
	m.Update(registryLoadedMsg{reg: reg})
	m.gameFilter = "retail"
	_, cmd := m.Update(keyMsg("m"))
	if !strings.Contains(m.View(), "Import installed addons") || cmd == nil {
		t.Fatal("m should open the import screen and start matching")
	}
	driveServiceOp(t, m, cmd)
	view := m.View()
	for _, want := range []string{"0 selected", "1 without an active match", "Owner/A", "curseforge:42", "Owner/B"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q in %s", want, view)
		}
	}
	if strings.Contains(view, "Owner/Other") {
		t.Fatal("classic installation leaked into retail scope")
	}
	m.Update(keyMsg("a"))
	if !strings.Contains(m.View(), "1 selected") {
		t.Fatal("bulk selection should skip ambiguous A")
	}
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view = m.View()
	if !strings.Contains(view, "Review import") || !strings.Contains(view, "Owner/A") || strings.Contains(view, "curseforge:42") {
		t.Fatalf("incorrect review %s", view)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	onDisk, err := os.ReadFile(m.deps.RegistryPath)
	if err != nil || string(onDisk) != string(original) {
		t.Fatal("review modified registry before confirmation")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_, cmd = m.Update(keyMsg("y"))
	driveServiceOp(t, m, cmd)
	view = m.View()
	for _, want := range []string{"Import results", "1 imported", "1 failed", "source unavailable"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q in %s", want, view)
		}
	}
	saved, err := registry.Load(m.deps.RegistryPath)
	if err != nil || len(saved.Addons) != 1 || saved.Addons[0].Ref != "Owner/A" {
		t.Fatalf("saved: %+v, %v", saved, err)
	}
	_, cmd = m.Update(keyMsg("r"))
	driveServiceOp(t, m, cmd)
	if view = m.View(); !strings.Contains(view, "Owner/B") || strings.Contains(view, "Owner/A") {
		t.Fatal("refresh after partial success should show pending folders")
	}
}

// Run the operation through the same event stream as Bubble Tea.
func driveServiceOp(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	for m.busy {
		if cmd == nil {
			t.Fatal("missing operation command")
		}
		_, cmd = m.Update(cmd())
	}
	if cmd != nil {
		m.Update(cmd())
	}
}

func TestImportUnmatchedDetailAndUnavailableIndex(t *testing.T) {
	m := loadedModel(t, 80, 24)
	m.deps.RegistryPath = filepath.Join(t.TempDir(), "registry.json")
	m.deps.LoadImportIndex = func() (*index.Index, error) { return nil, errors.New("bad signature") }
	if err := m.reg.Save(m.deps.RegistryPath); err != nil {
		t.Fatal(err)
	}
	m.Update(keyMsg("d"))
	_, cmd := m.Update(keyMsg("m"))
	driveServiceOp(t, m, cmd)
	view := m.View()
	for _, want := range []string{"No active source for WeakAuras", "bad signature", "built-in fingerprints", "1 without an active match"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q in %s", want, view)
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(keyMsg("y"))
	if m.busy || !strings.Contains(m.View(), "Import installed addons") {
		t.Fatal("empty selection was submittable")
	}
}

// This provider only resolves after cancellation, letting the test exercise
// keyboard input while an actual import is in flight.
type cancelImportProvider struct {
	provider.Provider
	started chan struct{}
}

func (p cancelImportProvider) Resolve(ctx context.Context, _ provider.Reference) (provider.RemoteAddon, error) {
	close(p.started)
	<-ctx.Done()
	return provider.RemoteAddon{}, ctx.Err()
}

func TestImportCancelDuringResolve(t *testing.T) {
	m := loadedModel(t, 80, 24)
	m.deps.RegistryPath = filepath.Join(t.TempDir(), "registry.json")
	started := make(chan struct{})
	m.deps.Providers["github"] = cancelImportProvider{started: started}
	m.reg.Installations[0].Folders[0].Hints = map[string]string{"x-source-repo": "https://github.com/Org/Repo"}
	if err := m.reg.Save(m.deps.RegistryPath); err != nil {
		t.Fatal(err)
	}
	_, cmd := m.Update(keyMsg("m"))
	driveServiceOp(t, m, cmd)
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_, cmd = m.Update(keyMsg("y"))
	if !m.busy || cmd == nil {
		t.Fatal("import did not start")
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("source resolution did not start")
	}
	if _, repeat := m.Update(keyMsg("y")); repeat != nil {
		t.Fatal("started second batch")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	driveServiceOp(t, m, cmd)
	if !strings.Contains(m.View(), "Import results") || !strings.Contains(m.View(), "canceled") {
		t.Fatalf("missing cancellation: %s", m.View())
	}
	r, err := registry.Load(m.deps.RegistryPath)
	if err != nil || len(r.Addons) != 0 {
		t.Fatalf("unexpected registry after cancellation: %+v, %v", r, err)
	}
}

func TestImportWaitsForInitialRegistry(t *testing.T) {
	m := testModel(t)
	_, cmd := m.Update(keyMsg("m"))
	if cmd != nil || m.busy || m.screen != screenAddons {
		t.Fatal("import overlapped initial registry load and auto-scan")
	}
}
