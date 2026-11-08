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
	"github.com/laurentpoirierfr/yt-dlp-go/internal/infodict"
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
	info       *infodict.InfoDict
	filename   string

	fragmentIndex int
	fragmentCount int

	ppName   string
	ppStatus events.Status
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

	finished  int
	failed    int
	done      bool
	err       error
	focusLog  bool
	selected  int // index in jobs (most recent shown order? or absolute)
	showHelp  bool
	filtering bool
	filterBuf string
	filter    string

	speeds []float64

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
		if j := m.jobForInfo(p.Info); j != nil {
			j.ppName = p.Name
			j.ppStatus = p.Status
		}
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
		// Keep the final summary on screen until the user quits.
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.filtering {
		return m.handleFilterKey(msg)
	}
	if m.showHelp {
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?", "esc":
			m.showHelp = false
		}
		return m, nil
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.filterBuf = ""
			m.refreshLogs()
			return m, nil
		}
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "/":
		m.filtering = true
		m.filterBuf = m.filter
		return m, nil
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

// handleFilterKey edits the log filter while "/" input mode is active.
func (m *Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.filtering = false
		m.filterBuf = ""
		m.filter = ""
		m.refreshLogs()
	case tea.KeyEnter:
		m.filtering = false
		m.filter = m.filterBuf
		m.refreshLogs()
	case tea.KeyBackspace:
		if r := []rune(m.filterBuf); len(r) > 0 {
			m.filterBuf = string(r[:len(r)-1])
		}
		m.filter = m.filterBuf
		m.refreshLogs()
	case tea.KeyCtrlU:
		m.filterBuf = ""
		m.filter = ""
		m.refreshLogs()
	case tea.KeyRunes:
		m.filterBuf += string(msg.Runes)
		m.filter = m.filterBuf
		m.refreshLogs()
	case tea.KeySpace:
		m.filterBuf += " "
		m.filter = m.filterBuf
		m.refreshLogs()
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
	j.fragmentIndex = p.FragmentIndex
	j.fragmentCount = p.FragmentCount
	if p.Info != nil {
		j.info = p.Info
	}
	if p.Filename != "" {
		j.filename = p.Filename
	}
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
	m.recordSpeed()
}

// recordSpeed appends the current aggregate speed to the sparkline history.
func (m *Model) recordSpeed() {
	var speed float64
	for _, j := range m.jobs {
		if j.status == events.StatusRunning {
			speed += j.speed
		}
	}
	m.speeds = append(m.speeds, speed)
	const maxSamples = 60
	if len(m.speeds) > maxSamples {
		m.speeds = m.speeds[len(m.speeds)-maxSamples:]
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

// jobForInfo looks up the job a post-processing event belongs to.
func (m *Model) jobForInfo(info *infodict.InfoDict) *job {
	if info == nil || info.ID == "" {
		return nil
	}
	j, ok := m.jobByKey(info.ID)
	if !ok {
		return nil
	}
	return j
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
	m.refreshLogs()
}

// refreshLogs rebuilds the viewport content, applying the active filter.
func (m *Model) refreshLogs() {
	lines := m.logs
	if m.filter != "" {
		q := strings.ToLower(m.filter)
		lines = make([]string, 0, len(m.logs))
		for _, line := range m.logs {
			if strings.Contains(strings.ToLower(line), q) {
				lines = append(lines, line)
			}
		}
	}
	m.vp.SetContent(strings.Join(lines, "\n"))
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
	lines = append(lines, m.viewBody()...)
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

	leftW := m.width - logoWidth() - 2
	if leftW < 10 {
		leftW = 10
	}
	left := []string{
		titleStyle.Render(truncateRunes(" yt-dlp-go ", leftW)),
		dimStyle.Render(truncateRunes(" "+status, leftW)),
		dimStyle.Render(truncateRunes(" "+m.aggregate(), leftW)),
	}
	for len(left) < m.headerHeight() {
		left = append(left, "")
	}

	if !m.showLogo() {
		return left[:m.headerHeight()]
	}

	leftBlock := lipgloss.NewStyle().Width(leftW).Render(strings.Join(left, "\n"))
	logoBlock := logoStyle.Render(strings.Join(logoLines(), "\n"))
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
		if spark := sparkline(m.speeds, 16); spark != "" {
			parts = append(parts, spark)
		}
	}
	if !m.started.IsZero() {
		parts = append(parts, "elapsed "+utils.FormatClock(time.Since(m.started)))
	}
	return strings.Join(parts, " · ")
}

// sparkline renders the last width speed samples as Unicode blocks.
func sparkline(values []float64, width int) string {
	if len(values) == 0 || width <= 0 {
		return ""
	}
	if len(values) > width {
		values = values[len(values)-width:]
	}
	maxV := 0.0
	for _, v := range values {
		if v > maxV {
			maxV = v
		}
	}
	if maxV <= 0 {
		return ""
	}
	blocks := []rune("▁▂▃▄▅▆▇█")
	var b strings.Builder
	for _, v := range values {
		if v < 0 {
			v = 0
		}
		idx := int(v / maxV * float64(len(blocks)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(blocks) {
			idx = len(blocks) - 1
		}
		b.WriteRune(blocks[idx])
	}
	return b.String()
}

func (m *Model) viewJobs() []string {
	h := m.jobsHeight()
	lines := make([]string, 0, h)

	if len(m.jobs) == 0 {
		lines = append(lines, dimStyle.Render("  Preparing…"))
	}

	// Each job takes 2 lines. Show the most recent ones that fit, scrolling so
	// the selected job stays visible.
	capacity := h / 2
	if capacity < 1 {
		capacity = 1
	}
	start := 0
	if len(m.jobs) > capacity {
		start = len(m.jobs) - capacity
	}
	if m.selected < start {
		start = m.selected
	}
	if m.selected >= start+capacity {
		start = m.selected - capacity + 1
	}
	if start < 0 {
		start = 0
	}
	end := start + capacity
	if end > len(m.jobs) {
		end = len(m.jobs)
	}
	for i := start; i < end; i++ {
		lines = append(lines, m.viewJob(m.jobs[i], i == m.selected)...)
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	return lines
}

func (m *Model) viewJob(j *job, selected bool) []string {
	cursor := " "
	if selected {
		cursor = focusedStyle.Render("▸")
	}
	marker := " "
	switch j.status {
	case events.StatusRunning:
		marker = dimStyle.Render("•")
	case events.StatusFinished:
		marker = okStyle.Render("✔")
	case events.StatusError:
		marker = errStyle.Render("✘")
	}
	title := j.title
	maxW := m.width - 5
	if maxW < 4 {
		maxW = 4
	}
	title = truncateRunes(title, maxW)
	titleStyle := jobTitleStyle
	if selected {
		titleStyle = focusedStyle.Bold(true)
	}
	line1 := fmt.Sprintf("%s%s %s", cursor, marker, titleStyle.Render(title))
	line2 := "   " + j.bar.ViewAs(j.percent) + "  " + m.jobStats(j)
	return []string{line1, line2}
}

func (m *Model) jobStats(j *job) string {
	parts := []string{}
	if j.total > 0 {
		parts = append(parts, fmt.Sprintf("%s/%s", utils.FormatBytes(j.downloaded), utils.FormatBytes(j.total)))
	} else {
		parts = append(parts, utils.FormatBytes(j.downloaded))
	}
	if j.fragmentCount > 0 && j.fragmentIndex > 0 {
		parts = append(parts, fmt.Sprintf("frag %d/%d", j.fragmentIndex, j.fragmentCount))
	}
	if j.speed > 0 && j.status == events.StatusRunning {
		parts = append(parts, utils.FormatBytes(int64(j.speed))+"/s")
	}
	if j.eta >= 0 && j.status == events.StatusRunning {
		parts = append(parts, "ETA "+utils.FormatClock(j.eta))
	}
	if j.ppName != "" && (j.ppStatus == events.StatusStarted || j.ppStatus == events.StatusRunning) {
		parts = append(parts, postProcessLabel(j.ppName)+"…")
	}
	if j.status == events.StatusError {
		parts = append(parts, errStyle.Render("error"))
	}
	return dimStyle.Render(strings.Join(parts, "  "))
}

// showDetail reports whether there is room for the side-by-side detail pane.
func (m *Model) showDetail() bool {
	return m.width >= 88 && m.height >= 18
}

// viewBody renders the lower region: full-width logs by default, or a detail
// pane next to the logs when the terminal is wide enough.
func (m *Model) viewBody() []string {
	h := m.logsHeightActual()
	if m.showHelp {
		return m.viewHelp(m.width-2, h)
	}
	if m.done {
		return m.viewDone(h)
	}
	if !m.showDetail() {
		return m.viewLogs(m.width-2, h)
	}
	detailW := m.detailWidth()
	sep := " │ "
	logW := m.width - detailW - len([]rune(sep))
	if logW < 20 {
		return m.viewLogs(m.width-2, h)
	}
	details := m.viewDetails(detailW, h)
	logs := m.viewLogs(logW, h)
	return joinPanes(details, logs, detailW, sep, h)
}

// detailWidth is the width reserved for the side pane.
func (m *Model) detailWidth() int {
	w := m.width * 2 / 5
	if w < 28 {
		w = 28
	}
	if w > 48 {
		w = 48
	}
	return w
}

// viewDone renders the final summary once work has finished. On wide
// terminals it sits next to the logs; otherwise it uses the full width.
func (m *Model) viewDone(h int) []string {
	if !m.showDetail() {
		return m.viewSummary(m.width-2, h)
	}
	summaryW := m.detailWidth()
	sep := " │ "
	logW := m.width - summaryW - len([]rune(sep))
	if logW < 20 {
		return m.viewSummary(m.width-2, h)
	}
	return joinPanes(m.viewSummary(summaryW, h), m.viewLogs(logW, h), summaryW, sep, h)
}

func joinPanes(left, right []string, leftW int, sep string, h int) []string {
	sepStr := dimStyle.Render(sep)
	lines := make([]string, 0, h)
	for i := 0; i < h; i++ {
		var l, r string
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		lines = append(lines, lipgloss.NewStyle().Width(leftW).Render(l)+sepStr+r)
	}
	return lines
}

func (m *Model) viewSummary(w, h int) []string {
	lines := []string{dimStyle.Render(" Bilan ")}
	add := func(key, val string) {
		if len(lines) < h {
			lines = append(lines, m.detailLine(w, key, val))
		}
	}

	var downloaded int64
	for _, j := range m.jobs {
		downloaded += j.downloaded
	}
	result := "terminé"
	if m.err != nil {
		result = "arrêté sur erreur"
	} else if m.failed > 0 {
		result = "terminé avec erreurs"
	}
	add("Résultat", result)
	add("Tâches", fmt.Sprintf("%d/%d réussies", m.finished, len(m.jobs)))
	if m.failed > 0 {
		add("Échecs", fmt.Sprintf("%d", m.failed))
	}
	add("Téléchargé", utils.FormatBytes(downloaded))
	add("Durée", utils.FormatClock(time.Since(m.started)))

	if m.failed > 0 && len(lines) < h {
		lines = append(lines, "")
		if len(lines) < h {
			lines = append(lines, dimStyle.Render(" Échecs :"))
		}
		for _, j := range m.jobs {
			if len(lines) >= h {
				break
			}
			if j.status != events.StatusError {
				continue
			}
			msg := j.title
			if j.err != nil {
				msg += " — " + j.err.Error()
			}
			lines = append(lines, errStyle.Render("  "+truncateRunes(msg, w-2)))
		}
	}

	var files []string
	for _, j := range m.jobs {
		if j.status == events.StatusFinished && j.filename != "" {
			files = append(files, j.filename)
		}
	}
	if len(files) > 0 && len(lines)+1 < h {
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render(" Fichiers :"))
		for _, f := range files {
			if len(lines) >= h {
				break
			}
			lines = append(lines, "  "+truncateRunes(f, w-2))
		}
	}

	for len(lines) < h {
		lines = append(lines, "")
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	return lines
}

func (m *Model) viewLogs(w, h int) []string {
	m.vp.Width = w
	m.vp.Height = h - 1
	labelText := " Logs "
	if m.filter != "" {
		labelText = " Logs · " + truncateRunes(m.filter, 24) + " "
	}
	label := dimStyle.Render(labelText)
	if m.focusLog {
		label = focusedStyle.Render(labelText)
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

func (m *Model) viewDetails(w, h int) []string {
	lines := []string{dimStyle.Render(" Détail ")}
	j := m.selectedJob()
	if j != nil {
		for _, f := range m.detailFields(j) {
			if len(lines) >= h {
				break
			}
			lines = append(lines, m.detailLine(w, f[0], f[1]))
		}
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	return lines
}

func (m *Model) detailFields(j *job) [][2]string {
	fields := [][2]string{{"Titre", j.title}}
	if j.info != nil {
		if f := formatDescription(j.info); f != "" {
			fields = append(fields, [2]string{"Format", f})
		}
		if res := resolutionOf(j.info); res != "" {
			fields = append(fields, [2]string{"Résolution", res})
		}
		if j.info.DurationString != "" {
			fields = append(fields, [2]string{"Durée", j.info.DurationString})
		}
	}
	size := utils.FormatBytes(j.downloaded)
	if j.total > 0 {
		size = fmt.Sprintf("%s / %s", utils.FormatBytes(j.downloaded), utils.FormatBytes(j.total))
	}
	fields = append(fields, [2]string{"Taille", size})
	fields = append(fields, [2]string{"Progrès", fmt.Sprintf("%.0f%%", j.percent*100)})
	if j.fragmentCount > 0 {
		fields = append(fields, [2]string{"Fragments", fmt.Sprintf("%d/%d", j.fragmentIndex, j.fragmentCount)})
	}
	if j.speed > 0 && j.status == events.StatusRunning {
		fields = append(fields, [2]string{"Débit", utils.FormatBytes(int64(j.speed)) + "/s"})
	}
	if j.status == events.StatusRunning && j.eta >= 0 {
		fields = append(fields, [2]string{"ETA", utils.FormatClock(j.eta)})
	}
	if j.ppName != "" {
		fields = append(fields, [2]string{"Étape", postProcessLabel(j.ppName) + " · " + statusLabel(j.ppStatus)})
	}
	if j.filename != "" {
		fields = append(fields, [2]string{"Fichier", j.filename})
	}
	fields = append(fields, [2]string{"Statut", statusLabel(j.status)})
	if j.err != nil {
		fields = append(fields, [2]string{"Erreur", j.err.Error()})
	}
	return fields
}

func (m *Model) detailLine(w int, key, val string) string {
	const keyW = 10
	// One column of padding after the label, plus a margin so values never
	// touch the separator between the two panes.
	avail := w - keyW - 2
	if avail < 4 {
		avail = 4
	}
	style := lipgloss.NewStyle()
	if key == "Erreur" {
		style = errStyle
	}
	return dimStyle.Render(padRight(key, keyW)) + " " + style.Render(truncateRunes(val, avail))
}

func (m *Model) selectedJob() *job {
	if len(m.jobs) == 0 {
		return nil
	}
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(m.jobs) {
		m.selected = len(m.jobs) - 1
	}
	return m.jobs[m.selected]
}

func statusLabel(s events.Status) string {
	switch s {
	case events.StatusStarted:
		return "démarrage"
	case events.StatusRunning:
		return "en cours"
	case events.StatusFinished:
		return "terminé"
	case events.StatusError:
		return "échec"
	}
	return "?"
}

// postProcessLabel translates the postprocessor names shown by the pipeline.
func postProcessLabel(name string) string {
	switch name {
	case "Merger":
		return "Fusion"
	case "AudioConvert":
		return "Conversion audio"
	case "SubtitlesConvertor":
		return "Sous-titres"
	case "MoveFiles":
		return "Déplacement"
	}
	return name
}

func formatDescription(info *infodict.InfoDict) string {
	f := info.Format
	if f == "" {
		f = info.FormatID
	}
	if info.Ext != "" && !strings.Contains(f, info.Ext) {
		if f != "" {
			f += " "
		}
		f += "(" + info.Ext + ")"
	}
	return f
}

func resolutionOf(info *infodict.InfoDict) string {
	if info.Resolution != "" {
		return info.Resolution
	}
	if info.Width > 0 && info.Height > 0 {
		return fmt.Sprintf("%dx%d", info.Width, info.Height)
	}
	if info.Height > 0 {
		return fmt.Sprintf("%dp", info.Height)
	}
	return ""
}

func padRight(s string, n int) string {
	r := []rune(s)
	if len(r) >= n {
		return string(r[:n])
	}
	return s + strings.Repeat(" ", n-len(r))
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

func (m *Model) viewFooter() []string {
	if m.filtering {
		return []string{
			footerStyle.Render(" / " + m.filterBuf + "▏"),
			footerStyle.Render(" entrée appliquer · esc effacer · ctrl+u vider"),
		}
	}
	help := " q quit · ↑↓ select · tab logs · / filter · ? help"
	if m.showHelp {
		help = " ? close help · q quit"
	} else if m.done {
		help = " q quit · final summary · ↑↓ select · tab logs · / filter"
	}
	return []string{
		footerStyle.Render(help),
		footerStyle.Render(fmt.Sprintf(" %d finished · %d failed", m.finished, m.failed)),
	}
}

// viewHelp renders the key binding reference.
func (m *Model) viewHelp(w, h int) []string {
	lines := []string{dimStyle.Render(" Aide ")}
	add := func(text string) {
		if len(lines) < h {
			lines = append(lines, truncateRunes(text, w))
		}
	}
	add(titleStyle.Render(" Navigation"))
	add("  ↑ ↓ / k j   sélectionner une tâche")
	add("  tab         journaux / tâches")
	add("  g / G       début / fin")
	add("")
	add(titleStyle.Render(" Journaux"))
	add("  j / k       faire défiler")
	add("  /           filtrer")
	add("  esc         effacer le filtre")
	add("")
	add(titleStyle.Render(" Affichage"))
	add("  ?           fermer l'aide")
	add("  q           quitter")
	add("")
	add(titleStyle.Render(" Thèmes"))
	add("  --theme     default, dracula, solarized, mono")
	add("  NO_COLOR=1  désactive les couleurs")
	for len(lines) < h {
		lines = append(lines, "")
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	return lines
}
