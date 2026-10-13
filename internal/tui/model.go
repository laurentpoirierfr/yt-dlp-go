package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/laurentpoirierfr/yt-dlp-go/internal/events"
	"github.com/laurentpoirierfr/yt-dlp-go/internal/utils"
)

// Messages fed from the events.Sink into the Bubble Tea update loop.
type (
	progressMsg    events.Progress
	postProcessMsg events.PostProcess
	logMsg         events.Log
	workDoneMsg    struct{ err error }
)

// job tracks a single download.
type job struct {
	key        string
	title      string
	bar        progress.Model
	percent    float64
	downloaded int64
	total      int64
	speed      float64
	eta        time.Duration
	status     events.Status
	err        error
}

// Model is the Bubble Tea model.
type Model struct {
	program *tea.Program

	width, height int
	ready         bool

	jobs     []*job
	jobIndex map[string]int

	logs []string
	vp   viewport.Model

	finished int
	failed   int
	done     bool
	err      error
	focusLog   bool
	selected   int // index in jobs (most recent shown order? or absolute)

	started time.Time
}

// New builds an empty TUI model.
func New() *Model {
	return &Model{
		jobIndex: map[string]int{},
		vp:       viewport.New(0, 0),
		started:  time.Now(),
	}
}

// Attach binds the model to its running program (for Send).
func (m *Model) Attach(p *tea.Program) { m.program = p }

// ---- events.Sink implementation (called from worker goroutines) ----

// Progress implements events.Sink.
func (m *Model) Progress(p events.Progress) {
	if m.program != nil {
		m.program.Send(progressMsg(p))
	}
}

// PostProcess implements events.Sink.
func (m *Model) PostProcess(p events.PostProcess) {
	if m.program != nil {
		m.program.Send(postProcessMsg(p))
	}
}

// Log implements events.Sink.
func (m *Model) Log(l events.Log) {
	if m.program != nil {
		m.program.Send(logMsg(l))
	}
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		m.vp.Width = m.width - 2
		if h := m.height - m.headerHeight() - m.jobsHeight() - 2; h > 2 {
			m.vp.Height = h - 1
		} else {
			m.vp.Height = h
		}
		m.resizeBars()
		return m, nil

	case progressMsg:
		m.applyProgress(events.Progress(msg))
		return m, nil

	case postProcessMsg:
		p := events.PostProcess(msg)
		switch p.Status {
		case events.StatusStarted:
			m.appendLogStyle(dimStyle, fmt.Sprintf("[%s] processing…", p.Name))
		case events.StatusFinished:
			m.appendLogStyle(okStyle, fmt.Sprintf("[%s] done", p.Name))
		case events.StatusError:
			m.appendLogStyle(errStyle, fmt.Sprintf("[%s] %v", p.Name, p.Err))
		}
		return m, nil

	case logMsg:
		l := events.Log(msg)
		m.appendLogStyle(levelStyle(l.Level.String()), l.Text)
		return m, nil

	case workDoneMsg:
		m.done = true
		m.err = msg.err
		if msg.err != nil {
			m.appendLogStyle(errStyle, "ERROR: "+msg.err.Error())
		}
		return m, tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
			return tea.Quit()
		})

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	case "tab":
		m.focusLog = !m.focusLog
		return m, nil
	case "up", "k":
		if m.focusLog {
			m.vp.LineUp(1)
		} else {
			if m.selected > 0 {
				m.selected--
			}
		}
		return m, nil
	case "down", "j":
		if m.focusLog {
			m.vp.LineDown(1)
		} else {
			if m.selected < len(m.jobs)-1 {
				m.selected++
			}
		}
		return m, nil
	case "pgup":
		m.vp.HalfViewUp()
		return m, nil
	case "pgdown":
		m.vp.HalfViewDown()
		return m, nil
	case "g":
		if m.focusLog {
			m.vp.GotoTop()
		} else if len(m.jobs) > 0 {
			m.selected = 0
		}
		return m, nil
	case "G":
		if m.focusLog {
			m.vp.GotoBottom()
		} else if len(m.jobs) > 0 {
			m.selected = len(m.jobs) - 1
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) applyProgress(p events.Progress) {
	key := p.Filename
	if p.Info != nil && p.Info.ID != "" {
		key = p.Info.ID
	}
	if key == "" {
		key = "download"
	}
	j, ok := m.jobByKey(key)
	if !ok {
		j = &job{
			key:   key,
			title: m.jobTitle(p),
			bar:   progress.New(progress.WithDefaultGradient(), progress.WithoutPercentage()),
		}
		if m.width > 0 {
			j.bar.Width = m.barWidth()
		}
		m.jobIndex[key] = len(m.jobs)
		m.jobs = append(m.jobs, j)
	}
	j.downloaded = p.Downloaded
	j.total = p.Total
	j.speed = p.Speed
	j.eta = p.ETA
	if p.Total > 0 {
		j.percent = float64(p.Downloaded) / float64(p.Total)
	} else if p.Status == events.StatusFinished {
		j.percent = 1
	}
	if j.percent > 1 {
		j.percent = 1
	}
	j.bar.SetPercent(j.percent)

	prev := j.status
	j.status = p.Status
	if p.Err != nil {
		j.err = p.Err
	}
	switch p.Status {
	case events.StatusFinished:
		if prev != events.StatusFinished {
			m.finished++
			m.appendLogStyle(okStyle, "Completed: "+j.title)
		}
	case events.StatusError:
		if prev != events.StatusError {
			m.failed++
			m.appendLogStyle(errStyle, fmt.Sprintf("Failed: %s: %v", j.title, p.Err))
		}
	}
}

func (m *Model) jobTitle(p events.Progress) string {
	if p.Info != nil && p.Info.Title != "" {
		return p.Info.Title
	}
	if p.Filename != "" {
		return p.Filename
	}
	return "download"
}

func (m *Model) jobByKey(key string) (*job, bool) {
	i, ok := m.jobIndex[key]
	if !ok {
		return nil, false
	}
	return m.jobs[i], true
}

func (m *Model) appendLogStyle(style lipgloss.Style, text string) {
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		m.logs = append(m.logs, style.Render(line))
	}
	const maxLogs = 2000
	if len(m.logs) > maxLogs {
		m.logs = m.logs[len(m.logs)-maxLogs:]
	}
	m.vp.SetContent(strings.Join(m.logs, "\n"))
	m.vp.GotoBottom()
}

func (m *Model) resizeBars() {
	bw := m.barWidth()
	for _, j := range m.jobs {
		j.bar.Width = bw
	}
}

// ---- Layout metrics ----

func (m *Model) showLogo() bool {
	return m.width >= logoWidth()+24 && m.height >= 16
}

func (m *Model) headerHeight() int {
	if m.showLogo() {
		return len(logoLines())
	}
	return 3
}

// jobsHeight returns the height of the jobs area. Jobs take exactly what they
// need (two lines each); the logs pane keeps a minimum and absorbs the rest.
func (m *Model) jobsHeight() int {
	needed := len(m.jobs) * 2
	if needed < 2 {
		needed = 2
	}
	available := m.height - m.headerHeight() - 2 // footer
	const minLogs = 6
	maxJobs := available - minLogs
	if maxJobs < 2 {
		maxJobs = 2
	}
	if needed > maxJobs {
		needed = maxJobs
	}
	return needed
}

// logsHeightActual is the height reserved for the logs pane.
func (m *Model) logsHeightActual() int {
	h := m.height - m.headerHeight() - m.jobsHeight() - 2
	if h < 2 {
		h = 2
	}
	return h
}

func (m *Model) barWidth() int {
	w := m.width/2 - 8
	if w < 16 {
		w = 16
	}
	if w > 60 {
		w = 60
	}
	return w
}

// ---- Rendering ----

// View implements tea.Model, filling the whole terminal.
func (m *Model) View() string {
	if !m.ready || m.width <= 0 || m.height <= 0 {
		return "Starting yt-dlp-go…"
	}
	var lines []string
	lines = append(lines, m.viewHeader()...)
	lines = append(lines, m.viewJobs()...)
	lines = append(lines, m.viewLogs()...)
	lines = append(lines, m.viewFooter()...)
	// Guarantee the view fills the whole terminal exactly.
	for len(lines) < m.height {
		lines = append(lines, "")
	}
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	return strings.Join(lines, "\n")
}

func (m *Model) viewHeader() []string {
	total := len(m.jobs)
	status := fmt.Sprintf("%d/%d completed", m.finished, total)
	if m.failed > 0 {
		status += fmt.Sprintf(" · %d failed", m.failed)
	}
	if m.done && m.err == nil {
		status += " · done"
	}

	left := []string{
		titleStyle.Render(" yt-dlp-go "),
		dimStyle.Render(" " + status),
		dimStyle.Render(" " + m.aggregate()),
	}
	for len(left) < m.headerHeight() {
		left = append(left, "")
	}

	if !m.showLogo() {
		return left[:m.headerHeight()]
	}

	leftW := m.width - logoWidth() - 2
	if leftW < 10 {
		leftW = 10
	}
	leftBlock := lipgloss.NewStyle().Width(leftW).Render(strings.Join(left, "\n"))
	logoBlock := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Render(strings.Join(logoLines(), "\n"))
	header := lipgloss.JoinHorizontal(lipgloss.Top, leftBlock, "  ", logoBlock)
	return strings.Split(header, "\n")
}

func (m *Model) aggregate() string {
	var speed float64
	var downloaded int64
	for _, j := range m.jobs {
		if j.status == events.StatusRunning {
			speed += j.speed
		}
		downloaded += j.downloaded
	}
	parts := []string{utils.FormatBytes(downloaded) + " total"}
	if speed > 0 {
		parts = append(parts, utils.FormatBytes(int64(speed))+"/s")
	}
	if !m.started.IsZero() {
		parts = append(parts, "elapsed "+utils.FormatClock(time.Since(m.started)))
	}
	return strings.Join(parts, " · ")
}

func (m *Model) viewJobs() []string {
	h := m.jobsHeight()
	lines := make([]string, 0, h)

	if len(m.jobs) == 0 {
		lines = append(lines, dimStyle.Render("  Preparing…"))
	}

	// Each job takes 2 lines; show the most recent ones that fit.
	capacity := h / 2
	if capacity < 1 {
		capacity = 1
	}
	shown := m.jobs
	if len(shown) > capacity {
		shown = shown[len(shown)-capacity:]
	}
	for _, j := range shown {
		lines = append(lines, m.viewJob(j)...)
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	return lines
}

func (m *Model) viewJob(j *job) []string {
	marker := " "
	switch j.status {
	case events.StatusFinished:
		marker = okStyle.Render("✔")
	case events.StatusError:
		marker = errStyle.Render("✘")
	}
	title := "  " + j.title
	maxW := m.width - 4
	if maxW < 4 {
		maxW = 4
	}
	if len([]rune(title)) > maxW {
		title = string([]rune(title)[:maxW])
	}
	line1 := fmt.Sprintf("%s %s", marker, jobTitleStyle.Render(title))
	line2 := "  " + j.bar.ViewAs(j.percent) + "  " + m.jobStats(j)
	return []string{line1, line2}
}

func (m *Model) jobStats(j *job) string {
	parts := []string{}
	if j.total > 0 {
		parts = append(parts, fmt.Sprintf("%s/%s", utils.FormatBytes(j.downloaded), utils.FormatBytes(j.total)))
	} else {
		parts = append(parts, utils.FormatBytes(j.downloaded))
	}
	if j.speed > 0 && j.status == events.StatusRunning {
		parts = append(parts, utils.FormatBytes(int64(j.speed))+"/s")
	}
	if j.eta >= 0 && j.status == events.StatusRunning {
		parts = append(parts, "ETA "+utils.FormatClock(j.eta))
	}
	if j.status == events.StatusError {
		parts = append(parts, errStyle.Render("error"))
	}
	return dimStyle.Render(strings.Join(parts, "  "))
}

func (m *Model) viewLogs() []string {
	h := m.logsHeightActual()
	m.vp.Width = m.width - 2
	m.vp.Height = h - 1
	label := dimStyle.Render(" Logs ")
	if m.focusLog {
		label = focusedStyle.Render(" Logs ")
	}
	lines := []string{label}
	lines = append(lines, strings.Split(m.vp.View(), "\n")...)
	for len(lines) < h {
		lines = append(lines, "")
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	return lines
}

func (m *Model) viewFooter() []string {
	help := " q quit · tab focus logs · j/k scroll · g/G top/bottom"
	return []string{
		footerStyle.Render(help),
		footerStyle.Render(fmt.Sprintf(" %d finished · %d failed", m.finished, m.failed)),
	}
}
