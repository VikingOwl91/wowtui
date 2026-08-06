package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"somegit.dev/vikingowl/wowtui/internal/config"
	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/registry"
)

func testModel(t *testing.T) *Model {
	t.Helper()
	deps := Deps{
		Config:       &config.Config{},
		Providers:    map[string]provider.Provider{},
		RegistryPath: "/tmp/none",
		BackupRoot:   "/tmp/none",
	}
	return New(deps)
}

func TestModelRendersAddonsScreen(t *testing.T) {
	m := testModel(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	mm := model.(*Model)

	reg := &registry.Registry{
		Version: 1,
		Installations: []registry.Installation{
			{Path: "/games/_retail_", Flavor: "retail", Folders: []registry.Folder{{Name: "WeakAuras", Version: "5.0"}}},
		},
	}
	model, _ = mm.Update(registryLoadedMsg{reg: reg})
	mm = model.(*Model)

	view := mm.View()
	if !strings.Contains(view, "wowtui") {
		t.Errorf("view missing title: %s", view)
	}
	if !strings.Contains(view, "WeakAuras") {
		t.Errorf("view missing addon: %s", view)
	}
	if !strings.Contains(view, "retail") {
		t.Errorf("view missing flavor: %s", view)
	}
}

func TestModelRendersEmptyState(t *testing.T) {
	m := testModel(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	mm := model.(*Model)
	model, _ = mm.Update(registryLoadedMsg{reg: &registry.Registry{Version: 1}})
	mm = model.(*Model)

	view := mm.View()
	if !strings.Contains(view, "scan") {
		t.Errorf("empty state should hint to scan: %s", view)
	}
}

func TestAutoScanOnEmptyRegistry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	base := t.TempDir()
	m := New(Deps{
		Config:       &config.Config{},
		Providers:    map[string]provider.Provider{},
		RegistryPath: filepath.Join(base, "registry.json"),
		BackupRoot:   filepath.Join(base, "backups"),
	})
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	mm := model.(*Model)
	model, _ = mm.Update(registryLoadedMsg{reg: &registry.Registry{Version: 1}})
	mm = model.(*Model)

	if !mm.autoScanned {
		t.Error("auto-scan should trigger on empty registry")
	}
	if !mm.busy {
		t.Error("auto-scan should set busy")
	}
}

func TestInstallationLabels(t *testing.T) {
	installs := []registry.Installation{
		{Path: "/a/_retail_", Flavor: "retail"},
		{Path: "/b/_retail_", Flavor: "retail"},
		{Path: "/c/_classic_", Flavor: "classic", Title: "Mein Classic"},
	}
	labels := installationLabels(installs)

	if labels[0] == labels[1] {
		t.Errorf("two retail installs must have distinct labels: %v", labels)
	}
	if labels[2] != "Mein Classic" {
		t.Errorf("title label = %q, want Mein Classic", labels[2])
	}
}

func TestInstallationFilter(t *testing.T) {
	m := testModel(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	mm := model.(*Model)

	reg := &registry.Registry{
		Version: 1,
		Installations: []registry.Installation{
			{Path: "/games/_retail_", Flavor: "retail", Folders: []registry.Folder{{Name: "WeakAuras", Version: "5.0"}}},
			{Path: "/games/_classic_", Flavor: "classic", Folders: []registry.Folder{{Name: "Questie", Version: "1.0"}}},
		},
	}
	model, _ = mm.Update(registryLoadedMsg{reg: reg})
	mm = model.(*Model)

	if len(mm.filteredRows()) != 2 {
		t.Fatalf("unfiltered rows = %d, want 2", len(mm.filteredRows()))
	}

	mm.installFilter = 0
	rows := mm.filteredRows()
	if len(rows) != 1 || rows[0].Name != "WeakAuras" {
		t.Errorf("install filter 0 rows = %+v", rows)
	}

	mm.installFilter = 1
	rows = mm.filteredRows()
	if len(rows) != 1 || rows[0].Name != "Questie" {
		t.Errorf("install filter 1 rows = %+v", rows)
	}
}

func TestFuzzyScore(t *testing.T) {
	if fuzzyScore("waks", "WeakAuras") < 0 {
		t.Fatal("expected subsequence match")
	}
	if fuzzyScore("zzz", "WeakAuras") >= 0 {
		t.Fatal("unexpected fuzzy match")
	}
}

func TestGameFilterCyclesVersions(t *testing.T) {
	rows := []addonRow{{Flavor: "retail"}, {Flavor: "classic"}, {Flavor: "retail"}}
	if got := cycleGameFilter("all", rows); got != "retail" {
		t.Fatalf("first game filter = %q, want retail", got)
	}
	if got := cycleGameFilter("retail", rows); got != "classic" {
		t.Fatalf("second game filter = %q, want classic", got)
	}
}
