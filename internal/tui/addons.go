package tui

import (
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"somegit.dev/vikingowl/wowtui/internal/provider"
	"somegit.dev/vikingowl/wowtui/internal/service"
)

func (m *Model) initTable() {
	m.table = table.New(
		table.WithColumns(addonColumns()),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	m.table.KeyMap.LineUp.SetKeys("up", "k")
	m.table.KeyMap.LineDown.SetKeys("down", "j")
}

func addonColumns() []table.Column {
	return []table.Column{
		{Title: "Addon", Width: 24},
		{Title: "Status", Width: 12},
		{Title: "Installed", Width: 13},
		{Title: "Latest", Width: 13},
		{Title: "Provider", Width: 10},
		{Title: "Author", Width: 16},
		{Title: "Game", Width: 10},
	}
}

func (m *Model) resizeTable() {
	if m.height > 8 {
		m.table.SetHeight(m.height - 8)
	}
}

func (m *Model) buildRows() {
	m.rows = nil
	if m.reg == nil {
		return
	}

	managed := map[string]int{}
	for i := range m.reg.Addons {
		a := &m.reg.Addons[i]
		for _, f := range a.Folders {
			managed[a.InstallPath+"/"+f] = i
		}
	}

	for _, inst := range m.reg.Installations {
		for _, f := range inst.Folders {
			row := addonRow{
				InstallPath: inst.Path,
				Flavor:      inst.Flavor,
				Name:        f.Name,
				Installed:   f.Version,
				Status:      "not managed",
			}
			if idx, ok := managed[inst.Path+"/"+f.Name]; ok {
				a := m.reg.Addons[idx]
				row.Managed = true
				row.Ref = a.Ref
				row.Provider = a.Provider
				row.Author = a.Author
				row.Installed = a.Version
				row.Status = "managed"
			}
			m.rows = append(m.rows, row)
		}
	}

	sort.Slice(m.rows, func(i, j int) bool {
		if m.rows[i].Flavor != m.rows[j].Flavor {
			return m.rows[i].Flavor < m.rows[j].Flavor
		}
		return m.rows[i].Name < m.rows[j].Name
	})
}

func (m *Model) filteredRows() []addonRow {
	type scoredRow struct {
		row   addonRow
		score int
	}
	var scored []scoredRow
	for _, r := range m.rows {
		if !m.matchesInstallFilter(r) {
			continue
		}
		if m.gameFilter != "" && m.gameFilter != "all" && r.Flavor != m.gameFilter {
			continue
		}
		if m.filterQuery != "" {
			score := fuzzyScore(m.filterQuery, strings.Join([]string{r.Name, r.Provider, r.Flavor, r.Author}, " "))
			if score < 0 {
				continue
			}
			scored = append(scored, scoredRow{row: r, score: score})
			continue
		}
		switch m.filter {
		case filterManaged:
			if r.Managed {
				scored = append(scored, scoredRow{row: r})
			}
		case filterUnmanaged:
			if !r.Managed {
				scored = append(scored, scoredRow{row: r})
			}
		default:
			scored = append(scored, scoredRow{row: r})
		}
	}
	if m.filterQuery != "" {
		sort.SliceStable(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	}
	out := make([]addonRow, 0, len(scored))
	for _, item := range scored {
		out = append(out, item.row)
	}
	return out
}

func (m *Model) matchesInstallFilter(r addonRow) bool {
	if m.installFilter < 0 || m.reg == nil {
		return true
	}
	if m.installFilter >= len(m.reg.Installations) {
		return true
	}
	return r.InstallPath == m.reg.Installations[m.installFilter].Path
}

func (m *Model) rebuildTable() {
	visible := m.filteredRows()
	rows := make([]table.Row, 0, len(visible))
	for _, r := range visible {
		rows = append(rows, table.Row{r.Name, r.Status, r.Installed, r.Latest, r.Provider, r.Author, r.Flavor})
	}
	m.table.SetRows(rows)
}

func (m *Model) mergeStatuses(statuses []service.UpdateStatus) {
	byRef := map[string]service.UpdateStatus{}
	for _, s := range statuses {
		byRef[s.Addon.Ref] = s
	}
	for i := range m.rows {
		row := &m.rows[i]
		if !row.Managed {
			continue
		}
		st, ok := byRef[row.Ref]
		if !ok {
			continue
		}
		switch {
		case st.Error != nil:
			row.Status = "error"
			row.Latest = "-"
		case st.HasUpdate:
			row.Status = "update"
			row.Latest = shortVersion(st.Latest)
		default:
			row.Status = "current"
			row.Latest = shortVersion(st.Latest)
		}
	}
	m.rebuildTable()
}

func shortVersion(release provider.Release) string {
	v := release.Tag
	if len(v) > 16 {
		return v[:12]
	}
	return v
}

func (m *Model) handleAddonsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.searching {
		switch msg.String() {
		case "esc":
			m.searching = false
			m.searchInput.Blur()
			return m, nil
		case "enter":
			m.filterQuery = strings.TrimSpace(m.searchInput.Value())
			m.searching = false
			m.searchInput.Blur()
			m.rebuildTable()
			return m, nil
		default:
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			return m, cmd
		}
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab", "2":
		m.screen = screenExplore
		m.exploreInput.Focus()
		return m, nil
	case "3":
		m.screen = screenSettings
		return m, nil
	case "1":
		return m, nil
	case "r":
		if m.busy {
			return m, nil
		}
		return m, m.startCheck()
	case "s":
		if m.busy {
			return m, nil
		}
		return m, m.startScan()
	case "u":
		if m.busy {
			return m, nil
		}
		row, ok := m.selectedRow()
		if !ok || !row.Managed {
			m.errMsg = "select a managed addon to update"
			return m, nil
		}
		return m, m.startUpdate(row.Ref)
	case "U":
		if m.busy {
			return m, nil
		}
		return m, m.startUpdateAll()
	case "a":
		m.screen = screenAdd
		m.addField = 0
		m.addInstall = 0
		m.addSource = 0
		m.input.SetValue("")
		m.input.Focus()
		return m, textinput.Blink
	case "f":
		m.searching = true
		m.searchInput.SetValue(m.filterQuery)
		m.searchInput.Focus()
		return m, textinput.Blink
	case "i":
		m.gameFilter = cycleGameFilter(m.gameFilter, m.rows)
		m.rebuildTable()
		return m, nil
	case "I":
		if m.reg != nil && len(m.reg.Installations) > 0 {
			m.installFilter++
			if m.installFilter >= len(m.reg.Installations) {
				m.installFilter = -1
			}
			m.rebuildTable()
		}
		return m, nil
	case "backspace":
		m.filterQuery = ""
		m.rebuildTable()
		return m, nil
	case "d", "enter":
		row, ok := m.selectedRow()
		if !ok {
			return m, nil
		}
		m.detailRow = row
		m.screen = screenDetails
		return m, nil
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *Model) addonsView() string {
	if m.reg == nil || len(m.reg.Installations) == 0 {
		return dimStyle.Render("No installations found. Press s to scan.")
	}

	filterName := []string{"all", "managed", "not managed"}[m.filter]
	installName := "all installations"
	if m.installFilter >= 0 && m.installFilter < len(m.installLabels) {
		installName = m.installLabels[m.installFilter]
	}
	gameName := m.gameFilter
	if gameName == "" || gameName == "all" {
		gameName = "all game versions"
	}
	counts := m.countRows()
	search := ""
	if m.filterQuery != "" {
		search = " · search: " + m.filterQuery
	}
	summary := dimStyle.Render(
		"game: " + gameName + " · install: " + installName + " · " +
			"status: " + filterName + search + " · " +
			strconv.Itoa(counts.total) + " addons · " +
			strconv.Itoa(counts.managed) + " managed · " +
			strconv.Itoa(counts.updates) + " updates")

	searchBar := ""
	if m.searching {
		searchBar = searchBox.Render(m.searchInput.View()) + "\n"
	}
	return searchBar + summary + "\n" + m.table.View()
}

func cycleGameFilter(current string, rows []addonRow) string {
	versions := []string{"all"}
	seen := map[string]bool{"all": true}
	for _, row := range rows {
		if !seen[row.Flavor] {
			seen[row.Flavor] = true
			versions = append(versions, row.Flavor)
		}
	}
	idx := 0
	for i, version := range versions {
		if version == current {
			idx = i
			break
		}
	}
	return versions[(idx+1)%len(versions)]
}

type rowCounts struct {
	total   int
	managed int
	updates int
}

func (m *Model) countRows() rowCounts {
	var c rowCounts
	for _, r := range m.rows {
		c.total++
		if r.Managed {
			c.managed++
		}
		if r.Status == "update" {
			c.updates++
		}
	}
	return c
}
