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
	ActionRun    Action = "run"
	ActionPass   Action = "passed"
	ActionFail   Action = "failed"
	ActionSkip   Action = "skipped"
	ActionError  Action = "error"
	ActionOutput Action = "output"
	ActionSetup  Action = "setup"
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

	// Source location for diagnostics
	Line int // 0-indexed line number in source file
}

// PathString returns the path as a slash-separated string.
func (e Event) PathString() string {
	return strings.Join(e.Path, "/")
}

// ID returns a unique identifier: "suite::path::components".
func (e Event) ID() string {
	if e.Suite == "" {
		return strings.Join(e.Path, "::")
	}

	return e.Suite + "::" + strings.Join(e.Path, "::")
}

// TestName returns the leaf test name.
func (e Event) TestName() string {
	if len(e.Path) == 0 {
		return ""
	}

	return e.Path[len(e.Path)-1]
}
