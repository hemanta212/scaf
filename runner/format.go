package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// Formatter renders test events and results.
type Formatter interface {
	Format(event Event, result *Result) error
	Summary(result *Result) error
}

// FormatHandler is a Handler that delegates to a Formatter.
type FormatHandler struct {
	formatter Formatter
	stderr    io.Writer
}

// NewFormatHandler creates a handler that formats events.
func NewFormatHandler(f Formatter, stderr io.Writer) *FormatHandler {
	return &FormatHandler{formatter: f, stderr: stderr}
}

// Event formats the event.
func (h *FormatHandler) Event(_ context.Context, event Event, result *Result) error {
	return h.formatter.Format(event, result)
}

// Err writes to stderr.
func (h *FormatHandler) Err(text string) error {
	_, err := h.stderr.Write([]byte(text + "\n"))

	return err
}

// Summary renders the final summary.
func (h *FormatHandler) Summary(result *Result) error {
	return h.formatter.Summary(result)
}

// -----------------------------------------------------------------------------
// Dots Formatter
// -----------------------------------------------------------------------------

// DotsFormatter is a minimal formatter that prints dots for progress.
type DotsFormatter struct {
	w     io.Writer
	count int
}

// NewDotsFormatter creates a dots formatter.
func NewDotsFormatter(w io.Writer) *DotsFormatter {
	return &DotsFormatter{w: w}
}

const lineWidth = 80

// Format prints a single character per terminal event.
func (d *DotsFormatter) Format(event Event, _ *Result) error {
	if !event.Action.IsTerminal() {
		return nil
	}

	var char string

	switch event.Action {
	case ActionPass:
		char = "."
	case ActionFail:
		char = "F"
	case ActionSkip:
		char = "S"
	case ActionError:
		char = "E"
	case ActionRun, ActionOutput, ActionSetup:
		return nil
	}

	_, err := fmt.Fprint(d.w, char)
	d.count++

	if d.count%lineWidth == 0 {
		_, _ = fmt.Fprintln(d.w)
	}

	return err
}

// Summary prints the final results.
func (d *DotsFormatter) Summary(result *Result) error {
	if d.count > 0 && d.count%lineWidth != 0 {
		_, _ = fmt.Fprintln(d.w)
	}

	_, _ = fmt.Fprintln(d.w)

	for _, tr := range result.FailedTests() {
		switch tr.Status {
		case ActionFail:
			_, _ = fmt.Fprintf(d.w, "FAIL %s\n", tr.PathString())

			if tr.Field != "" {
				_, _ = fmt.Fprintf(d.w, "  %s:\n", tr.Field)
				_, _ = fmt.Fprintf(d.w, "    expected: %v\n", tr.Expected)
				_, _ = fmt.Fprintf(d.w, "    actual:   %v\n", tr.Actual)
			}
		case ActionError:
			_, _ = fmt.Fprintf(d.w, "ERROR %s: %v\n", tr.PathString(), tr.Error)
		case ActionPass, ActionSkip, ActionRun, ActionOutput, ActionSetup:
			// Not failures
		}

		_, _ = fmt.Fprintln(d.w)
	}

	status := "PASS"
	if !result.Ok() {
		status = "FAIL"
	}

	_, _ = fmt.Fprintf(d.w, "%s %d tests, %d passed, %d failed, %d skipped in %s\n",
		status,
		result.Total,
		result.Passed,
		result.Failed,
		result.Skipped,
		result.Elapsed().Round(time.Millisecond),
	)

	return nil
}

// -----------------------------------------------------------------------------
// Verbose Formatter
// -----------------------------------------------------------------------------

// VerboseFormatter prints full test names and output.
type VerboseFormatter struct {
	w io.Writer
}

// NewVerboseFormatter creates a verbose formatter.
func NewVerboseFormatter(w io.Writer) *VerboseFormatter {
	return &VerboseFormatter{w: w}
}

// Format prints each event as it occurs.
func (v *VerboseFormatter) Format(event Event, _ *Result) error {
	switch event.Action {
	case ActionRun:
		_, _ = fmt.Fprintf(v.w, "=== RUN   %s\n", event.PathString())
	case ActionPass:
		_, _ = fmt.Fprintf(v.w, "--- PASS: %s (%s)\n", event.PathString(), event.Elapsed)
	case ActionFail:
		_, _ = fmt.Fprintf(v.w, "--- FAIL: %s (%s)\n", event.PathString(), event.Elapsed)

		if event.Field != "" {
			_, _ = fmt.Fprintf(v.w, "    %s:\n", event.Field)
			_, _ = fmt.Fprintf(v.w, "        expected: %v\n", event.Expected)
			_, _ = fmt.Fprintf(v.w, "        actual:   %v\n", event.Actual)
		}
	case ActionSkip:
		_, _ = fmt.Fprintf(v.w, "--- SKIP: %s (%s)\n", event.PathString(), event.Elapsed)
	case ActionError:
		_, _ = fmt.Fprintf(v.w, "--- ERROR: %s (%s)\n", event.PathString(), event.Elapsed)
		_, _ = fmt.Fprintf(v.w, "    %v\n", event.Error)
	case ActionOutput:
		_, _ = fmt.Fprintf(v.w, "    %s\n", event.Output)
	case ActionSetup:
		_, _ = fmt.Fprintf(v.w, "=== SETUP %s\n", event.PathString())
	}

	return nil
}

// Summary prints the final results.
func (v *VerboseFormatter) Summary(result *Result) error {
	_, _ = fmt.Fprintln(v.w)

	status := "PASS"
	if !result.Ok() {
		status = "FAIL"
	}

	_, _ = fmt.Fprintf(v.w, "%s\n", status)
	_, _ = fmt.Fprintf(v.w, "  %d total, %d passed, %d failed, %d skipped, %d errors\n",
		result.Total,
		result.Passed,
		result.Failed,
		result.Skipped,
		result.Errors,
	)
	_, _ = fmt.Fprintf(v.w, "  elapsed: %s\n", result.Elapsed().Round(time.Millisecond))

	return nil
}

// -----------------------------------------------------------------------------
// JSON Formatter
// -----------------------------------------------------------------------------

// JSONFormatter outputs newline-delimited JSON events.
type JSONFormatter struct {
	enc *json.Encoder
}

// NewJSONFormatter creates a JSON formatter.
func NewJSONFormatter(w io.Writer) *JSONFormatter {
	return &JSONFormatter{enc: json.NewEncoder(w)}
}

type jsonEvent struct {
	Time     string  `json:"time"`
	Action   string  `json:"action"`
	Suite    string  `json:"suite,omitempty"`
	Path     string  `json:"path"`
	Test     string  `json:"test,omitempty"`
	Elapsed  float64 `json:"elapsed,omitempty"`
	Output   string  `json:"output,omitempty"`
	Error    string  `json:"error,omitempty"`
	Field    string  `json:"field,omitempty"`
	Expected any     `json:"expected,omitempty"`
	Actual   any     `json:"actual,omitempty"`
}

// Format outputs a JSON event.
func (j *JSONFormatter) Format(event Event, _ *Result) error {
	je := jsonEvent{
		Time:   event.Time.Format(time.RFC3339Nano),
		Action: string(event.Action),
		Suite:  event.Suite,
		Path:   event.PathString(),
		Test:   event.TestName(),
	}

	if event.Action.IsTerminal() {
		je.Elapsed = event.Elapsed.Seconds()
	}

	if event.Output != "" {
		je.Output = event.Output
	}

	if event.Error != nil {
		je.Error = event.Error.Error()
	}

	if event.Action == ActionFail {
		je.Field = event.Field
		je.Expected = event.Expected
		je.Actual = event.Actual
	}

	return j.enc.Encode(je)
}

type jsonSummary struct {
	Action  string  `json:"action"`
	Total   int     `json:"total"`
	Passed  int     `json:"passed"`
	Failed  int     `json:"failed"`
	Skipped int     `json:"skipped"`
	Errors  int     `json:"errors"`
	Elapsed float64 `json:"elapsed"`
	Ok      bool    `json:"ok"`
}

// Summary outputs the final JSON summary.
func (j *JSONFormatter) Summary(result *Result) error {
	return j.enc.Encode(jsonSummary{
		Action:  "summary",
		Total:   result.Total,
		Passed:  result.Passed,
		Failed:  result.Failed,
		Skipped: result.Skipped,
		Errors:  result.Errors,
		Elapsed: result.Elapsed().Seconds(),
		Ok:      result.Ok(),
	})
}

// NewFormatter creates a formatter by name.
func NewFormatter(name string, w io.Writer) *formatterWrapper {
	var f Formatter

	switch name {
	case "verbose":
		f = NewVerboseFormatter(w)
	case "json":
		f = NewJSONFormatter(w)
	default:
		f = NewDotsFormatter(w)
	}

	return &formatterWrapper{f}
}

type formatterWrapper struct {
	Formatter
}
