package ui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#1d4ed8", Dark: "#93c5fd"})
	headerStyle = lipgloss.NewStyle().Bold(true)
	selColStyle = lipgloss.NewStyle().Bold(true).Reverse(true)
	selRowStyle = lipgloss.NewStyle().
			Background(lipgloss.AdaptiveColor{Light: "#dbeafe", Dark: "#1e3a5f"}).
			Foreground(lipgloss.AdaptiveColor{Light: "#0f172a", Dark: "#f8fafc"})
	dimStyle  = lipgloss.NewStyle().Faint(true)
	errStyle  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#b91c1c", Dark: "#f87171"})
	noteStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#92400e", Dark: "#fbbf24"})
)
