package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *Model) handleDetailsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "backspace", "d", "enter":
		m.screen = screenAddons
		return m, nil
	}
	return m, nil
}

func (m *Model) detailsView() string {
	r := m.detailRow

	lines := []string{
		headerStyle.Render(r.Name),
		"",
		labelStyle.Render("Flavor:     ") + r.Flavor,
		labelStyle.Render("Status:     ") + r.Status,
		labelStyle.Render("Installed:  ") + r.Installed,
	}

	if r.Managed {
		lines = append(lines,
			labelStyle.Render("Provider:   ")+r.Provider,
			labelStyle.Render("Ref:        ")+r.Ref,
		)
		if r.Latest != "" {
			lines = append(lines, labelStyle.Render("Latest:     ")+r.Latest)
		}
		if m.reg != nil {
			if a := m.reg.FindAddon(r.Ref, r.InstallPath); a != nil {
				lines = append(lines,
					labelStyle.Render("Source:     ")+a.Source,
					labelStyle.Render("Asset:      ")+a.AssetName,
					labelStyle.Render("SHA256:     ")+a.SHA256,
					labelStyle.Render("Installed:  ")+a.InstalledAt.Format("2006-01-02 15:04"),
					labelStyle.Render("Folders:    ")+joinStrings(a.Folders),
				)
			}
		}
	} else {
		lines = append(lines, dimStyle.Render("Not managed by wowtui. Install it via 'a' to manage updates."))
	}

	lines = append(lines, "", labelStyle.Render("Install path:"))
	lines = append(lines, "  "+r.InstallPath)

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func joinStrings(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}
