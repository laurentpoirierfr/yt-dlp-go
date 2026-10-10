// Package tui provides a Bubble Tea terminal interface for yt-dlp-go. It
// consumes the events.Sink stream, so the extraction/download core remains
// unaware of the front-end.
package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	okStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	warnStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	focusedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	jobTitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	footerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	boxStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
)

func levelStyle(level string) lipgloss.Style {
	switch level {
	case "warning":
		return warnStyle
	case "error":
		return errStyle
	case "debug":
		return dimStyle
	default:
		return lipgloss.NewStyle()
	}
}
