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
	"github.com/rlch/scaf"
)

// TUIFormatter implements Formatter with an animated terminal UI.
type TUIFormatter struct {
	program  *tea.Program
	model    *tuiModel
	mu       sync.Mutex
	finished bool
}

// NewTUIFormatter creates a TUI formatter with animations.
func NewTUIFormatter(w io.Writer, suites []SuiteTree) *TUIFormatter {
	model := newTUIModel(suites)

	opts := []tea.ProgramOption{
		tea.WithOutput(w),
		tea.WithoutSignalHandler(),
		tea.WithAltScreen(), // Use alternate screen so animation doesn't pollute scrollback
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

	// Send done signal
	t.program.Send(doneMsg{result: result})
	time.Sleep(50 * time.Millisecond)

	// Quit and wait for program to exit cleanly
	t.program.Quit()
	time.Sleep(50 * time.Millisecond)

	// Print the final static output. The TUI used the alternate screen,
	// so exiting it returns us to the main screen with clean scrollback.
	fmt.Println(t.model.FinalView())

	return nil
}

// -----------------------------------------------------------------------------
// Tree Model - Built from Suite before tests run
// -----------------------------------------------------------------------------

// nodeKind identifies what type of tree node this is.
type nodeKind int

const (
	kindSuite nodeKind = iota
	kindScope
	kindGroup
	kindTest
)

// nodeStatus tracks the execution state of a node.
type nodeStatus int

const (
	statusPending nodeStatus = iota
	statusRunning
	statusPass
	statusFail
	statusSkip
	statusError
)

// treeNode represents a single node in the test tree.
type treeNode struct {
	name     string
	kind     nodeKind
	status   nodeStatus
	children []*treeNode
	parent   *treeNode

	// For leaf nodes (tests)
	elapsed time.Duration
	field   string
	expect  any
	actual  any
	err     error
}

// SuiteTree holds a parsed suite and its tree representation.
type SuiteTree struct {
	path string                // file path
	root *treeNode             // tree root for this suite
	idx  map[string]*treeNode  // "suite::path" -> node lookup
}

// BuildSuiteTree creates a tree representation from a parsed Suite.
func BuildSuiteTree(suite *scaf.Suite, suitePath string) SuiteTree {
	st := SuiteTree{
		path: suitePath,
		idx:  make(map[string]*treeNode),
	}

	// Root is the suite file
	st.root = &treeNode{
		name: suitePath,
		kind: kindSuite,
	}

	// Add query scopes
	for _, scope := range suite.Scopes {
		scopeNode := &treeNode{
			name:   scope.QueryName,
			kind:   kindScope,
			parent: st.root,
		}
		st.root.children = append(st.root.children, scopeNode)

		// Add items (tests and groups) - include suite path to avoid collisions
		addItems(scopeNode, scope.Items, []string{scope.QueryName}, suitePath, st.idx)
	}

	return st
}

func addItems(parent *treeNode, items []*scaf.TestOrGroup, pathPrefix []string, suitePath string, idx map[string]*treeNode) {
	for _, item := range items {
		if item.Test != nil {
			testNode := &treeNode{
				name:   item.Test.Name,
				kind:   kindTest,
				parent: parent,
			}
			parent.children = append(parent.children, testNode)

			// Index by "suite::path" to avoid collisions between files
			path := make([]string, len(pathPrefix)+1)
			copy(path, pathPrefix)
			path[len(pathPrefix)] = item.Test.Name
			key := suitePath + "::" + strings.Join(path, "/")
			idx[key] = testNode
		}

		if item.Group != nil {
			groupNode := &treeNode{
				name:   item.Group.Name,
				kind:   kindGroup,
				parent: parent,
			}
			parent.children = append(parent.children, groupNode)

			// Recurse into group (make a copy to avoid slice aliasing)
			groupPath := make([]string, len(pathPrefix)+1)
			copy(groupPath, pathPrefix)
			groupPath[len(pathPrefix)] = item.Group.Name
			addItems(groupNode, item.Group.Items, groupPath, suitePath, idx)
		}
	}
}

// -----------------------------------------------------------------------------
// Bubbletea Model
// -----------------------------------------------------------------------------

// tuiModel is the bubbletea model for the test runner UI.
type tuiModel struct {
	styles  *Styles
	spinner spinner.Model

	// State
	width  int
	height int

	// Test tree
	suites []SuiteTree
	allIdx map[string]*treeNode // combined index across all suites

	// Counters
	counters counters

	// Timing
	startTime time.Time
	endTime   time.Time

	// Final result
	finalResult *Result
	isDone      bool
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

func newTUIModel(suites []SuiteTree) *tuiModel {
	s := spinner.New()
	s.Spinner = spinner.Spinner{
		Frames: SpinnerFrames(),
		FPS:    time.Second / 10,
	}
	s.Style = DefaultStyles().Running

	// Build combined index
	allIdx := make(map[string]*treeNode)
	totalTests := 0

	for i := range suites {
		for path, node := range suites[i].idx {
			allIdx[path] = node
			totalTests++
		}
	}

	return &tuiModel{
		styles:    DefaultStyles(),
		spinner:   s,
		suites:    suites,
		allIdx:    allIdx,
		startTime: time.Now(),
		width:     80,
		height:    24,
		counters:  counters{total: totalTests},
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
	// Key format: "suite::path/to/test"
	key := event.Suite + "::" + event.PathString()
	node, ok := m.allIdx[key]

	if !ok {
		return // Unknown test path
	}

	switch event.Action {
	case ActionRun:
		node.status = statusRunning

	case ActionPass:
		node.status = statusPass
		node.elapsed = event.Elapsed
		m.counters.passed++

	case ActionFail:
		node.status = statusFail
		node.elapsed = event.Elapsed
		node.field = event.Field
		node.expect = event.Expected
		node.actual = event.Actual
		m.counters.failed++

	case ActionSkip:
		node.status = statusSkip
		node.elapsed = event.Elapsed
		m.counters.skipped++

	case ActionError:
		node.status = statusError
		node.elapsed = event.Elapsed
		node.err = event.Error
		m.counters.errors++
	}
}

// clearEOL is the ANSI escape sequence to clear from cursor to end of line.
const clearEOL = "\033[K"

// FinalView renders the complete final output for printing after the TUI exits.
// Unlike View(), this doesn't include clear-to-EOL sequences and uses a static
// checkmark instead of the spinner for any "running" items (shouldn't happen).
func (m *tuiModel) FinalView() string {
	var lines []string

	// Header
	lines = append(lines, m.renderHeader())

	// Progress bar
	lines = append(lines, m.renderProgress())
	lines = append(lines, "") // blank line

	// Test tree
	for _, st := range m.suites {
		treeLines := strings.Split(strings.TrimSuffix(m.renderTree(st), "\n"), "\n")
		lines = append(lines, treeLines...)
	}

	// Summary
	lines = append(lines, "")
	lines = append(lines, m.renderSummary())

	return strings.Join(lines, "\n")
}

func (m *tuiModel) View() string {
	var lines []string

	// Header
	lines = append(lines, m.renderHeader())

	// Progress bar
	lines = append(lines, m.renderProgress())
	lines = append(lines, "") // blank line

	// Test tree
	for _, st := range m.suites {
		treeLines := strings.Split(strings.TrimSuffix(m.renderTree(st), "\n"), "\n")
		lines = append(lines, treeLines...)
	}

	// Summary (only when done)
	if m.isDone {
		lines = append(lines, "")
		lines = append(lines, m.renderSummary())
	}

	// Add clear-to-EOL to each line to prevent rendering artifacts
	for i := range lines {
		lines[i] += clearEOL
	}

	return strings.Join(lines, "\n") + "\n"
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
	} else {
		running := m.countRunning()
		if running > 0 {
			status = m.styles.Running.Render(fmt.Sprintf("running %d", running))
		} else {
			status = m.styles.Dim.Render("starting")
		}
	}

	return fmt.Sprintf("%s%s  %s", logo, subtitle, status)
}

func (m *tuiModel) countRunning() int {
	count := 0

	for _, node := range m.allIdx {
		if node.status == statusRunning {
			count++
		}
	}

	return count
}

func (m *tuiModel) renderProgress() string {
	done := m.counters.passed + m.counters.failed + m.counters.skipped + m.counters.errors
	total := m.counters.total

	if total == 0 {
		total = 1
	}

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

func (m *tuiModel) renderTree(st SuiteTree) string {
	var b strings.Builder

	// Simple file header - just the path, dimmed
	b.WriteString(m.styles.Path.Render(st.path))
	b.WriteString("\n")

	// Render the tree
	for i, child := range st.root.children {
		isLast := i == len(st.root.children)-1
		m.renderNode(&b, child, "", isLast)
	}

	b.WriteString("\n")
	return b.String()
}

// computeGroupStatus calculates status for a group based on its children.
func (m *tuiModel) computeGroupStatus(node *treeNode) nodeStatus {
	if node.kind == kindTest {
		return node.status
	}

	hasRunning := false
	hasFailed := false
	hasPending := false
	allPassed := true

	for _, child := range node.children {
		childStatus := m.computeGroupStatus(child)
		switch childStatus {
		case statusRunning:
			hasRunning = true
			allPassed = false
		case statusFail, statusError:
			hasFailed = true
			allPassed = false
		case statusPending:
			hasPending = true
			allPassed = false
		case statusSkip:
			// Skip doesn't affect pass status
		case statusPass:
			// Good
		}
	}

	if hasRunning {
		return statusRunning
	}
	if hasFailed {
		return statusFail
	}
	if hasPending {
		return statusPending
	}
	if allPassed && len(node.children) > 0 {
		return statusPass
	}
	return statusPending
}

func (m *tuiModel) renderNode(b *strings.Builder, node *treeNode, prefix string, isLast bool) {
	// Tree branch character
	branch := "├─"
	if isLast {
		branch = "╰─"
	}

	// Status symbol (with group status inheritance)
	symbol := m.renderSymbol(node)

	// Name with appropriate styling
	name := node.name
	switch node.kind {
	case kindScope:
		name = m.styles.Bold.Render(name)
	case kindGroup:
		name = m.styles.Muted.Render(name)
	case kindTest:
		name = m.styles.TestName.Render(name)
	}

	// Duration (for completed tests only)
	dur := ""
	if node.kind == kindTest && node.status != statusPending && node.status != statusRunning {
		dur = m.styles.Dim.Render(fmt.Sprintf("  [%s]", formatDuration(node.elapsed)))
	}

	// Render the line
	b.WriteString(m.styles.Dim.Render(prefix + branch + " "))
	b.WriteString(symbol)
	b.WriteString(" ")
	b.WriteString(name)
	b.WriteString(dur)
	b.WriteString("\n")

	// Failure details (indented under the test)
	if node.status == statusFail && node.field != "" {
		detailPrefix := prefix
		if isLast {
			detailPrefix += "  "
		} else {
			detailPrefix += "│ "
		}

		detail := fmt.Sprintf("%s: expected %v, got %v", node.field, node.expect, node.actual)
		b.WriteString(m.styles.Dim.Render(detailPrefix + "   "))
		b.WriteString(m.styles.Fail.Render(detail))
		b.WriteString("\n")
	}

	if node.status == statusError && node.err != nil {
		detailPrefix := prefix
		if isLast {
			detailPrefix += "  "
		} else {
			detailPrefix += "│ "
		}

		b.WriteString(m.styles.Dim.Render(detailPrefix + "   "))
		b.WriteString(m.styles.Error.Render(node.err.Error()))
		b.WriteString("\n")
	}

	// Render children
	childPrefix := prefix
	if isLast {
		childPrefix += "  "
	} else {
		childPrefix += "│ "
	}

	for i, child := range node.children {
		childIsLast := i == len(node.children)-1
		m.renderNode(b, child, childPrefix, childIsLast)
	}
}

func (m *tuiModel) renderSymbol(node *treeNode) string {
	// For groups/scopes, compute status from children
	status := node.status
	if node.kind != kindTest {
		status = m.computeGroupStatus(node)
	}

	switch status {
	case statusPending:
		return m.styles.Dim.Render("⋯")
	case statusRunning:
		return m.spinner.View()
	case statusPass:
		return m.styles.Pass.Render(m.styles.SymbolPass)
	case statusFail:
		return m.styles.Fail.Render(m.styles.SymbolFail)
	case statusSkip:
		return m.styles.Skip.Render(m.styles.SymbolSkip)
	case statusError:
		return m.styles.Error.Render(m.styles.SymbolFail)
	default:
		return " "
	}
}

func (m *tuiModel) renderSummary() string {
	var parts []string

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

	return "  " + strings.Join(parts, sep) + " " + total
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

// -----------------------------------------------------------------------------
// TUIHandler - Bridges TUI to Handler interface
// -----------------------------------------------------------------------------

// TUIHandler wraps TUIFormatter to implement Handler.
type TUIHandler struct {
	formatter *TUIFormatter
	stderr    io.Writer
}

// NewTUIHandler creates a handler that uses the TUI formatter.
// Call SetSuites before Start to initialize the tree view.
func NewTUIHandler(w io.Writer, stderr io.Writer) *TUIHandler {
	return &TUIHandler{
		stderr: stderr,
	}
}

// SetSuites initializes the TUI with parsed suites for tree display.
func (h *TUIHandler) SetSuites(suites []SuiteTree) {
	h.formatter = NewTUIFormatter(os.Stdout, suites)
}

// Start initializes the TUI.
func (h *TUIHandler) Start() error {
	if h.formatter == nil {
		// Fallback: empty tree
		h.formatter = NewTUIFormatter(os.Stdout, nil)
	}

	return h.formatter.Start()
}

// Event sends an event to the TUI.
func (h *TUIHandler) Event(_ context.Context, event Event, result *Result) error {
	if h.formatter == nil {
		return nil
	}

	return h.formatter.Format(event, result)
}

// Err writes to stderr.
func (h *TUIHandler) Err(text string) error {
	_, err := h.stderr.Write([]byte(text + "\n"))

	return err
}

// Summary renders the final summary.
func (h *TUIHandler) Summary(result *Result) error {
	if h.formatter == nil {
		return nil
	}

	return h.formatter.Summary(result)
}