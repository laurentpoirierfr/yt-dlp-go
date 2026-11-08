package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

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

func addJob(m *Model, id, title, format string) {
	info := &infodict.InfoDict{
		ID: id, Title: title, Format: format, Ext: "mp4",
		Width: 1920, Height: 1080, DurationString: "1:00",
	}
	m.Update(progressMsg(events.Progress{
		Info: info, Filename: id + ".mp4", Status: events.StatusRunning,
		Downloaded: 500, Total: 1000, Speed: 1024, ETA: 5 * time.Second,
	}))
}

func TestDetailPaneRendersSelectedJob(t *testing.T) {
	m := New()
	sized(m, 120, 40)
	addJob(m, "a", "Première vidéo", "137 - 1080p")
	addJob(m, "b", "Deuxième vidéo", "248 - 1080p")

	// By default the first job is selected.
	if m.selected != 0 {
		t.Fatalf("selected = %d, want 0", m.selected)
	}
	if !m.showDetail() {
		t.Fatal("detail pane should be shown on a wide terminal")
	}
	view := m.View()
	if !strings.Contains(view, "Détail") {
		t.Fatalf("detail label missing:\n%s", view)
	}
	if !strings.Contains(view, "Première vidéo") {
		t.Fatalf("selected title missing:\n%s", view)
	}

	// Move selection down: detail must follow.
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.selected != 1 {
		t.Fatalf("selected = %d, want 1", m.selected)
	}
	view = m.View()
	if !strings.Contains(view, "Format") || !strings.Contains(view, "248 - 1080p") {
		t.Fatalf("second job details missing:\n%s", view)
	}
}

func TestDetailPaneHiddenNarrow(t *testing.T) {
	m := New()
	sized(m, 60, 30)
	addJob(m, "a", "Vidéo", "137")
	if m.showDetail() {
		t.Fatal("detail pane should be hidden on a narrow terminal")
	}
	if strings.Contains(m.View(), "Détail") {
		t.Fatalf("detail pane should not be rendered:\n%s", m.View())
	}
}

func TestSelectionClamped(t *testing.T) {
	m := New()
	sized(m, 120, 40)
	addJob(m, "a", "A", "137")
	addJob(m, "b", "B", "248")
	addJob(m, "c", "C", "313")
	for i := 0; i < 10; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.selected != 2 {
		t.Fatalf("selected = %d, want 2", m.selected)
	}
	if got := m.selectedJob(); got == nil || got.key != "c" {
		t.Fatalf("selectedJob = %v, want job c", got)
	}
}

func TestFragmentProgressRendered(t *testing.T) {
	m := New()
	sized(m, 120, 40)
	addJob(m, "a", "Vidéo", "137")
	m.Update(progressMsg(events.Progress{
		Info: &infodict.InfoDict{ID: "a", Title: "Vidéo"}, Filename: "a.mp4",
		Status: events.StatusRunning, Downloaded: 750, Total: 1000,
		FragmentIndex: 3, FragmentCount: 10,
	}))
	view := m.View()
	if !strings.Contains(view, "frag 3/10") {
		t.Fatalf("fragment progress missing from job line:\n%s", view)
	}
	if !strings.Contains(view, "Fragments") || !strings.Contains(view, "3/10") {
		t.Fatalf("fragment progress missing from detail pane:\n%s", view)
	}
}

func TestPostProcessShownOnTask(t *testing.T) {
	m := New()
	sized(m, 120, 40)
	addJob(m, "a", "Vidéo", "137")
	m.Update(postProcessMsg(events.PostProcess{
		Info: &infodict.InfoDict{ID: "a"}, Name: "Merger", Status: events.StatusRunning,
	}))
	view := m.View()
	if !strings.Contains(view, "Fusion…") {
		t.Fatalf("running postprocessor missing from job line:\n%s", view)
	}
	if !strings.Contains(view, "Étape") || !strings.Contains(view, "Fusion · en cours") {
		t.Fatalf("running postprocessor missing from detail pane:\n%s", view)
	}

	m.Update(postProcessMsg(events.PostProcess{
		Info: &infodict.InfoDict{ID: "a"}, Name: "Merger", Status: events.StatusFinished,
	}))
	view = m.View()
	if strings.Contains(view, "Fusion…") {
		t.Fatalf("finished postprocessor should not look active:\n%s", view)
	}
	if !strings.Contains(view, "Fusion · terminé") {
		t.Fatalf("finished postprocessor state missing:\n%s", view)
	}
}

func TestFinalSummaryPersists(t *testing.T) {
	m := New()
	sized(m, 120, 40)
	addJob(m, "a", "Réussie", "137")
	m.Update(progressMsg(events.Progress{
		Info: &infodict.InfoDict{ID: "a", Title: "Réussie"}, Filename: "ok.mp4",
		Status: events.StatusFinished, Downloaded: 1234, Total: 1234,
	}))
	addJob(m, "b", "Ratée", "137")
	m.Update(progressMsg(events.Progress{
		Info: &infodict.InfoDict{ID: "b", Title: "Ratée"}, Filename: "ko.mp4",
		Status: events.StatusError, Err: errors.New("boom"),
	}))

	_, cmd := m.Update(workDoneMsg{})
	if cmd != nil {
		t.Fatal("the final summary should stay on screen instead of auto-quitting")
	}
	if !m.done {
		t.Fatal("model should be marked done")
	}

	view := m.View()
	for _, want := range []string{
		"Bilan", "terminé avec erreurs", "1/2 réussies", "boom", "ok.mp4",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("summary missing %q:\n%s", want, view)
		}
	}
	if got := strings.Count(view, "\n") + 1; got != 40 {
		t.Fatalf("view has %d lines, want 40", got)
	}
}

func TestFinalSummaryNarrow(t *testing.T) {
	m := New()
	sized(m, 60, 24)
	addJob(m, "a", "Vidéo", "137")
	m.Update(workDoneMsg{})
	view := m.View()
	if !strings.Contains(view, "Bilan") {
		t.Fatalf("summary missing on a narrow terminal:\n%s", view)
	}
	if strings.Contains(view, "Détail") {
		t.Fatalf("detail pane should be replaced by the summary:\n%s", view)
	}
	if got := strings.Count(view, "\n") + 1; got != 24 {
		t.Fatalf("view has %d lines, want 24", got)
	}
}

func runeKey(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestLogFilter(t *testing.T) {
	m := New()
	sized(m, 120, 40)
	m.Update(logMsg(events.Log{Level: events.LevelInfo, Text: "info alpha"}))
	m.Update(logMsg(events.Log{Level: events.LevelError, Text: "error beta"}))
	m.Update(logMsg(events.Log{Level: events.LevelWarning, Text: "warning gamma"}))

	// "/" enters filter mode; typed text filters live.
	m.Update(runeKey("/"))
	if !m.filtering {
		t.Fatal("expected filter input mode")
	}
	m.Update(runeKey("err"))
	if got := m.vp.View(); !strings.Contains(got, "error beta") ||
		strings.Contains(got, "info alpha") || strings.Contains(got, "warning gamma") {
		t.Fatalf("live filter mismatch:\n%s", got)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.filtering || m.filter != "err" {
		t.Fatalf("filter = %q filtering = %v, want %q", m.filter, m.filtering, "err")
	}
	if view := m.View(); !strings.Contains(view, "Logs · err") {
		t.Fatalf("filter indicator missing:\n%s", view)
	}

	// Esc clears an applied filter without quitting.
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.filter != "" {
		t.Fatalf("filter = %q, want cleared", m.filter)
	}
	if got := m.vp.View(); !strings.Contains(got, "info alpha") {
		t.Fatalf("logs not restored after clearing the filter:\n%s", got)
	}
}

func TestLogFilterIgnoresANSI(t *testing.T) {
	m := New()
	sized(m, 120, 40)
	m.logs = []string{"\x1b[31mhello\x1b[0m", "plain world"}
	m.filter = "31"
	m.refreshLogs()
	if strings.Contains(m.vp.View(), "hello") {
		t.Fatalf("escape sequence matched as visible text:\n%s", m.vp.View())
	}
	m.filter = "hello"
	m.refreshLogs()
	if !strings.Contains(m.vp.View(), "hello") {
		t.Fatalf("visible text should match:\n%s", m.vp.View())
	}
}

func TestHelpToggle(t *testing.T) {
	m := New()
	sized(m, 120, 40)
	m.Update(runeKey("?"))
	if !m.showHelp {
		t.Fatal("expected help to open")
	}
	view := m.View()
	if !strings.Contains(view, "Aide") || !strings.Contains(view, "filtrer") {
		t.Fatalf("help content missing:\n%s", view)
	}
	if got := strings.Count(view, "\n") + 1; got != 40 {
		t.Fatalf("help view has %d lines, want 40", got)
	}
	m.Update(runeKey("?"))
	if m.showHelp {
		t.Fatal("expected help to close")
	}
	if strings.Contains(m.View(), " Navigation") {
		t.Fatal("help still rendered")
	}
}

func TestSparkline(t *testing.T) {
	if got, want := sparkline([]float64{0, 1, 2, 3}, 4), "▁▃▅█"; got != want {
		t.Fatalf("sparkline = %q, want %q", got, want)
	}
	// Only the last `width` samples are shown.
	if got, want := sparkline([]float64{1, 2, 3}, 2), "▅█"; got != want {
		t.Fatalf("truncated sparkline = %q, want %q", got, want)
	}
	if got := sparkline(nil, 10); got != "" {
		t.Fatalf("empty sparkline = %q, want empty", got)
	}
	if got := sparkline([]float64{0, 0}, 10); got != "" {
		t.Fatalf("zero sparkline = %q, want empty", got)
	}
}

func TestSpeedHistoryRecorded(t *testing.T) {
	m := New()
	sized(m, 120, 40)
	addJob(m, "a", "Vidéo", "137")
	if len(m.speeds) == 0 {
		t.Fatal("speed history should be recorded on progress")
	}
	if got := m.speeds[len(m.speeds)-1]; got != 1024 {
		t.Fatalf("last speed = %v, want 1024", got)
	}
}

func TestSetTheme(t *testing.T) {
	defer SetTheme("default")
	SetTheme("dracula")
	if got, want := titleStyle.GetForeground(), lipgloss.Color("141"); got != want {
		t.Fatalf("dracula title = %v, want %v", got, want)
	}
	SetTheme("unknown")
	if got, want := titleStyle.GetForeground(), lipgloss.Color("205"); got != want {
		t.Fatalf("unknown theme should fall back to default, got %v", got)
	}
}

func TestNoColorDetection(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	if noColor() {
		t.Fatal("empty NO_COLOR should not disable color")
	}
	t.Setenv("NO_COLOR", "1")
	if !noColor() {
		t.Fatal("NO_COLOR=1 should disable color")
	}
}
