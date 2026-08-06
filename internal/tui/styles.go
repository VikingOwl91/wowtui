package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			Padding(0, 1)

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

	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	labelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
)
