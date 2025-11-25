// Package runner implements the scaf test execution engine.
package runner

import (
	"strings"
	"time"
)

// Action represents the type of test event.
type Action string

// Action constants for test events.
const (
	ActionRun    Action = "run"    // Test/group starting
	ActionPass   Action = "pass"   // Test passed
	ActionFail   Action = "fail"   // Test failed
	ActionSkip   Action = "skip"   // Test skipped
	ActionError  Action = "error"  // Infrastructure error
	ActionOutput Action = "output" // Log/debug output
	ActionSetup  Action = "setup"  // Setup block executing
)

// IsTerminal returns true if this action ends a test.
func (a Action) IsTerminal() bool {
	return a == ActionPass || a == ActionFail || a == ActionSkip || a == ActionError
}

// Event represents a single test event emitted during execution.
type Event struct {
	Time    time.Time     // When the event occurred
	Action  Action        // What happened
	Suite   string        // Source file path
	Path    []string      // Test path: ["QueryScope", "Group", "Test"]
	Elapsed time.Duration // Time taken (for terminal events)
	Output  string        // Log output (for ActionOutput)
	Error   error         // Error details (for ActionFail/ActionError)

	// For assertion failures
	Expected any
	Actual   any
	Field    string // Which field failed (e.g., "u.name")
}

// PathString returns the path as a slash-separated string.
func (e Event) PathString() string {
	return strings.Join(e.Path, "/")
}

// TestName returns the leaf test name.
func (e Event) TestName() string {
	if len(e.Path) == 0 {
		return ""
	}

	return e.Path[len(e.Path)-1]
}
