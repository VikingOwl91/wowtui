package tui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"somegit.dev/vikingowl/wowtui/internal/config"
	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/registry"
	"somegit.dev/vikingowl/wowtui/internal/service"
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
	model, cmd := mm.Update(registryLoadedMsg{reg: &registry.Registry{Version: 1}})
	mm = model.(*Model)

	if !mm.autoScanned {
		t.Error("auto-scan should trigger on empty registry")
	}
	if !mm.busy {
		t.Error("auto-scan should set busy")
	}
	// Finish the asynchronous scan while the fixture home is still active.
	driveServiceOp(t, mm, cmd)
	if len(mm.reg.Installations) != 0 {
		t.Fatal("auto-scan found a client outside the fixture home")
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

func loadedModel(t *testing.T, width, height int) *Model {
	t.Helper()
	m := testModel(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	mm := model.(*Model)
	reg := &registry.Registry{
		Version: 1,
		Installations: []registry.Installation{
			{Path: "/games/_retail_", Flavor: "retail", Folders: []registry.Folder{{Name: "WeakAuras", Version: "5.0"}}},
		},
	}
	model, _ = mm.Update(registryLoadedMsg{reg: reg})
	return model.(*Model)
}

func startTestOp(t *testing.T, m *Model, err error) {
	t.Helper()
	m.startOp(opUpdateAll, func(*service.Service) opResult { return opResult{err: err} })
	for range m.eventChan {
	}
}

func runTestOp(t *testing.T, m *Model, err error, events ...service.Event) {
	t.Helper()
	startTestOp(t, m, err)
	for _, e := range events {
		m.Update(serviceEventMsg(e))
	}
	m.Update(opClosedMsg{})
}

func keyMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func lineContaining(view, text string) string {
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, text) {
			return line
		}
	}
	return ""
}

const (
	testWarning = "warning: github:a/b replaced unmanaged folders Foo (backup: /backups/foo)"
	testFailure = "github:c/d: download failed"
)

func TestOperationWarningsPersistAfterClose(t *testing.T) {
	mm := loadedModel(t, 120, 30)
	startTestOp(t, mm, errors.New("update failed for 1 addon"))

	mm.Update(serviceEventMsg(service.Event{Ref: "github:a/b", Stage: service.StageWarning, Message: testWarning}))
	if mm.busyMsg != testWarning {
		t.Fatalf("busyMsg = %q, want live warning", mm.busyMsg)
	}
	mm.Update(serviceEventMsg(service.Event{Ref: "github:c/d", Stage: service.StageError, Message: testFailure}))
	mm.Update(serviceEventMsg(service.Event{Ref: "github:e/f", Stage: service.StageDone, Message: "installed github:e/f"}))
	mm.Update(opClosedMsg{})

	view := mm.View()
	for _, want := range []string{testWarning, testFailure, "update failed for 1 addon", "ctrl+x dismiss"} {
		if !strings.Contains(view, want) {
			t.Errorf("view after close missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "installed github:e/f") {
		t.Errorf("non-warning event leaked into notices:\n%s", view)
	}
}

func TestNoticeKindsAreDistinct(t *testing.T) {
	mm := loadedModel(t, 120, 30)
	runTestOp(t, mm, nil,
		service.Event{Ref: "github:a/b", Stage: service.StageWarning, Message: testWarning},
		service.Event{Ref: "github:c/d", Stage: service.StageError, Message: testFailure},
	)

	view := mm.View()
	if line := lineContaining(view, testWarning); !strings.Contains(line, "! "+testWarning) {
		t.Errorf("warning line = %q, want warning marker", line)
	}
	if line := lineContaining(view, testFailure); !strings.Contains(line, "✗ "+testFailure) {
		t.Errorf("error line = %q, want error marker", line)
	}
}

func TestOperationErrorSurvivesLaterMessages(t *testing.T) {
	mm := loadedModel(t, 120, 30)
	runTestOp(t, mm, errors.New("update failed for 1 addon"))

	mm.Update(keyMsg("u"))

	view := mm.View()
	for _, want := range []string{"update failed for 1 addon", "select a managed addon to update"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}

func TestDismissKeyClearsNoticesOnEveryScreen(t *testing.T) {
	cases := []struct {
		name  string
		setup func(m *Model)
	}{
		{"addons", func(m *Model) {}},
		{"addons search", func(m *Model) { m.Update(keyMsg("f")) }},
		{"explore", func(m *Model) { m.Update(tea.KeyMsg{Type: tea.KeyTab}) }},
		{"details", func(m *Model) { m.Update(keyMsg("d")) }},
		{"add", func(m *Model) { m.Update(keyMsg("a")) }},
		{"settings", func(m *Model) { m.Update(keyMsg("3")) }},
		{"settings rename", func(m *Model) { m.Update(keyMsg("3")); m.Update(keyMsg("r")) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mm := loadedModel(t, 120, 30)
			runTestOp(t, mm, errors.New("update failed for 1 addon"),
				service.Event{Ref: "github:a/b", Stage: service.StageWarning, Message: testWarning},
			)
			tc.setup(mm)
			screen, searching, renaming := mm.screen, mm.searching, mm.renaming
			inputs := []string{mm.input.Value(), mm.searchInput.Value(), mm.exploreInput.Value(), mm.renameInput.Value()}
			if view := mm.View(); !strings.Contains(view, testWarning) || !strings.Contains(view, "ctrl+x dismiss") {
				t.Fatalf("notice not shown before dismiss:\n%s", view)
			}

			mm.Update(tea.KeyMsg{Type: tea.KeyCtrlX})

			view := mm.View()
			for _, gone := range []string{testWarning, "update failed for 1 addon", "ctrl+x dismiss"} {
				if strings.Contains(view, gone) {
					t.Errorf("view still shows %q after dismiss:\n%s", gone, view)
				}
			}
			if mm.screen != screen || mm.searching != searching || mm.renaming != renaming {
				t.Errorf("dismiss changed screen state")
			}
			after := []string{mm.input.Value(), mm.searchInput.Value(), mm.exploreInput.Value(), mm.renameInput.Value()}
			for i := range inputs {
				if inputs[i] != after[i] {
					t.Errorf("dismiss changed input %d: %q -> %q", i, inputs[i], after[i])
				}
			}
		})
	}
}

func TestNewOperationClearsNotices(t *testing.T) {
	mm := loadedModel(t, 120, 30)
	runTestOp(t, mm, errors.New("update failed for 1 addon"),
		service.Event{Ref: "github:a/b", Stage: service.StageWarning, Message: testWarning},
	)
	if view := mm.View(); !strings.Contains(view, testWarning) {
		t.Fatalf("notice not shown after first operation:\n%s", view)
	}

	startTestOp(t, mm, nil)
	if view := mm.View(); strings.Contains(view, testWarning) {
		t.Errorf("old warning shown while next operation runs:\n%s", view)
	}
	mm.Update(opClosedMsg{})

	view := mm.View()
	for _, gone := range []string{testWarning, "update failed for 1 addon", "ctrl+x dismiss"} {
		if strings.Contains(view, gone) {
			t.Errorf("view still shows %q after next operation:\n%s", gone, view)
		}
	}
}

func TestExploreAddIgnoredWhileOperationRuns(t *testing.T) {
	mm := loadedModel(t, 120, 30)
	mm.exploreResults = []provider.RemoteAddon{{Name: "WeakAuras", Provider: "github", ID: "WeakAuras/WeakAuras2"}}
	mm.Update(tea.KeyMsg{Type: tea.KeyTab})
	mm.exploreInput.Blur()

	mm.busyMsg = "updating all addons..."
	startTestOp(t, mm, nil)
	events := mm.eventChan

	_, cmd := mm.Update(keyMsg("a"))
	if cmd != nil {
		t.Error("a returned a command while an operation runs")
	}
	if mm.opKind != opUpdateAll || mm.eventChan != events {
		t.Errorf("a replaced the running operation: opKind = %v", mm.opKind)
	}
	if mm.errMsg != "" {
		t.Errorf("errMsg = %q, want none", mm.errMsg)
	}
	if footer := mm.footerView(); !strings.Contains(footer, "updating all addons...") {
		t.Errorf("footer = %q, want running operation", footer)
	}

	mm.Update(opClosedMsg{})
	if _, cmd := mm.Update(keyMsg("a")); cmd == nil || mm.opKind != opAdd {
		t.Errorf("a after the operation finished: opKind = %v, want add", mm.opKind)
	}
	for range mm.eventChan {
	}
}

func multiAddonError(n int) error {
	lines := []string{fmt.Sprintf("update failed for %d addons:", n)}
	for i := 1; i <= n; i++ {
		lines = append(lines, fmt.Sprintf("github:owner/addon%02d: %s", i, strings.Repeat("connection reset ", 10)))
	}
	return errors.New(strings.Join(lines, "\n"))
}

func TestLongErrorRendersOneTruncatedLinePerEntry(t *testing.T) {
	mm := loadedModel(t, 80, 30)
	runTestOp(t, mm, multiAddonError(12))

	view := mm.View()
	if line := lineContaining(view, "github:owner/addon01"); !strings.Contains(line, "✗ github:owner/addon01") {
		t.Errorf("entry not rendered as its own notice line: %q", line)
	}
	for _, want := range []string{"github:owner/addon01", "github:owner/addon03", "+9 more"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "github:owner/addon04") {
		t.Errorf("view shows entries beyond the cap:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if !strings.Contains(line, "✗") && !strings.Contains(line, "more") {
			continue
		}
		if w := lipgloss.Width(line); w > 80 {
			t.Errorf("notice line width %d > 80: %q", w, line)
		}
	}
	if line := lineContaining(view, "github:owner/addon01"); !strings.HasSuffix(strings.TrimRight(line, " "), "…") {
		t.Errorf("long entry not marked as truncated: %q", line)
	}
}

func TestTruncateText(t *testing.T) {
	cases := []struct {
		in    string
		width int
		want  string
	}{
		{"abc", 3, "abc"},
		{"abcdef", 4, "abc…"},
		{"abcdef", 1, ""},
		{"abcdef", 0, ""},
	}
	for _, tc := range cases {
		if got := truncateText(tc.in, tc.width); got != tc.want {
			t.Errorf("truncateText(%q, %d) = %q, want %q", tc.in, tc.width, got, tc.want)
		}
	}
}

func TestNoticesListErrorsFirstWithoutDuplicates(t *testing.T) {
	mm := loadedModel(t, 120, 30)
	runTestOp(t, mm, errors.New("update failed for 1 addon:\n"+testFailure),
		service.Event{Ref: "github:a/b", Stage: service.StageWarning, Message: testWarning},
		service.Event{Ref: "github:c/d", Stage: service.StageError, Message: testFailure},
		service.Event{Ref: "github:g/h", Stage: service.StageError, Message: "github:g/h: checksum mismatch"},
	)

	view := mm.View()
	if n := strings.Count(view, testFailure); n != 1 {
		t.Errorf("failure shown %d times, want 1:\n%s", n, view)
	}
	if strings.Index(view, testWarning) < strings.Index(view, "github:g/h: checksum mismatch") {
		t.Errorf("warning listed before error:\n%s", view)
	}
}

func TestViewFitsWindowHeightWithNotices(t *testing.T) {
	screens := []struct {
		name  string
		setup func(m *Model)
	}{
		{"addons", func(m *Model) {}},
		{"addons search", func(m *Model) { m.Update(keyMsg("f")) }},
		{"explore", func(m *Model) {
			m.exploreResults = []provider.RemoteAddon{{Name: "WeakAuras", Provider: "github", ID: "WeakAuras/WeakAuras2"}}
			m.Update(tea.KeyMsg{Type: tea.KeyTab})
		}},
	}
	for _, height := range []int{30, 20, 16} {
		for _, sc := range screens {
			t.Run(fmt.Sprintf("%s/%d", sc.name, height), func(t *testing.T) {
				mm := loadedModel(t, 120, height)
				events := make([]service.Event, 0, 6)
				for i := 0; i < 6; i++ {
					events = append(events, service.Event{Stage: service.StageWarning, Message: fmt.Sprintf("warning %d", i)})
				}
				runTestOp(t, mm, multiAddonError(12), events...)
				sc.setup(mm)

				view := mm.View()
				if got := lipgloss.Height(view); got > height {
					t.Errorf("view height %d > window height %d:\n%s", got, height, view)
				}
				if !strings.Contains(view, " more") || !strings.Contains(view, "ctrl+x dismiss") {
					t.Errorf("capped notice or dismiss hint missing:\n%s", view)
				}
			})
		}
	}
}

const typedText = "Weak Auras quad 1/3"

func typeText(m *Model, text string) {
	for _, r := range text {
		if r == ' ' {
			m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{r}})
			continue
		}
		m.Update(keyMsg(string(r)))
	}
}

func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func drainOp(m *Model) {
	for range m.eventChan {
	}
	m.Update(opClosedMsg{})
}

func openExploreWithResult(m *Model) {
	m.exploreResults = []provider.RemoteAddon{{Name: "WeakAuras", Provider: "github", ID: "WeakAuras/WeakAuras2"}}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
}

func openAddonsSearch(m *Model) { m.Update(keyMsg("f")) }

func openAddDialog(m *Model) { m.Update(keyMsg("a")) }

func openSettingsRename(m *Model) {
	m.Update(keyMsg("3"))
	m.Update(keyMsg("r"))
}

func TestFocusedInputsTakeCommandLetters(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(m *Model)
		screen  screen
		value   func(m *Model) string
		focused func(m *Model) bool
	}{
		{"explore search", openExploreWithResult, screenExplore,
			func(m *Model) string { return m.exploreInput.Value() },
			func(m *Model) bool { return m.exploreInput.Focused() }},
		{"addons search", openAddonsSearch, screenAddons,
			func(m *Model) string { return m.searchInput.Value() },
			func(m *Model) bool { return m.searching && m.searchInput.Focused() }},
		{"add reference", openAddDialog, screenAdd,
			func(m *Model) string { return m.input.Value() },
			func(m *Model) bool { return m.addField == 0 && m.input.Focused() }},
		{"settings rename", openSettingsRename, screenSettings,
			func(m *Model) string { return m.renameInput.Value() },
			func(m *Model) bool { return m.renaming && m.renameInput.Focused() }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mm := loadedModel(t, 120, 30)
			tc.setup(mm)
			if !tc.focused(mm) {
				t.Fatal("input not focused after setup")
			}

			typeText(mm, typedText)

			if got := tc.value(mm); got != typedText {
				t.Errorf("input = %q, want %q", got, typedText)
			}
			if mm.screen != tc.screen {
				t.Errorf("screen = %v, want %v", mm.screen, tc.screen)
			}
			if !tc.focused(mm) {
				t.Error("input lost focus while typing")
			}
			if mm.busy || mm.opKind != opNone {
				t.Errorf("typing started an operation: opKind = %v", mm.opKind)
			}
			if mm.detailRemote != nil || mm.errMsg != "" {
				t.Errorf("typing triggered a command: detailRemote = %v, errMsg = %q", mm.detailRemote, mm.errMsg)
			}
		})
	}
}

func TestFocusedInputLeaveAndSubmitKeys(t *testing.T) {
	cases := []struct {
		name  string
		setup func(m *Model)
		key   tea.KeyMsg
		check func(t *testing.T, m *Model, cmd tea.Cmd)
	}{
		{"explore enter searches", openExploreWithResult, tea.KeyMsg{Type: tea.KeyEnter}, func(t *testing.T, m *Model, cmd tea.Cmd) {
			if m.exploreQuery != typedText || m.exploreInput.Focused() {
				t.Errorf("exploreQuery = %q, focused = %v", m.exploreQuery, m.exploreInput.Focused())
			}
			if cmd == nil || m.opKind != opSearch {
				t.Errorf("enter did not start a search: opKind = %v", m.opKind)
			}
			drainOp(m)
		}},
		{"explore esc clears", openExploreWithResult, tea.KeyMsg{Type: tea.KeyEsc}, func(t *testing.T, m *Model, cmd tea.Cmd) {
			if m.exploreInput.Value() != "" || m.exploreInput.Focused() || m.screen != screenExplore {
				t.Errorf("value = %q, focused = %v, screen = %v", m.exploreInput.Value(), m.exploreInput.Focused(), m.screen)
			}
		}},
		{"explore tab", openExploreWithResult, tea.KeyMsg{Type: tea.KeyTab}, func(t *testing.T, m *Model, cmd tea.Cmd) {
			if m.screen != screenSettings || m.exploreInput.Focused() {
				t.Errorf("screen = %v, focused = %v", m.screen, m.exploreInput.Focused())
			}
		}},
		{"explore shift+tab", openExploreWithResult, tea.KeyMsg{Type: tea.KeyShiftTab}, func(t *testing.T, m *Model, cmd tea.Cmd) {
			if m.screen != screenAddons || m.exploreInput.Focused() {
				t.Errorf("screen = %v, focused = %v", m.screen, m.exploreInput.Focused())
			}
		}},
		{"explore ctrl+c", openExploreWithResult, tea.KeyMsg{Type: tea.KeyCtrlC}, func(t *testing.T, m *Model, cmd tea.Cmd) {
			if !isQuitCmd(cmd) {
				t.Error("ctrl+c did not quit")
			}
		}},
		{"addons search enter", openAddonsSearch, tea.KeyMsg{Type: tea.KeyEnter}, func(t *testing.T, m *Model, cmd tea.Cmd) {
			if m.filterQuery != typedText || m.searching || m.searchInput.Focused() {
				t.Errorf("filterQuery = %q, searching = %v", m.filterQuery, m.searching)
			}
		}},
		{"addons search esc", openAddonsSearch, tea.KeyMsg{Type: tea.KeyEsc}, func(t *testing.T, m *Model, cmd tea.Cmd) {
			if m.filterQuery != "" || m.searching || m.searchInput.Focused() {
				t.Errorf("filterQuery = %q, searching = %v", m.filterQuery, m.searching)
			}
		}},
		{"add enter", openAddDialog, tea.KeyMsg{Type: tea.KeyEnter}, func(t *testing.T, m *Model, cmd tea.Cmd) {
			if m.addField != 1 || m.input.Focused() || m.input.Value() != typedText {
				t.Errorf("addField = %d, focused = %v, value = %q", m.addField, m.input.Focused(), m.input.Value())
			}
		}},
		{"add tab", openAddDialog, tea.KeyMsg{Type: tea.KeyTab}, func(t *testing.T, m *Model, cmd tea.Cmd) {
			if m.addField != 1 || m.input.Focused() {
				t.Errorf("addField = %d, focused = %v", m.addField, m.input.Focused())
			}
		}},
		{"add down", openAddDialog, tea.KeyMsg{Type: tea.KeyDown}, func(t *testing.T, m *Model, cmd tea.Cmd) {
			if m.addField != 1 || m.input.Focused() {
				t.Errorf("addField = %d, focused = %v", m.addField, m.input.Focused())
			}
		}},
		{"add shift+tab", openAddDialog, tea.KeyMsg{Type: tea.KeyShiftTab}, func(t *testing.T, m *Model, cmd tea.Cmd) {
			if m.addField != 2 || m.input.Focused() {
				t.Errorf("addField = %d, focused = %v", m.addField, m.input.Focused())
			}
		}},
		{"add up", openAddDialog, tea.KeyMsg{Type: tea.KeyUp}, func(t *testing.T, m *Model, cmd tea.Cmd) {
			if m.addField != 2 || m.input.Focused() {
				t.Errorf("addField = %d, focused = %v", m.addField, m.input.Focused())
			}
		}},
		{"add esc", openAddDialog, tea.KeyMsg{Type: tea.KeyEsc}, func(t *testing.T, m *Model, cmd tea.Cmd) {
			if m.screen != screenAddons || m.input.Focused() {
				t.Errorf("screen = %v, focused = %v", m.screen, m.input.Focused())
			}
		}},
		{"add ctrl+c", openAddDialog, tea.KeyMsg{Type: tea.KeyCtrlC}, func(t *testing.T, m *Model, cmd tea.Cmd) {
			if !isQuitCmd(cmd) {
				t.Error("ctrl+c did not quit")
			}
		}},
		{"settings rename enter", openSettingsRename, tea.KeyMsg{Type: tea.KeyEnter}, func(t *testing.T, m *Model, cmd tea.Cmd) {
			if m.renaming || m.renameInput.Focused() || m.opKind != opRename {
				t.Errorf("renaming = %v, opKind = %v", m.renaming, m.opKind)
			}
			drainOp(m)
		}},
		{"settings rename esc", openSettingsRename, tea.KeyMsg{Type: tea.KeyEsc}, func(t *testing.T, m *Model, cmd tea.Cmd) {
			if m.renaming || m.renameInput.Focused() || m.screen != screenSettings || m.opKind != opNone {
				t.Errorf("renaming = %v, screen = %v, opKind = %v", m.renaming, m.screen, m.opKind)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mm := loadedModel(t, 120, 30)
			mm.deps.RegistryPath = filepath.Join(t.TempDir(), "registry.json")
			tc.setup(mm)
			typeText(mm, typedText)

			_, cmd := mm.Update(tc.key)
			tc.check(t, mm, cmd)
		})
	}
}

func TestCommandKeysWorkWithoutFocusedInput(t *testing.T) {
	cases := []struct {
		name  string
		setup func(m *Model)
		key   string
		check func(t *testing.T, m *Model, cmd tea.Cmd)
	}{
		{"explore q quits", openExploreWithResult, "q", func(t *testing.T, m *Model, cmd tea.Cmd) {
			if !isQuitCmd(cmd) {
				t.Error("q did not quit")
			}
		}},
		{"explore 1 opens addons", openExploreWithResult, "1", func(t *testing.T, m *Model, cmd tea.Cmd) {
			if m.screen != screenAddons {
				t.Errorf("screen = %v, want addons", m.screen)
			}
		}},
		{"explore 3 opens settings", openExploreWithResult, "3", func(t *testing.T, m *Model, cmd tea.Cmd) {
			if m.screen != screenSettings {
				t.Errorf("screen = %v, want settings", m.screen)
			}
		}},
		{"explore d opens details", openExploreWithResult, "d", func(t *testing.T, m *Model, cmd tea.Cmd) {
			if m.screen != screenDetails || m.detailRemote == nil {
				t.Errorf("screen = %v, detailRemote = %v", m.screen, m.detailRemote)
			}
		}},
		{"explore / focuses search", openExploreWithResult, "/", func(t *testing.T, m *Model, cmd tea.Cmd) {
			if !m.exploreInput.Focused() || m.exploreInput.Value() != "" {
				t.Errorf("focused = %v, value = %q", m.exploreInput.Focused(), m.exploreInput.Value())
			}
		}},
		{"explore a installs", openExploreWithResult, "a", func(t *testing.T, m *Model, cmd tea.Cmd) {
			if cmd == nil || m.opKind != opAdd {
				t.Errorf("opKind = %v, want add", m.opKind)
			}
			drainOp(m)
		}},
		{"add q quits off the reference field", func(m *Model) {
			openAddDialog(m)
			m.Update(tea.KeyMsg{Type: tea.KeyTab})
		}, "q", func(t *testing.T, m *Model, cmd tea.Cmd) {
			if !isQuitCmd(cmd) {
				t.Error("q did not quit")
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mm := loadedModel(t, 120, 30)
			tc.setup(mm)
			mm.exploreInput.Blur()

			_, cmd := mm.Update(keyMsg(tc.key))
			tc.check(t, mm, cmd)
		})
	}
}

func TestExtractionPolicyAndFallbackNotice(t *testing.T) {
	m := loadedModel(t, 120, 30)
	m.deps.Config.Security = config.Security{Sandbox: "required", Seccomp: "auto"}
	svc := m.buildService(nil)
	if svc.Security != m.deps.Config.Security {
		t.Fatalf("service security = %+v, want %+v", svc.Security, m.deps.Config.Security)
	}

	const fallback = "warning: extraction isolation: seccomp unavailable"
	runTestOp(t, m, nil,
		service.Event{Stage: service.StageInfo, Message: "extract backend: bubblewrap"},
		service.Event{Stage: service.StageWarning, Message: fallback},
		service.Event{Stage: service.StageDone, Message: "installed"},
	)
	if !strings.Contains(m.View(), "! "+fallback) {
		t.Fatalf("missing extraction fallback notice: %s", m.View())
	}
}
