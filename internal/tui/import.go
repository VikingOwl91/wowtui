package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"somegit.dev/vikingowl/wowtui/internal/index"
	"somegit.dev/vikingowl/wowtui/internal/service"
)

type importPhase int

const (
	importChoose importPhase = iota
	importReview
	importResults
)

type importFolder struct{ path, name string }

func importKey(c service.MappingCandidate) importFolder { return importFolder{c.InstallPath, c.Folder} }

type importState struct {
	phase      importPhase
	candidates []service.MappingCandidate
	selected   map[importFolder]service.MappingCandidate
	choices    map[importFolder]int
	reviewed   []service.MappingCandidate
	results    []service.ImportResult
	focus      importFolder
	index      *index.Index
	cursor     int
	cancel     context.CancelFunc
}

func (m *Model) openImport(focus importFolder) tea.Cmd {
	if m.busy || m.reg == nil {
		return nil
	}
	m.screen = screenImport
	m.importing = importState{focus: focus, selected: make(map[importFolder]service.MappingCandidate), choices: make(map[importFolder]int)}
	m.busyMsg = "matching installed folders..."
	load := m.deps.LoadImportIndex
	return m.startOp(opImportMap, func(svc *service.Service) opResult {
		if load != nil {
			idx, err := load()
			if err != nil {
				svc.OnEvent(service.Event{Stage: service.StageWarning, Message: "Index unavailable: " + err.Error() + ". Using built-in fingerprints and installed metadata."})
			} else {
				svc.Index = idx
			}
		}
		candidates, err := svc.MapUnmanaged(nil)
		return opResult{candidates: candidates, importIndex: svc.Index, err: err}
	})
}

func (m *Model) inImportScope(path, flavor string) bool {
	return (m.gameFilter == "" || m.gameFilter == "all" || m.gameFilter == flavor) && m.matchesInstallFilter(addonRow{InstallPath: path})
}

func (m *Model) finishImportMap(result opResult) {
	state := &m.importing
	state.index = result.importIndex
	for _, c := range result.candidates {
		if c.Availability != service.AvailabilityActive || !m.inImportScope(c.InstallPath, c.Flavor) {
			continue
		}
		k := importKey(c)
		if k == state.focus && state.choices[k] == 0 {
			state.cursor = len(state.candidates)
		}
		state.choices[k]++
		state.candidates = append(state.candidates, c)
	}
	if state.focus.name != "" && state.choices[state.focus] == 0 && result.err == nil {
		m.notices = append(m.notices, notice{kind: noticeWarning, text: "No active source for " + importText(state.focus.name)})
	}
}

func (m *Model) finishImportApply(result opResult) {
	state := &m.importing
	state.cancel()
	state.cancel = nil
	state.phase = importResults
	state.cursor = 0
	state.results = result.imported
}

func importFolders(c service.MappingCandidate) []string {
	if len(c.PackageFolders) > 0 {
		return c.PackageFolders
	}
	return []string{c.Folder}
}

func (s *importState) choose(c service.MappingCandidate, enable bool) {
	for _, name := range importFolders(c) {
		k := importFolder{c.InstallPath, name}
		if old, ok := s.selected[k]; ok {
			for _, oldName := range importFolders(old) {
				delete(s.selected, importFolder{old.InstallPath, oldName})
			}
		}
	}
	if enable {
		for _, name := range importFolders(c) {
			s.selected[importFolder{c.InstallPath, name}] = c
		}
	}
}

func (s *importState) unambiguous(c service.MappingCandidate) bool {
	for _, name := range importFolders(c) {
		if s.choices[importFolder{c.InstallPath, name}] != 1 {
			return false
		}
	}
	return true
}

func (s *importState) toggleSingles() {
	selectAll := false
	for _, c := range s.candidates {
		if s.unambiguous(c) && s.selected[importKey(c)].Reference == "" {
			selectAll = true
			break
		}
	}
	for _, c := range s.candidates {
		chosen := s.selected[importKey(c)].Reference == c.Reference
		if chosen != selectAll && s.unambiguous(c) {
			s.choose(c, selectAll)
		}
	}
}

func (m *Model) handleImportKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := &m.importing
	key := msg.String()
	if m.busy {
		if s.cancel != nil && (key == "esc" || key == "ctrl+c") {
			s.cancel()
			m.busyMsg = "stopping import..."
		}
		return m, nil
	}
	switch key {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc":
		if s.phase == importReview {
			s.phase = importChoose
			s.cursor = 0
		} else {
			m.screen = screenAddons
		}
	case "up", "k", "down", "j", "pgup", "pgdown", "home", "end":
		n := len(s.candidates)
		if s.phase == importReview {
			n = len(s.reviewed)
		} else if s.phase == importResults {
			n = len(s.results)
		}
		step := max(1, m.height-12-len(m.noticeLines()))
		switch key {
		case "up", "k":
			s.cursor--
		case "down", "j":
			s.cursor++
		case "pgup":
			s.cursor -= step
		case "pgdown":
			s.cursor += step
		case "home":
			s.cursor = 0
		case "end":
			s.cursor = n - 1
		}
		s.cursor = max(0, min(s.cursor, n-1))
	case " ":
		if s.phase == importChoose && len(s.candidates) > 0 {
			c := s.candidates[s.cursor]
			s.choose(c, s.selected[importKey(c)].Reference != c.Reference)
		}
	case "d":
		candidates := s.candidates
		if s.phase == importReview {
			candidates = s.reviewed
		}
		if s.phase != importResults && s.cursor < len(candidates) && len(candidates[s.cursor].PackageFolders) > 0 {
			m.showImportFiles(candidates[s.cursor])
		}
	case "a":
		if s.phase == importChoose {
			s.toggleSingles()
		}
	case "enter":
		if s.phase == importChoose && len(s.selected) > 0 {
			s.reviewed = nil
			for _, c := range s.candidates {
				if s.selected[importKey(c)].Reference == c.Reference {
					s.reviewed = append(s.reviewed, c)
				}
			}
			s.phase = importReview
			s.cursor = 0
		}
	case "y":
		if s.phase == importReview && len(s.reviewed) > 0 {
			selected := slices.Clone(s.reviewed)
			idx := s.index
			ctx, cancel := context.WithCancel(context.Background())
			s.cancel = cancel
			m.busyMsg = "importing selected folders..."
			return m, m.startOp(opImportApply, func(svc *service.Service) opResult {
				svc.Index = idx
				imported, err := svc.Import(ctx, selected)
				return opResult{imported: imported, err: err}
			})
		}
	case "r":
		if s.phase != importReview {
			return m, m.openImport(importFolder{})
		}
	}
	return m, nil
}

func (m *Model) importHelp() string {
	switch m.importing.phase {
	case importReview:
		return "↑↓ move · d package details · y confirm import · esc edit"
	case importResults:
		return "↑/↓ scroll · r refresh matches · esc addons"
	default:
		return "↑↓ move · space choose · a single matches · d details · enter review · esc back"
	}
}

func (m *Model) importView() string {
	s := &m.importing
	width := max(1, m.width-contentPanel.GetHorizontalFrameSize())
	title := "Import installed addons"
	scope := "all installations"
	if m.reg != nil && m.installFilter >= 0 && m.installFilter < len(m.reg.Installations) {
		scope = m.reg.Installations[m.installFilter].Path
	}
	if m.gameFilter != "" && m.gameFilter != "all" {
		scope = m.gameFilter + " · " + scope
	}
	summary := "Matching folders..."
	var lines []string
	var currentPath, currentSource, currentReason, firstChange string
	switch s.phase {
	case importResults:
		title = "Import results"
		imported := 0
		for i, r := range s.results {
			outcome := "imported"
			if r.Err == nil {
				imported++
			} else {
				outcome = r.Err.Error()
			}
			folders := strings.Join(r.Folders, ", ")
			lines = append(lines, folders+" · "+r.Reference+" · "+outcome)
			if i == s.cursor {
				currentPath = r.InstallPath
				currentSource = r.Reference
				currentReason = outcome
			}
		}
		summary = fmt.Sprintf("%d imported · %d failed (sources)", imported, len(s.results)-imported)
	case importChoose, importReview:
		candidates := s.candidates
		if s.phase == importReview {
			title = "Review import"
			summary = fmt.Sprintf("%s selected · y to register for updates", folderCount(len(s.selected), ""))
			candidates = s.reviewed
		} else if !m.busy {
			unmanaged := 0
			for _, r := range m.rows {
				if !r.Managed && m.inImportScope(r.InstallPath, r.Flavor) {
					unmanaged++
				}
			}
			summary = fmt.Sprintf("%d selected · %s · %d without an active match", len(s.selected), folderCount(len(s.choices), "matched "), max(0, unmanaged-len(s.choices)))
		}
		for i, c := range candidates {
			mark := "[ ] "
			if s.selected[importKey(c)].Reference == c.Reference {
				mark = "[x] "
			}
			lines = append(lines, mark+c.Folder+" · "+c.Flavor+" · "+c.Reference)
			if i == s.cursor {
				currentPath = c.InstallPath
				currentSource = c.Reference
				currentReason = c.Reason
				if len(c.PackageFolders) > 0 {
					currentReason = fmt.Sprintf("release %s · %s · %s · d details", c.ReleaseVersion, folderCount(len(c.PackageFolders), ""), changeCount(len(c.FileChanges)))
				}
				if len(c.FileChanges) > 0 {
					change := c.FileChanges[0]
					firstChange = "\n" + truncateText(importText(change.Kind+"  "+change.Path), width)
				}
			}
		}
	}
	var b strings.Builder
	fmt.Fprintln(&b, headerStyle.Render(title))
	fmt.Fprintln(&b, truncateText(importText(summary), width))
	if s.phase == importChoose {
		fmt.Fprintln(&b, truncateText(importText("Scope: "+scope), width))
	}
	if len(lines) == 0 {
		if !m.busy && s.phase == importChoose {
			fmt.Fprint(&b, "No folders with an active source. Press s on Addons to rescan.")
		}
		return strings.TrimSuffix(b.String(), "\n")
	}
	// The focused entry exposes the complete path and source on multiple lines.
	detail := lipgloss.NewStyle().Width(width).Render(importText(currentPath) + "\n" + importText(currentSource) + " · " + importText(currentReason) + firstChange)
	visible := max(1, m.height-8-lipgloss.Height(detail)-len(m.noticeLines()))
	start := max(0, min(s.cursor-visible/2, len(lines)-visible))
	for i := start; i < min(start+visible, len(lines)); i++ {
		prefix := "  "
		if i == s.cursor {
			prefix = "› "
		}
		line := truncateText(importText(prefix+lines[i]), width)
		if i == s.cursor {
			line = labelStyle.Render(line)
		}
		fmt.Fprintln(&b, line)
	}
	fmt.Fprintf(&b, "%d/%d\n%s", s.cursor+1, len(lines), detail)
	return b.String()
}

func importText(v string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, v)
}

func folderCount(n int, qualifier string) string {
	if n == 1 {
		return "1 " + qualifier + "folder"
	}
	return fmt.Sprintf("%d %sfolders", n, qualifier)
}
