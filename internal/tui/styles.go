package tui

import "github.com/charmbracelet/lipgloss"

var (
	CheckedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))  // red
	UncheckedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // green
	CursorStyle    = lipgloss.NewStyle().Bold(true)
	PartialStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // yellow
	DimStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	HeaderStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	SupportStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
	SupportURLStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
)
