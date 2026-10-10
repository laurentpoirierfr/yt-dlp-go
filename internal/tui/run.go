package tui

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/events"
)

// Supported reports whether stdout is an interactive terminal.
func Supported() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// Run starts the TUI and executes work in a background goroutine. work
// receives the model as its events.Sink; the TUI exits when work returns (or
// when the user quits). It returns work's error, if any.
func Run(work func(events.Sink) error) error {
	m := New()
	p := tea.NewProgram(m, tea.WithAltScreen())
	m.Attach(p)

	go func() {
		err := work(m)
		p.Send(workDoneMsg{err: err})
	}()

	if _, err := p.Run(); err != nil {
		return err
	}
	return m.err
}
