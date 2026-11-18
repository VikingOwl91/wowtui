package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"somegit.dev/vikingowl/wowtui/internal/provider"
)

func (m *Model) initExploreTable() {
	m.exploreTable = table.New(
		table.WithColumns([]table.Column{
			{Title: "Addon", Width: 30},
			{Title: "Author", Width: 24},
			{Title: "Provider", Width: 14},
			{Title: "Reference", Width: 28},
		}),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	m.exploreTable.KeyMap.LineUp.SetKeys("up", "k")
	m.exploreTable.KeyMap.LineDown.SetKeys("down", "j")
}

func (m *Model) handleExploreKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab", "3":
		m.screen = screenSettings
		m.exploreInput.Blur()
		return m, nil
	case "shift+tab", "1":
		m.screen = screenAddons
		m.exploreInput.Blur()
		return m, nil
	case "esc":
		m.exploreInput.SetValue("")
		m.exploreInput.Blur()
		return m, nil
	case "enter":
		if strings.TrimSpace(m.exploreInput.Value()) == "" {
			m.exploreInput.Focus()
			return m, textinput.Blink
		}
		m.exploreQuery = strings.TrimSpace(m.exploreInput.Value())
		m.exploreInput.Blur()
		if m.busy {
			return m, nil
		}
		return m, m.startSearch(m.exploreQuery)
	case "a":
		if m.busy {
			return m, nil
		}
		remote, ok := m.selectedRemote()
		if !ok {
			m.errMsg = "select an addon result first"
			return m, nil
		}
		if m.installCount() == 0 {
			m.errMsg = "no installations, scan the Addons tab first"
			return m, nil
		}
		return m, m.startAdd(remote.Provider+":"+remote.ID, 0, "")
	case "d":
		if remote, ok := m.selectedRemote(); ok {
			copy := remote
			m.detailRemote = &copy
			m.screen = screenDetails
			return m, nil
		}
	case "/":
		m.exploreInput.Focus()
		return m, textinput.Blink
	}
	if m.exploreInput.Focused() {
		var cmd tea.Cmd
		m.exploreInput, cmd = m.exploreInput.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.exploreTable, cmd = m.exploreTable.Update(msg)
	m.exploreCursor = m.exploreTable.Cursor()
	return m, cmd
}

func (m *Model) selectedRemote() (provider.RemoteAddon, bool) {
	idx := m.exploreTable.Cursor()
	if idx < 0 || idx >= len(m.exploreResults) {
		return provider.RemoteAddon{}, false
	}
	return m.exploreResults[idx], true
}

func (m *Model) rebuildExploreTable() {
	rows := make([]table.Row, 0, len(m.exploreResults))
	for _, result := range m.exploreResults {
		author := result.Author
		if author == "" {
			author = "unknown"
		}
		rows = append(rows, table.Row{result.Name, author, result.Provider, result.ID})
	}
	m.exploreTable.SetRows(rows)
}

func (m *Model) resizeExploreTable() {
	if m.height > 10 {
		m.exploreTable.SetHeight(m.height - 11 - len(m.noticeLines()))
	}
}

func (m *Model) exploreView() string {
	m.rebuildExploreTable()
	version := m.selectedFlavor()
	header := lipgloss.JoinHorizontal(lipgloss.Top,
		sectionTitle.Render("Explore"),
		dimStyle.Render(fmt.Sprintf("  %s catalog", version)),
	)
	search := searchBox.Render(m.exploreInput.View())
	info := dimStyle.Render("Search active providers for an addon. Results can be inspected or installed.")
	if len(m.exploreResults) == 0 {
		return header + "\n\n" + search + "\n" + info + "\n\n" + emptyPanel.Render("No results yet\nEnter a search such as WeakAuras or Details")
	}
	return header + "\n\n" + search + "\n" + info + "\n\n" + m.exploreTable.View()
}
