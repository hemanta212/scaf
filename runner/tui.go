package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
)

// TUIFormatter implements Formatter with an animated terminal UI.
type TUIFormatter struct {
	program  *tea.Program
	model    *tuiModel
	mu       sync.Mutex
	finished bool
}

// NewTUIFormatter creates a TUI formatter with animations.
func NewTUIFormatter(w io.Writer) *TUIFormatter {
	model := newTUIModel()

	opts := []tea.ProgramOption{
		tea.WithOutput(w),
		tea.WithoutSignalHandler(),
	}

	// Only use input if we have a TTY
	if f, ok := w.(*os.File); ok && isatty.IsTerminal(f.Fd()) {
		// TTY mode - full interactive
	} else {
		// Non-TTY mode - disable input
		opts = append(opts, tea.WithInput(nil))
	}

	p := tea.NewProgram(model, opts...)

	return &TUIFormatter{
		program: p,
		model:   model,
	}
}

// Start begins the TUI event loop. Call this before running tests.
func (t *TUIFormatter) Start() error {
	go func() {
		_, _ = t.program.Run()
	}()

	// Give the program a moment to initialize
	time.Sleep(20 * time.Millisecond)

	return nil
}

// Format sends an event to the TUI.
func (t *TUIFormatter) Format(event Event, _ *Result) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.finished {
		return nil
	}

	t.program.Send(testEventMsg(event))

	return nil
}

// Summary waits for completion and renders final output.
func (t *TUIFormatter) Summary(result *Result) error {
	t.mu.Lock()
	t.finished = true
	t.mu.Unlock()

	// Send done signal and wait for final render
	t.program.Send(doneMsg{result: result})
	time.Sleep(50 * time.Millisecond)

	// Quit and wait for program to exit cleanly
	t.program.Quit()
	time.Sleep(50 * time.Millisecond)

	return nil
}

// tuiModel is the bubbletea model for the test runner UI.
type tuiModel struct {
	styles  *Styles
	spinner spinner.Model

	// State
	width  int
	height int

	// Test tracking
	running  map[string]*runningTest // path -> test info
	finished []finishedTest
	counters counters

	// Timing
	startTime time.Time
	endTime   time.Time

	// Final result
	finalResult *Result
	isDone      bool
}

type runningTest struct {
	path      []string
	suite     string
	startTime time.Time
}

type finishedTest struct {
	path    []string
	suite   string
	action  Action
	elapsed time.Duration
	err     error
	field   string
	expect  any
	actual  any
}

type counters struct {
	total   int
	passed  int
	failed  int
	skipped int
	errors  int
}

// Messages
type (
	tickMsg      time.Time
	testEventMsg Event
	doneMsg      struct{ result *Result }
)

func newTUIModel() *tuiModel {
	s := spinner.New()
	s.Spinner = spinner.Spinner{
		Frames: SpinnerFrames(),
		FPS:    time.Second / 10,
	}
	s.Style = DefaultStyles().Running

	return &tuiModel{
		styles:    DefaultStyles(),
		spinner:   s,
		running:   make(map[string]*runningTest),
		startTime: time.Now(),
		width:     80,
		height:    24,
	}
}

func (m *tuiModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.tick(),
	)
}

func (m *tuiModel) tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.QuitMsg:
		return m, tea.Quit

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		return m, nil

	case tickMsg:
		if !m.isDone {
			cmds = append(cmds, m.tick())
		}

	case spinner.TickMsg:
		if !m.isDone {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}

	case testEventMsg:
		m.handleEvent(Event(msg))

	case doneMsg:
		m.isDone = true
		m.endTime = time.Now()
		m.finalResult = msg.result
	}

	return m, tea.Batch(cmds...)
}

func (m *tuiModel) handleEvent(event Event) {
	pathStr := event.PathString()

	switch event.Action {
	case ActionRun:
		m.running[pathStr] = &runningTest{
			path:      event.Path,
			suite:     event.Suite,
			startTime: event.Time,
		}

	case ActionPass:
		delete(m.running, pathStr)
		m.counters.total++
		m.counters.passed++

		m.finished = append(m.finished, finishedTest{
			path:    event.Path,
			suite:   event.Suite,
			action:  ActionPass,
			elapsed: event.Elapsed,
		})

	case ActionFail:
		delete(m.running, pathStr)
		m.counters.total++
		m.counters.failed++

		m.finished = append(m.finished, finishedTest{
			path:    event.Path,
			suite:   event.Suite,
			action:  ActionFail,
			elapsed: event.Elapsed,
			field:   event.Field,
			expect:  event.Expected,
			actual:  event.Actual,
		})

	case ActionSkip:
		delete(m.running, pathStr)
		m.counters.total++
		m.counters.skipped++

		m.finished = append(m.finished, finishedTest{
			path:    event.Path,
			suite:   event.Suite,
			action:  ActionSkip,
			elapsed: event.Elapsed,
		})

	case ActionError:
		delete(m.running, pathStr)
		m.counters.total++
		m.counters.errors++

		m.finished = append(m.finished, finishedTest{
			path:    event.Path,
			suite:   event.Suite,
			action:  ActionError,
			elapsed: event.Elapsed,
			err:     event.Error,
		})
	}
}

func (m *tuiModel) View() string {
	var b strings.Builder

	// Header with logo and status
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")

	// Progress section
	b.WriteString(m.renderProgress())
	b.WriteString("\n")

	// Running tests (if any and not done)
	if len(m.running) > 0 && !m.isDone {
		b.WriteString("\n")
		b.WriteString(m.renderRunning())
		b.WriteString("\n")
	}

	// Recent finished tests
	b.WriteString("\n")
	b.WriteString(m.renderFinished())

	// Footer with summary
	b.WriteString("\n\n")
	b.WriteString(m.renderSummary())
	b.WriteString("\n")

	return b.String()
}

func (m *tuiModel) renderHeader() string {
	logo := m.styles.Bold.Render("scaf")
	subtitle := m.styles.Dim.Render(" test")

	var status string
	if m.isDone {
		if m.counters.failed > 0 || m.counters.errors > 0 {
			status = m.styles.Fail.Render("FAIL")
		} else {
			status = m.styles.Pass.Render("PASS")
		}
	} else if len(m.running) > 0 {
		status = m.styles.Running.Render("running")
	} else {
		status = m.styles.Dim.Render("starting")
	}

	return fmt.Sprintf("%s%s  %s", logo, subtitle, status)
}

func (m *tuiModel) renderProgress() string {
	total := m.counters.total + len(m.running)
	if total == 0 {
		total = 1 // Avoid division by zero
	}

	done := m.counters.total
	pct := float64(done) / float64(total)

	// Elapsed time
	elapsed := time.Since(m.startTime)
	if !m.endTime.IsZero() {
		elapsed = m.endTime.Sub(m.startTime)
	}

	elapsedStr := m.styles.Dim.Render(fmt.Sprintf("[%s]", formatDuration(elapsed)))

	// Progress bar
	barWidth := 30
	filled := int(pct * float64(barWidth))
	filledChar, emptyChar := ProgressChars()

	bar := m.styles.ProgressFilled.Render(strings.Repeat(filledChar, filled)) +
		m.styles.ProgressEmpty.Render(strings.Repeat(emptyChar, barWidth-filled))

	// Counter
	counter := m.styles.Muted.Render(fmt.Sprintf("%d/%d", done, total))

	return fmt.Sprintf("%s %s %s", elapsedStr, bar, counter)
}

func (m *tuiModel) renderRunning() string {
	var lines []string

	header := m.styles.Dim.Render("  Running:")
	lines = append(lines, header)

	// Show up to 5 running tests
	count := 0

	for _, rt := range m.running {
		if count >= 5 {
			break
		}

		elapsed := time.Since(rt.startTime)
		isSlow := elapsed > 300*time.Millisecond

		symbol := m.spinner.View()
		if isSlow {
			symbol = m.styles.Slow.Render(m.styles.SymbolSlow)
		}

		name := m.styles.TestName.Render(rt.path[len(rt.path)-1])
		dur := m.styles.Duration.Render(formatDuration(elapsed))

		if isSlow {
			dur = m.styles.Slow.Render(formatDuration(elapsed))
		}

		line := fmt.Sprintf("    %s %s %s", symbol, name, dur)
		lines = append(lines, line)
		count++
	}

	if len(m.running) > 5 {
		overflow := m.styles.Dim.Render(fmt.Sprintf("    ... and %d more", len(m.running)-5))
		lines = append(lines, overflow)
	}

	return strings.Join(lines, "\n")
}

func (m *tuiModel) renderFinished() string {
	var lines []string

	// Show last 10 finished tests (or all if done)
	start := 0
	limit := 10

	if m.isDone {
		limit = len(m.finished) // Show all when done
	}

	if len(m.finished) > limit {
		start = len(m.finished) - limit
	}

	for i := start; i < len(m.finished); i++ {
		ft := m.finished[i]
		lines = append(lines, m.renderTestResult(ft))
	}

	return strings.Join(lines, "\n")
}

func (m *tuiModel) renderTestResult(ft finishedTest) string {
	var symbol, status string

	switch ft.action {
	case ActionPass:
		symbol = m.styles.Pass.Render(m.styles.SymbolPass)
		status = m.styles.Pass.Render("PASS")
	case ActionFail:
		symbol = m.styles.Fail.Render(m.styles.SymbolFail)
		status = m.styles.Fail.Render("FAIL")
	case ActionSkip:
		symbol = m.styles.Skip.Render(m.styles.SymbolSkip)
		status = m.styles.Skip.Render("SKIP")
	case ActionError:
		symbol = m.styles.Error.Render(m.styles.SymbolFail)
		status = m.styles.Error.Render("ERR ")
	default:
		symbol = " "
		status = "    "
	}

	// Build path with tree structure
	name := strings.Join(ft.path, "/")
	dur := m.styles.Dim.Render(fmt.Sprintf("[%s]", formatDuration(ft.elapsed)))

	line := fmt.Sprintf("  %s %s %s %s", symbol, padRight(status, 4), dur, m.styles.TestName.Render(name))

	// Add failure details
	if ft.action == ActionFail && ft.field != "" {
		detail := m.styles.Dim.Render(fmt.Sprintf("\n       %s: expected %v, got %v",
			ft.field, ft.expect, ft.actual))
		line += detail
	}

	if ft.action == ActionError && ft.err != nil {
		detail := m.styles.Dim.Render(fmt.Sprintf("\n       %v", ft.err))
		line += detail
	}

	return line
}

func (m *tuiModel) renderSummary() string {
	var parts []string

	// Status counts
	if m.counters.passed > 0 {
		parts = append(parts, m.styles.Pass.Render(fmt.Sprintf("%d passed", m.counters.passed)))
	}

	if m.counters.failed > 0 {
		parts = append(parts, m.styles.Fail.Render(fmt.Sprintf("%d failed", m.counters.failed)))
	}

	if m.counters.skipped > 0 {
		parts = append(parts, m.styles.Skip.Render(fmt.Sprintf("%d skipped", m.counters.skipped)))
	}

	if m.counters.errors > 0 {
		parts = append(parts, m.styles.Error.Render(fmt.Sprintf("%d errors", m.counters.errors)))
	}

	if len(parts) == 0 {
		return m.styles.Dim.Render("  No tests run")
	}

	total := m.styles.Muted.Render(fmt.Sprintf("(%d total)", m.counters.total))

	sep := m.styles.Dim.Render(" │ ")
	summary := strings.Join(parts, sep)

	return fmt.Sprintf("  %s %s", summary, total)
}

// Helper functions

func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return "<1ms"
	}

	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}

	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}

	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}

	return s + strings.Repeat(" ", width-len(s))
}

// TUIHandler wraps TUIFormatter to implement Handler.
type TUIHandler struct {
	formatter *TUIFormatter
	stderr    io.Writer
}

// NewTUIHandler creates a handler that uses the TUI formatter.
func NewTUIHandler(w io.Writer, stderr io.Writer) *TUIHandler {
	return &TUIHandler{
		formatter: NewTUIFormatter(w),
		stderr:    stderr,
	}
}

// Start initializes the TUI.
func (h *TUIHandler) Start() error {
	return h.formatter.Start()
}

// Event sends an event to the TUI.
func (h *TUIHandler) Event(_ context.Context, event Event, result *Result) error {
	return h.formatter.Format(event, result)
}

// Err writes to stderr.
func (h *TUIHandler) Err(text string) error {
	_, err := h.stderr.Write([]byte(text + "\n"))

	return err
}

// Summary renders the final summary.
func (h *TUIHandler) Summary(result *Result) error {
	return h.formatter.Summary(result)
}
