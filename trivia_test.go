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
