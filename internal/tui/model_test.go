package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/events"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
)

func sized(m *Model, w, h int) {
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
}

func TestViewFillsTerminal(t *testing.T) {
	m := New()
	sized(m, 120, 40)
	m.Update(progressMsg(events.Progress{
		Info:     &infodict.InfoDict{ID: "abc", Title: "Ma vidéo"},
		Filename: "out.mp4", Status: events.StatusStarted, Total: 1000,
	}))
	m.Update(progressMsg(events.Progress{
		Info:     &infodict.InfoDict{ID: "abc", Title: "Ma vidéo"},
		Filename: "out.mp4", Status: events.StatusRunning,
		Downloaded: 500, Total: 1000, Speed: 1024,
	}))

	view := m.View()
	if got := strings.Count(view, "\n") + 1; got != 40 {
		t.Fatalf("view has %d lines, want 40", got)
	}
	if !strings.Contains(view, "Ma vidéo") {
		t.Fatalf("title not rendered:\n%s", view)
	}
}

func TestLogoShownAndHidden(t *testing.T) {
	wide := New()
	sized(wide, 120, 40)
	if !wide.showLogo() {
		t.Fatal("logo should show on a wide terminal")
	}
	if !strings.Contains(wide.View(), logoLines()[0]) {
		t.Fatalf("logo line missing:\n%s", wide.View())
	}

	narrow := New()
	sized(narrow, 60, 40)
	if narrow.showLogo() {
		t.Fatal("logo should be hidden on a narrow terminal")
	}
	if strings.Contains(narrow.View(), logoLines()[0]) {
		t.Fatal("logo should not appear on a narrow terminal")
	}
}

func TestJobLifecycleCounts(t *testing.T) {
	m := New()
	sized(m, 120, 40)
	info := &infodict.InfoDict{ID: "a", Title: "A"}
	m.Update(progressMsg(events.Progress{Info: info, Filename: "a", Status: events.StatusStarted}))
	m.Update(progressMsg(events.Progress{Info: info, Filename: "a", Status: events.StatusFinished, Downloaded: 10, Total: 10}))
	if m.finished != 1 || m.failed != 0 {
		t.Fatalf("finished=%d failed=%d", m.finished, m.failed)
	}

	info2 := &infodict.InfoDict{ID: "b", Title: "B"}
	m.Update(progressMsg(events.Progress{Info: info2, Filename: "b", Status: events.StatusError, Err: errString("boom")}))
	if m.failed != 1 {
		t.Fatalf("failed=%d want 1", m.failed)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestLogLevelsAreStyled(t *testing.T) {
	cases := map[string]lipgloss.TerminalColor{
		"warning": lipgloss.Color("214"),
		"error":   lipgloss.Color("196"),
		"debug":   lipgloss.Color("241"),
	}
	for level, want := range cases {
		if got := levelStyle(level).GetForeground(); got != want {
			t.Fatalf("levelStyle(%q) foreground = %v, want %v", level, got, want)
		}
	}
	if got := levelStyle("info").GetForeground(); got != (lipgloss.NoColor{}) {
		t.Fatalf("info should be unstyled, got foreground %v", got)
	}
}

func TestLogMessagesRendered(t *testing.T) {
	m := New()
	sized(m, 100, 30)
	m.Update(logMsg(events.Log{Level: events.LevelWarning, Text: "attention"}))
	m.Update(logMsg(events.Log{Level: events.LevelError, Text: "ligne1\nligne2"}))
	view := m.View()
	for _, want := range []string{"attention", "ligne1", "ligne2"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}
