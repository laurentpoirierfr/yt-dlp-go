// Package tui provides a Bubble Tea terminal interface for yt-dlp-go. It
// consumes the events.Sink stream, so the extraction/download core remains
// unaware of the front-end.
package tui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Styles are package-level variables so the whole view shares one theme. They
// are (re)assigned by SetTheme.
var (
	titleStyle    lipgloss.Style
	logoStyle     lipgloss.Style
	dimStyle      lipgloss.Style
	okStyle       lipgloss.Style
	errStyle      lipgloss.Style
	warnStyle     lipgloss.Style
	focusedStyle  lipgloss.Style
	jobTitleStyle lipgloss.Style
	footerStyle   lipgloss.Style
)

type theme struct {
	title    lipgloss.Style
	dim      lipgloss.Style
	ok       lipgloss.Style
	err      lipgloss.Style
	warn     lipgloss.Style
	focused  lipgloss.Style
	jobTitle lipgloss.Style
	footer   lipgloss.Style
}

func init() {
	SetTheme("default")
}

// SetTheme applies a named theme. Unknown names fall back to "default", and
// the NO_COLOR environment variable forces the uncolored "mono" theme.
func SetTheme(name string) {
	if noColor() {
		applyTheme(themes["mono"])
		// Drop every escape sequence, including bold/underline, so redirected
		// or plain output stays readable.
		lipgloss.DefaultRenderer().SetColorProfile(termenv.Ascii)
		return
	}
	t, ok := themes[name]
	if !ok {
		t = themes["default"]
	}
	applyTheme(t)
}

// noColor implements the NO_COLOR convention: the variable disables color when
// set to a non-empty value.
func noColor() bool {
	v, ok := os.LookupEnv("NO_COLOR")
	return ok && v != ""
}

func applyTheme(t theme) {
	titleStyle = t.title
	logoStyle = lipgloss.NewStyle().Foreground(t.title.GetForeground())
	dimStyle = t.dim
	okStyle = t.ok
	errStyle = t.err
	warnStyle = t.warn
	focusedStyle = t.focused
	jobTitleStyle = t.jobTitle
	footerStyle = t.footer
}

// paletteTheme builds a theme from 256-color palette indexes.
func paletteTheme(title, dim, ok, err, warn, focused, jobTitle, footer string) theme {
	return theme{
		title:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(title)),
		dim:      lipgloss.NewStyle().Foreground(lipgloss.Color(dim)),
		ok:       lipgloss.NewStyle().Foreground(lipgloss.Color(ok)),
		err:      lipgloss.NewStyle().Foreground(lipgloss.Color(err)),
		warn:     lipgloss.NewStyle().Foreground(lipgloss.Color(warn)),
		focused:  lipgloss.NewStyle().Foreground(lipgloss.Color(focused)),
		jobTitle: lipgloss.NewStyle().Foreground(lipgloss.Color(jobTitle)),
		footer:   lipgloss.NewStyle().Foreground(lipgloss.Color(footer)),
	}
}

var themes = map[string]theme{
	"default": paletteTheme("205", "241", "42", "196", "214", "212", "252", "241"),
	"dracula": paletteTheme("141", "61", "84", "203", "215", "212", "255", "61"),
	"solarized": paletteTheme(
		"33", "240", "64", "160", "136", "37", "244", "240",
	),
	"mono": {
		title:    lipgloss.NewStyle().Bold(true),
		dim:      lipgloss.NewStyle(),
		ok:       lipgloss.NewStyle(),
		err:      lipgloss.NewStyle().Bold(true),
		warn:     lipgloss.NewStyle().Bold(true),
		focused:  lipgloss.NewStyle().Bold(true).Underline(true),
		jobTitle: lipgloss.NewStyle(),
		footer:   lipgloss.NewStyle(),
	},
}

// ThemeNames lists the available theme names.
func ThemeNames() []string {
	return []string{"default", "dracula", "solarized", "mono"}
}

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
