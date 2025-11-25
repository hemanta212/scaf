package runner

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestDotsFormatter_Format(t *testing.T) {
	var buf bytes.Buffer

	f := NewDotsFormatter(&buf)

	_ = f.Format(Event{Action: ActionRun}, nil)

	if buf.Len() != 0 {
		t.Error("Non-terminal should produce no output")
	}

	_ = f.Format(Event{Action: ActionPass}, nil)
	_ = f.Format(Event{Action: ActionFail}, nil)
	_ = f.Format(Event{Action: ActionSkip}, nil)
	_ = f.Format(Event{Action: ActionError}, nil)

	if got := buf.String(); got != ".FSE" {
		t.Errorf("got %q, want %q", got, ".FSE")
	}
}

func TestDotsFormatter_Summary(t *testing.T) {
	var buf bytes.Buffer

	f := NewDotsFormatter(&buf)

	result := NewResult()
	result.Add(Event{Action: ActionPass, Path: []string{"Test1"}})
	result.Add(Event{Action: ActionFail, Path: []string{"Test2"}, Field: "name", Expected: "a", Actual: "b"})
	result.Finish()

	_ = f.Summary(result)

	got := buf.String()

	if !bytes.Contains(buf.Bytes(), []byte("FAIL Test2")) {
		t.Errorf("missing 'FAIL Test2' in:\n%s", got)
	}

	if !bytes.Contains(buf.Bytes(), []byte("expected: a")) {
		t.Errorf("missing 'expected: a' in:\n%s", got)
	}

	if !bytes.Contains(buf.Bytes(), []byte("2 tests, 1 passed, 1 failed")) {
		t.Errorf("missing summary counts in:\n%s", got)
	}
}

func TestVerboseFormatter_Format(t *testing.T) {
	var buf bytes.Buffer

	f := NewVerboseFormatter(&buf)

	_ = f.Format(Event{Action: ActionRun, Path: []string{"Test1"}}, nil)

	if got, want := buf.String(), "=== RUN   Test1\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	buf.Reset()

	_ = f.Format(Event{Action: ActionPass, Path: []string{"Test1"}, Elapsed: 10 * time.Millisecond}, nil)

	if got, want := buf.String(), "--- PASS: Test1 (10ms)\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	buf.Reset()

	_ = f.Format(Event{Action: ActionFail, Path: []string{"Test1"}, Field: "x", Expected: 1, Actual: 2}, nil)

	want := `--- FAIL: Test1 (0s)
    x:
        expected: 1
        actual:   2
`
	if got := buf.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestJSONFormatter_Format(t *testing.T) {
	var buf bytes.Buffer

	f := NewJSONFormatter(&buf)

	fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	_ = f.Format(Event{
		Time:    fixedTime,
		Action:  ActionPass,
		Suite:   "test.scaf",
		Path:    []string{"Query", "Test"},
		Elapsed: 50 * time.Millisecond,
	}, nil)

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if got["action"] != "pass" {
		t.Errorf("action = %v, want pass", got["action"])
	}

	if got["path"] != "Query/Test" {
		t.Errorf("path = %v, want Query/Test", got["path"])
	}

	if got["test"] != "Test" {
		t.Errorf("test = %v, want Test", got["test"])
	}
}

func TestJSONFormatter_Summary(t *testing.T) {
	var buf bytes.Buffer

	f := NewJSONFormatter(&buf)

	result := NewResult()
	result.Add(Event{Action: ActionPass, Path: []string{"Test1"}})
	result.Add(Event{Action: ActionFail, Path: []string{"Test2"}})
	result.Finish()

	_ = f.Summary(result)

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if got["action"] != "summary" {
		t.Errorf("action = %v, want summary", got["action"])
	}

	total, ok := got["total"].(float64)
	if !ok || total != 2 {
		t.Errorf("total = %v, want 2", got["total"])
	}

	okVal, ok := got["ok"].(bool)
	if !ok || okVal {
		t.Errorf("ok = %v, want false", got["ok"])
	}
}
