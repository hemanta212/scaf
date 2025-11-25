package runner

import (
	"sync"
	"time"
)

// Result accumulates test results during execution.
type Result struct {
	mu sync.RWMutex

	StartTime time.Time
	EndTime   time.Time

	Total   int
	Passed  int
	Failed  int
	Skipped int
	Errors  int

	// Tests indexed by path string: "GetUser/existing users/finds Alice"
	Tests map[string]*TestResult

	// Order preserves insertion order for display
	Order []string
}

// NewResult creates an initialized Result.
func NewResult() *Result {
	return &Result{
		StartTime: time.Now(),
		Tests:     make(map[string]*TestResult),
	}
}

// Add records a terminal event in the result.
func (r *Result) Add(event Event) {
	if !event.Action.IsTerminal() {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	path := event.PathString()

	tr := &TestResult{
		Path:    event.Path,
		Status:  event.Action,
		Elapsed: event.Elapsed,
		Error:   event.Error,
	}

	if event.Action == ActionFail {
		tr.Expected = event.Expected
		tr.Actual = event.Actual
		tr.Field = event.Field
	}

	r.Tests[path] = tr
	r.Order = append(r.Order, path)
	r.Total++

	switch event.Action {
	case ActionPass:
		r.Passed++
	case ActionFail:
		r.Failed++
	case ActionSkip:
		r.Skipped++
	case ActionError:
		r.Errors++
	case ActionRun, ActionOutput, ActionSetup:
		// Not terminal actions
	}
}

// AddOutput appends output to an existing test result.
func (r *Result) AddOutput(event Event) {
	if event.Action != ActionOutput {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	path := event.PathString()
	if tr, ok := r.Tests[path]; ok {
		tr.Output = append(tr.Output, event.Output)
	}
}

// Finish marks the result as complete.
func (r *Result) Finish() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.EndTime = time.Now()
}

// Elapsed returns the total execution time.
func (r *Result) Elapsed() time.Duration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.EndTime.IsZero() {
		return time.Since(r.StartTime)
	}

	return r.EndTime.Sub(r.StartTime)
}

// Ok returns true if all tests passed.
func (r *Result) Ok() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.Failed == 0 && r.Errors == 0
}

// FailedTests returns all failed test results.
func (r *Result) FailedTests() []*TestResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var failed []*TestResult

	for _, path := range r.Order {
		tr := r.Tests[path]
		if tr.Status == ActionFail || tr.Status == ActionError {
			failed = append(failed, tr)
		}
	}

	return failed
}

// TestResult holds the outcome of a single test.
type TestResult struct {
	Path    []string
	Status  Action
	Elapsed time.Duration
	Error   error
	Output  []string

	// Assertion failure details
	Expected any
	Actual   any
	Field    string
}

// PathString returns the path as a slash-separated string.
func (tr *TestResult) PathString() string {
	result := ""

	for i, p := range tr.Path {
		if i > 0 {
			result += "/"
		}

		result += p
	}

	return result
}
