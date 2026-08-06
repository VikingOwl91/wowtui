package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"somegit.dev/vikingowl/wowtui/internal/service"
)

func (m *Model) handleSettingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.renaming {
		return m.handleRenameKey(msg)
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "tab", "1":
		m.screen = screenAddons
		return m, nil
	case "up", "k":
		if m.settingsCursor > 0 {
			m.settingsCursor--
		}
		return m, nil
	case "down", "j":
		if m.reg != nil && m.settingsCursor < len(m.reg.Installations)-1 {
			m.settingsCursor++
		}
		return m, nil
	case "r":
		if m.reg != nil && len(m.reg.Installations) > 0 && m.settingsCursor < len(m.reg.Installations) {
			m.renaming = true
			m.renameInput.SetValue(m.reg.Installations[m.settingsCursor].Title)
			m.renameInput.Focus()
			return m, textinput.Blink
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) handleRenameKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.renaming = false
		m.renameInput.Blur()
		return m, nil
	case "enter":
		if m.busy {
			return m, nil
		}
		title := strings.TrimSpace(m.renameInput.Value())
		inst := m.reg.Installations[m.settingsCursor]
		m.renaming = false
		m.renameInput.Blur()
		return m, m.startRename(inst.Path, title)
	default:
		var cmd tea.Cmd
		m.renameInput, cmd = m.renameInput.Update(msg)
		return m, cmd
	}
}

func (m *Model) startRename(path, title string) tea.Cmd {
	m.busyMsg = "renaming installation..."
	return m.startOp(opRename, func(svc *service.Service) opResult {
		return opResult{err: svc.SetInstallTitle(path, title)}
	})
}

func (m *Model) settingsView() string {
	lines := []string{headerStyle.Render("Settings"), ""}

	lines = append(lines, labelStyle.Render("Registry:     ")+m.deps.RegistryPath)
	lines = append(lines, labelStyle.Render("Backups:      ")+m.deps.BackupRoot)

	token := "not set"
	if m.deps.Config.Providers.GitHub.Token != "" {
		token = "set"
	}
	lines = append(lines, labelStyle.Render("GitHub token: ")+token)

	lines = append(lines, "", labelStyle.Render("Providers (priority order):"))
	for i, name := range m.deps.Config.Providers.Order() {
		state := "disabled"
		if m.deps.Config.Providers.Enabled(name) {
			state = "enabled"
		}
		lines = append(lines, "  "+strconv.Itoa(i+1)+". "+name+"  "+state)
	}

	lines = append(lines, "", labelStyle.Render("Installations:")+" "+dimStyle.Render("up/down select · r rename"))
	if m.reg == nil || len(m.reg.Installations) == 0 {
		lines = append(lines, dimStyle.Render("  none, press s on the Addons tab to scan"))
	} else {
		for i, inst := range m.reg.Installations {
			cursor := "  "
			if i == m.settingsCursor {
				cursor = "> "
			}
			if i == m.settingsCursor && m.renaming {
				lines = append(lines, cursor+labelStyle.Render("title: ")+m.renameInput.View())
				continue
			}
			label := m.installLabels[i]
			line := cursor + label + dimStyle.Render("   "+inst.Flavor+"   "+inst.Path)
			lines = append(lines, line)
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}
