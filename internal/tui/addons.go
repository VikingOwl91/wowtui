package tui

import (
	"sort"
	"strconv"

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
		{Title: "Name", Width: 26},
		{Title: "Status", Width: 13},
		{Title: "Installed", Width: 16},
		{Title: "Latest", Width: 16},
		{Title: "Provider", Width: 9},
		{Title: "Flavor", Width: 11},
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
	var out []addonRow
	for _, r := range m.rows {
		if !m.matchesInstallFilter(r) {
			continue
		}
		switch m.filter {
		case filterManaged:
			if r.Managed {
				out = append(out, r)
			}
		case filterUnmanaged:
			if !r.Managed {
				out = append(out, r)
			}
		default:
			out = append(out, r)
		}
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
		rows = append(rows, table.Row{r.Name, r.Status, r.Installed, r.Latest, r.Provider, r.Flavor})
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
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab", "2":
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
		m.filter = (m.filter + 1) % 3
		m.rebuildTable()
		return m, nil
	case "i":
		if m.reg != nil && len(m.reg.Installations) > 0 {
			m.installFilter++
			if m.installFilter >= len(m.reg.Installations) {
				m.installFilter = -1
			}
			m.rebuildTable()
		}
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
	counts := m.countRows()
	summary := dimStyle.Render(
		"install: " + installName + " · " +
			"filter: " + filterName + " · " +
			strconv.Itoa(counts.total) + " addons · " +
			strconv.Itoa(counts.managed) + " managed · " +
			strconv.Itoa(counts.updates) + " updates")

	return summary + "\n" + m.table.View()
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
