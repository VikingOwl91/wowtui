package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			Padding(0, 1).
			MarginRight(2)

	tabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			Underline(true).
			Padding(0, 1)

	tabInactive = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Padding(0, 1)

	statusCurrent = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	statusUpdate  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	statusError   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	statusDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	statusBusy    = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	helpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	labelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	sectionTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	searchBox    = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(0, 1)
	emptyPanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238")).
			Foreground(lipgloss.Color("241")).
			Padding(1, 2)
	contentPanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("236")).
			Padding(0, 1)
	footerBar = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Padding(0, 1)
)
