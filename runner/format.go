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
// Verbose Formatter (placeholder until Charm TUI)
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

// jsonError represents an error with source location.
type jsonError struct {
	Message  string `json:"message"`
	Line     *int   `json:"line,omitempty"`
	Severity int    `json:"severity,omitempty"` // 1=error, 2=warn, 3=info, 4=hint
}

type jsonEvent struct {
	Time     string       `json:"time"`
	Action   string       `json:"action"`
	ID       string       `json:"id"`
	Suite    string       `json:"suite,omitempty"`
	Path     string       `json:"path"`
	Test     string       `json:"test,omitempty"`
	Elapsed  float64      `json:"elapsed,omitempty"`
	Output   string       `json:"output,omitempty"`
	Short    string       `json:"short,omitempty"`
	Errors   []jsonError  `json:"errors,omitempty"`
	Field    string       `json:"field,omitempty"`
	Expected any          `json:"expected,omitempty"`
	Actual   any          `json:"actual,omitempty"`
}

// Format outputs a JSON event.
func (j *JSONFormatter) Format(event Event, _ *Result) error {
	je := jsonEvent{
		Time:   event.Time.Format(time.RFC3339Nano),
		Action: string(event.Action),
		ID:     event.ID(),
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
		je.Short = event.Error.Error()
		je.Errors = []jsonError{{
			Message:  event.Error.Error(),
			Line:     intPtr(event.Line),
			Severity: 1,
		}}
	}

	if event.Action == ActionFail {
		je.Field = event.Field
		je.Expected = event.Expected
		je.Actual = event.Actual

		if event.Field != "" {
			je.Short = fmt.Sprintf("%s: expected %v, got %v", event.Field, event.Expected, event.Actual)
			je.Errors = []jsonError{{
				Message:  je.Short,
				Line:     intPtr(event.Line),
				Severity: 1,
			}}
		}
	}

	return j.enc.Encode(je)
}

func intPtr(i int) *int {
	if i == 0 {
		return nil
	}

	return &i
}

type jsonTestResult struct {
	Status string      `json:"status"`
	Short  string      `json:"short,omitempty"`
	Errors []jsonError `json:"errors,omitempty"`
}

type jsonSummary struct {
	Action  string                    `json:"action"`
	Total   int                       `json:"total"`
	Passed  int                       `json:"passed"`
	Failed  int                       `json:"failed"`
	Skipped int                       `json:"skipped"`
	Errors  int                       `json:"errors"`
	Elapsed float64                   `json:"elapsed"`
	Ok      bool                      `json:"ok"`
	Results map[string]jsonTestResult `json:"results"`
}

// Summary outputs the final JSON summary.
func (j *JSONFormatter) Summary(result *Result) error {
	results := make(map[string]jsonTestResult, len(result.Tests))

	for _, tr := range result.Tests {
		jtr := jsonTestResult{
			Status: string(tr.Status),
		}

		if tr.Error != nil {
			jtr.Short = tr.Error.Error()
			jtr.Errors = []jsonError{{
				Message:  tr.Error.Error(),
				Line:     intPtr(tr.Line),
				Severity: 1,
			}}
		} else if tr.Status == ActionFail && tr.Field != "" {
			jtr.Short = fmt.Sprintf("%s: expected %v, got %v", tr.Field, tr.Expected, tr.Actual)
			jtr.Errors = []jsonError{{
				Message:  jtr.Short,
				Line:     intPtr(tr.Line),
				Severity: 1,
			}}
		}

		results[tr.ID()] = jtr
	}

	return j.enc.Encode(jsonSummary{
		Action:  "summary",
		Total:   result.Total,
		Passed:  result.Passed,
		Failed:  result.Failed,
		Skipped: result.Skipped,
		Errors:  result.Errors,
		Elapsed: result.Elapsed().Seconds(),
		Ok:      result.Ok(),
		Results: results,
	})
}


