package scaf_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/rlch/scaf"
)

func TestCommentAttachment(t *testing.T) {
	// These tests cannot run in parallel because they share lexer trivia state

	tests := []struct {
		name            string
		input           string
		wantSuite       []string
		wantFirstQuery  []string
		wantSecondQuery []string // optional, for multi-query tests
	}{
		{
			name:           "comment directly before query attaches to query",
			input:          "// This is a query doc comment\nfn GetUser() `MATCH (u:User) RETURN u`\n",
			wantSuite:      nil,
			wantFirstQuery: []string{"// This is a query doc comment"},
		},
		{
			name:           "comment separated by blank line attaches to suite",
			input:          "// This is a suite/file comment\n\n// This is a query doc comment\nfn GetUser() `MATCH (u:User) RETURN u`\n",
			wantSuite:      []string{"// This is a suite/file comment"},
			wantFirstQuery: []string{"// This is a query doc comment"},
		},
		{
			name:           "multi-line query doc comment stays together",
			input:          "// First line of query doc\n// Second line of query doc\nfn GetUser() `MATCH (u:User) RETURN u`\n",
			wantSuite:      nil,
			wantFirstQuery: []string{"// First line of query doc", "// Second line of query doc"},
		},
		{
			name:           "suite comment only when blank line before query",
			input:          "// Suite level comment\n\nfn GetUser() `MATCH (u:User) RETURN u`\n",
			wantSuite:      []string{"// Suite level comment"},
			wantFirstQuery: nil,
		},
		{
			name:           "multiple consecutive suite comments with blank line before query doc",
			input:          "// Suite comment 1\n// Suite comment 2\n\n// Query doc\nfn GetUser() `MATCH (u:User) RETURN u`\n",
			wantSuite:      []string{"// Suite comment 1", "// Suite comment 2"},
			wantFirstQuery: []string{"// Query doc"},
		},
		{
			name:            "multiple queries with separate doc comments",
			input:           "// Doc for first\nfn First() `Q1`\n\n// Doc for second\nfn Second() `Q2`\n",
			wantSuite:       nil,
			wantFirstQuery:  []string{"// Doc for first"},
			wantSecondQuery: []string{"// Doc for second"},
		},
		{
			name:           "no comments",
			input:          "fn GetUser() `MATCH (u:User) RETURN u`\n",
			wantSuite:      nil,
			wantFirstQuery: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suite, err := scaf.Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			if diff := cmp.Diff(tt.wantSuite, suite.LeadingComments); diff != "" {
				t.Errorf("Suite.LeadingComments mismatch (-want +got):\n%s", diff)
			}

			if len(suite.Functions) > 0 {
				if diff := cmp.Diff(tt.wantFirstQuery, suite.Functions[0].LeadingComments); diff != "" {
					t.Errorf("Functions[0].LeadingComments mismatch (-want +got):\n%s", diff)
				}
			}

			if len(suite.Functions) > 1 && tt.wantSecondQuery != nil {
				if diff := cmp.Diff(tt.wantSecondQuery, suite.Functions[1].LeadingComments); diff != "" {
					t.Errorf("Functions[1].LeadingComments mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestTrailingCommentAttachment(t *testing.T) {
	input := "fn GetUser() `MATCH (u:User) RETURN u` // trailing comment\n"

	suite, err := scaf.Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(suite.Functions) == 0 {
		t.Fatal("Expected at least one query")
	}

	expected := "// trailing comment"
	if suite.Functions[0].TrailingComment != expected {
		t.Errorf("TrailingComment = %q, want %q", suite.Functions[0].TrailingComment, expected)
	}
}

func TestScopeCommentAttachment(t *testing.T) {
	input := `fn Q() ` + "`Q`" + `

// Scope doc comment
Q {
	// Test doc comment
	test "example" {
	}
}
`
	suite, err := scaf.Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(suite.Scopes) == 0 {
		t.Fatal("Expected at least one scope")
	}

	scope := suite.Scopes[0]
	wantScopeComments := []string{"// Scope doc comment"}
	if diff := cmp.Diff(wantScopeComments, scope.LeadingComments); diff != "" {
		t.Errorf("Scope.LeadingComments mismatch (-want +got):\n%s", diff)
	}

	if len(scope.Items) == 0 || scope.Items[0].Test == nil {
		t.Fatal("Expected at least one test in scope")
	}

	test := scope.Items[0].Test
	wantTestComments := []string{"// Test doc comment"}
	if diff := cmp.Diff(wantTestComments, test.LeadingComments); diff != "" {
		t.Errorf("Test.LeadingComments mismatch (-want +got):\n%s", diff)
	}
}

func TestParameterCommentAttachment(t *testing.T) {
	// Tests for parameter doc comments in multi-line function signatures
	input := `fn CreateUser(
	// The user's unique identifier
	id: string,
	// The user's display name
	name: string,
) ` + "`CREATE (u:User {id: $id, name: $name}) RETURN u`" + `
`
	suite, err := scaf.Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(suite.Functions) == 0 {
		t.Fatal("Expected at least one function")
	}

	fn := suite.Functions[0]
	if len(fn.Params) != 2 {
		t.Fatalf("Expected 2 params, got %d", len(fn.Params))
	}

	// Check first param doc comment
	wantIdComments := []string{"// The user's unique identifier"}
	if diff := cmp.Diff(wantIdComments, fn.Params[0].LeadingComments); diff != "" {
		t.Errorf("Params[0].LeadingComments mismatch (-want +got):\n%s", diff)
	}

	// Check second param doc comment
	wantNameComments := []string{"// The user's display name"}
	if diff := cmp.Diff(wantNameComments, fn.Params[1].LeadingComments); diff != "" {
		t.Errorf("Params[1].LeadingComments mismatch (-want +got):\n%s", diff)
	}
}

func TestStatementCommentAttachment(t *testing.T) {
	// Tests for statement doc comments within tests
	input := `fn Q() ` + "`Q`" + `

Q {
	test "example" {
		// Comment for input
		$id: 1
		// Comment for output
		u.name: "Alice"
	}
}
`
	suite, err := scaf.Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(suite.Scopes) == 0 || len(suite.Scopes[0].Items) == 0 {
		t.Fatal("Expected scope with test")
	}

	test := suite.Scopes[0].Items[0].Test
	if len(test.Statements) != 2 {
		t.Fatalf("Expected 2 statements, got %d", len(test.Statements))
	}

	// Check input statement comment
	wantInputComments := []string{"// Comment for input"}
	if diff := cmp.Diff(wantInputComments, test.Statements[0].LeadingComments); diff != "" {
		t.Errorf("Statements[0].LeadingComments mismatch (-want +got):\n%s", diff)
	}

	// Check output statement comment
	wantOutputComments := []string{"// Comment for output"}
	if diff := cmp.Diff(wantOutputComments, test.Statements[1].LeadingComments); diff != "" {
		t.Errorf("Statements[1].LeadingComments mismatch (-want +got):\n%s", diff)
	}
}

func TestAssertCommentAttachment(t *testing.T) {
	// Tests for assert doc comments
	input := `fn Q() ` + "`Q`" + `

Q {
	test "example" {
		// Verify the user exists
		assert (u != null)
	}
}
`
	suite, err := scaf.Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(suite.Scopes) == 0 || len(suite.Scopes[0].Items) == 0 {
		t.Fatal("Expected scope with test")
	}

	test := suite.Scopes[0].Items[0].Test
	if len(test.Asserts) == 0 {
		t.Fatal("Expected at least one assert")
	}

	wantComments := []string{"// Verify the user exists"}
	if diff := cmp.Diff(wantComments, test.Asserts[0].LeadingComments); diff != "" {
		t.Errorf("Asserts[0].LeadingComments mismatch (-want +got):\n%s", diff)
	}
}

// =============================================================================
// Round-trip tests for comment preservation
// =============================================================================

// TestCommentRoundTrip verifies that formatting preserves all comments.
// This is the comprehensive test that catches any missing comment handling.
func TestCommentRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "function trailing comment",
			input: `fn Q() ` + "`Q`" + ` // function trailing
`,
		},
		{
			name: "function leading comment",
			input: `// function doc
fn Q() ` + "`Q`" + `
`,
		},
		{
			name: "scope trailing comment",
			input: `fn Q() ` + "`Q`" + `

Q {
	test "t" {
	}
} // scope trailing
`,
		},
		{
			name: "scope leading comment",
			input: `fn Q() ` + "`Q`" + `

// scope doc
Q {
	test "t" {
	}
}
`,
		},
		{
			name: "test trailing comment",
			input: `fn Q() ` + "`Q`" + `

Q {
	test "t" {
	} // test trailing
}
`,
		},
		{
			name: "test leading comment",
			input: `fn Q() ` + "`Q`" + `

Q {
	// test doc
	test "t" {
	}
}
`,
		},
		{
			name: "group trailing comment",
			input: `fn Q() ` + "`Q`" + `

Q {
	group "g" {
		test "t" {
		}
	} // group trailing
}
`,
		},
		{
			name: "group leading comment",
			input: `fn Q() ` + "`Q`" + `

Q {
	// group doc
	group "g" {
		test "t" {
		}
	}
}
`,
		},
		{
			name: "statement trailing comment",
			input: `fn Q() ` + "`Q`" + `

Q {
	test "t" {
		$id: 1 // input trailing
	}
}
`,
		},
		{
			name: "statement leading comment",
			input: `fn Q() ` + "`Q`" + `

Q {
	test "t" {
		// input doc
		$id: 1
	}
}
`,
		},
		{
			name: "assert shorthand trailing comment",
			input: `fn Q() ` + "`Q`" + `

Q {
	test "t" {
		assert (x > 0) // assert trailing
	}
}
`,
		},
		{
			name: "assert shorthand leading comment",
			input: `fn Q() ` + "`Q`" + `

Q {
	test "t" {
		// assert doc
		assert (x > 0)
	}
}
`,
		},
		{
			name: "assert block trailing comment",
			input: `fn Q() ` + "`Q`" + `

Q {
	test "t" {
		assert ` + "`Q`" + ` { (x > 0) } // assert trailing
	}
}
`,
		},
		{
			name: "import trailing comment",
			input: `import "foo" // import trailing

fn Q() ` + "`Q`" + `
`,
		},
		{
			name: "import leading comment",
			input: `// import doc
import "foo"

fn Q() ` + "`Q`" + `
`,
		},
		{
			name: "parameter comments in multi-line function",
			input: `fn Q(
	// param a doc
	a: string,
	// param b doc
	b: int,
) ` + "`Q`" + `
`,
		},
		{
			name: "multiple comments throughout file",
			input: `// File header comment

// Import section
import "foo" // inline import comment

// Function docs
fn Q(
	// param doc
	id: string,
) ` + "`Q`" + ` // function trailing

// Scope docs
Q {
	// Test docs
	test "example" {
		// Input section
		$id: 1 // input comment

		// Assert section
		assert (x > 0) // assert comment
	} // test trailing
} // scope trailing
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse
			suite, err := scaf.Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			// Format
			output := scaf.Format(suite)

			// Parse again
			suite2, err := scaf.Parse([]byte(output))
			if err != nil {
				t.Fatalf("Parse(formatted) error = %v\n\nFormatted:\n%s", err, output)
			}

			// Format again (idempotency check)
			output2 := scaf.Format(suite2)

			// Check idempotency
			if diff := cmp.Diff(output, output2); diff != "" {
				t.Errorf("Format() not idempotent (-first +second):\n%s\n\nFirst:\n%s\n\nSecond:\n%s", diff, output, output2)
			}

			// The key test: check that all comments from input appear in output
			// We can't do exact string matching because formatting might normalize whitespace
			// But all comment text should be preserved
			checkCommentsPreserved(t, tt.input, output)
		})
	}
}

// checkCommentsPreserved verifies all comment text from input appears in output.
func checkCommentsPreserved(t *testing.T, input, output string) {
	t.Helper()

	// Extract all comments from input (lines starting with // after trimming)
	inputLines := splitLines(input)
	for _, line := range inputLines {
		trimmed := trimLeft(line)
		if len(trimmed) >= 2 && trimmed[:2] == "//" {
			// Found a comment - make sure it appears in output
			if !containsString(output, trimmed) {
				t.Errorf("Comment %q from input not found in output.\n\nInput:\n%s\n\nOutput:\n%s", trimmed, input, output)
			}
		}
	}
}

// splitLines splits a string into lines.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// trimLeft trims leading whitespace from a string.
func trimLeft(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			return s[i:]
		}
	}
	return ""
}

// containsString checks if substr appears in s.
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestTrailingCommentOnAssertBlock(t *testing.T) {
	// Specific test for the original issue - trailing comments after assert blocks
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "shorthand assert with trailing",
			input: `fn Q() ` + "`Q`" + `

Q {
	test "t" {
		assert (x > 0) // trailing
	}
}
`,
		},
		{
			name: "block assert single condition with trailing",
			input: `fn Q() ` + "`Q`" + `

Q {
	test "t" {
		assert ` + "`Q`" + ` { (x > 0) } // trailing
	}
}
`,
		},
		{
			name: "block assert multi-line with trailing",
			input: `fn Q() ` + "`Q`" + `

Q {
	test "t" {
		assert ` + "`Q`" + ` {
			(x > 0)
			(y < 10)
		} // trailing after multi-line
	}
}
`,
		},
		{
			name: "assert with query and trailing",
			input: `fn Q() ` + "`Q`" + `

fn Check() ` + "`CHECK`" + `

Q {
	test "t" {
		assert Check() { (x > 0) } // trailing
	}
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suite, err := scaf.Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			output := scaf.Format(suite)

			// The comment "// trailing" must appear in output
			if !containsString(output, "// trailing") {
				t.Errorf("Trailing comment not preserved.\n\nInput:\n%s\n\nOutput:\n%s", tt.input, output)
			}
		})
	}
}
