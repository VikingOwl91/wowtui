package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handleAddKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		if m.addField != 0 {
			return m, tea.Quit
		}
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.screen = screenAddons
		m.input.Blur()
		return m, nil
	case "tab", "down":
		m.addField = (m.addField + 1) % 3
		return m, m.syncAddField()
	case "shift+tab", "up":
		m.addField = (m.addField + 2) % 3
		return m, m.syncAddField()
	case "enter":
		if m.addField < 2 {
			m.addField++
			return m, m.syncAddField()
		}
		return m.submitAdd()
	case "left", "right":
		if m.addField == 1 {
			if n := m.installCount(); n > 0 {
				m.addInstall = cycleIndex(m.addInstall, n, msg.String() == "right")
			}
			return m, nil
		}
		if m.addField == 2 {
			m.addSource = cycleIndex(m.addSource, len(sourceOptions), msg.String() == "right")
			return m, nil
		}
	}

	if m.addField == 0 {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) syncAddField() tea.Cmd {
	if m.addField == 0 {
		m.input.Focus()
		return textinput.Blink
	}
	m.input.Blur()
	return nil
}

func (m *Model) submitAdd() (tea.Model, tea.Cmd) {
	if m.busy {
		return m, nil
	}
	ref := strings.TrimSpace(m.input.Value())
	if ref == "" {
		m.errMsg = "enter a reference (github:Owner/Repo or a URL)"
		return m, nil
	}
	if m.installCount() == 0 {
		m.errMsg = "no installations, run scan first"
		return m, nil
	}
	source := sourceOptions[m.addSource]
	if source == "auto" {
		source = ""
	}
	m.screen = screenAddons
	m.input.Blur()
	return m, m.startAdd(ref, m.addInstall, source)
}

func (m *Model) installCount() int {
	if m.reg == nil {
		return 0
	}
	return len(m.reg.Installations)
}

func cycleIndex(i, n int, forward bool) int {
	if n <= 0 {
		return 0
	}
	if forward {
		return (i + 1) % n
	}
	return (i + n - 1) % n
}

func (m *Model) addView() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Add addon") + "\n\n")

	b.WriteString(fieldRow(m.addField == 0, "Reference:    ", m.input.View()) + "\n")

	installLabel := dimStyle.Render("none, run scan first")
	if m.installCount() > 0 {
		if m.addInstall >= m.installCount() {
			m.addInstall = 0
		}
		installLabel = cycleLabel(m.installLabels[m.addInstall], m.addField == 1)
	}
	b.WriteString(fieldRow(m.addField == 1, "Installation: ", installLabel) + "\n")
	b.WriteString(fieldRow(m.addField == 2, "Source:       ", cycleLabel(sourceOptions[m.addSource], m.addField == 2)) + "\n")
	b.WriteString("\n" + dimStyle.Render("tab next field · left/right change · enter submit · esc cancel"))
	return b.String()
}

func fieldRow(active bool, label, value string) string {
	marker := "  "
	if active {
		marker = "> "
	}
	return marker + labelStyle.Render(label) + value
}

func cycleLabel(v string, active bool) string {
	if active {
		return "< " + v + " >"
	}
	return v
}
