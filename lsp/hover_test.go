package lsp_test

import (
	"context"
	"strings"
	"testing"

	"github.com/rlch/scaf/analysis"
	"go.lsp.dev/protocol"
)

func TestHover_Expression_ShowsTypeInfo(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with an expression value
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `fn GetUser() ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	test "t" {
		$id: (2 * 4)
	}
}
`,
		},
	})

	// Hover on the expression (2 * 4)
	// Line 4: "\t\t$id: (2 * 4)"
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 4, Character: 10}, // Inside the expression
		},
	})
	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected hover result")
	}

	content := result.Contents.Value
	t.Logf("Hover content:\n%s", content)

	// Should show expression content
	if !strings.Contains(content, "(expression)") {
		t.Errorf("Expected expression marker in hover, got: %s", content)
	}

	// Should show the expression value
	if !strings.Contains(content, "2 * 4") {
		t.Errorf("Expected expression value in hover, got: %s", content)
	}

	// Should show type info
	if !strings.Contains(content, "evaluated at runtime") {
		t.Errorf("Expected runtime evaluation info in hover, got: %s", content)
	}

	// Should mention expr-lang
	if !strings.Contains(content, "expr-lang") {
		t.Errorf("Expected expr-lang mention in hover, got: %s", content)
	}
}

func TestHover_WhereClause_ShowsBoolType(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with a where clause
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `fn GetUser() ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	test "t" {
		$id: 5 where (id > 0)
	}
}
`,
		},
	})

	// Hover on the where clause
	// Line 4: "\t\t$id: 5 where (id > 0)"
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 4, Character: 18}, // Inside where clause
		},
	})
	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected hover result")
	}

	content := result.Contents.Value
	t.Logf("Hover content:\n%s", content)

	// Should show constraint
	if !strings.Contains(content, "(constraint)") {
		t.Errorf("Expected constraint marker in hover, got: %s", content)
	}

	// Should show bool type
	if !strings.Contains(content, "→ bool") {
		t.Errorf("Expected bool type indicator in hover, got: %s", content)
	}
}

func TestHover_AssertBlock_ShowsSummary(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with an assert block with multiple conditions
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `fn GetUser() ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	test "t" {
		assert {
			(u.age > 0)
			(u.name != "")
			(u.active == true)
		}
	}
}
`,
		},
	})

	// Hover on the assert keyword
	// Line 4: "\t\tassert {"
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 4, Character: 3}, // On "assert"
		},
	})
	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected hover result")
	}

	content := result.Contents.Value
	t.Logf("Hover content:\n%s", content)

	// Should show assertion block with count
	if !strings.Contains(content, "Assertion block") {
		t.Errorf("Expected assertion block marker in hover, got: %s", content)
	}

	// Should show condition count
	if !strings.Contains(content, "3 condition") {
		t.Errorf("Expected '3 condition(s)' in hover, got: %s", content)
	}

	// Should show some conditions with bool type
	if !strings.Contains(content, "→ bool") {
		t.Errorf("Expected bool type indicator for conditions, got: %s", content)
	}
}

func TestHover_AssertShorthand_ShowsSummary(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with a shorthand assert
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `fn GetUser() ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	test "t" {
		assert (u.age >= 18)
	}
}
`,
		},
	})

	// Hover on the assert keyword for shorthand
	// Line 4: "\t\tassert (u.age >= 18)"
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 4, Character: 3}, // On "assert"
		},
	})
	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected hover result")
	}

	content := result.Contents.Value
	t.Logf("Hover content:\n%s", content)

	// Should show shorthand indicator
	if !strings.Contains(content, "shorthand") {
		t.Errorf("Expected shorthand indicator in hover, got: %s", content)
	}

	// Should show condition with bool type
	if !strings.Contains(content, "→ bool") {
		t.Errorf("Expected bool type indicator for condition, got: %s", content)
	}
}

func TestHover_ExprIdentifier_VariableWithType(t *testing.T) {
	t.Parallel()

	server, _ := newTestServerWithDebug(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with an assert expression
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `fn GetUser() ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	test "t" {
		assert (u.age > 0)
	}
}
`,
		},
	})

	// Hover on the variable 'u' in the assertion
	// Line 4: "\t\tassert (u.age > 0)"
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 4, Character: 10}, // On "u"
		},
	})
	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected hover result")
	}

	content := result.Contents.Value
	t.Logf("Hover content:\n%s", content)

	// Should show variable marker
	if !strings.Contains(content, "(variable)") {
		t.Errorf("Expected variable marker in hover, got: %s", content)
	}

	// Should show variable name
	if !strings.Contains(content, "`u`") {
		t.Errorf("Expected variable name 'u' in hover, got: %s", content)
	}

	// Should show source hint
	if !strings.Contains(content, "from RETURN clause") {
		t.Errorf("Expected source hint in hover, got: %s", content)
	}
}

func TestHover_ExprIdentifier_PropertyWithType(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with an assert expression
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `fn GetUser() ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	test "t" {
		assert (u.age > 0)
	}
}
`,
		},
	})

	// Hover on the property 'age' in the assertion
	// Line 4: "\t\tassert (u.age > 0)"
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 4, Character: 13}, // On "age"
		},
	})
	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected hover result")
	}

	content := result.Contents.Value
	t.Logf("Hover content:\n%s", content)

	// Should show property marker
	if !strings.Contains(content, "(property)") {
		t.Errorf("Expected property marker in hover, got: %s", content)
	}

	// Should show full path
	if !strings.Contains(content, "`u.age`") {
		t.Errorf("Expected full path 'u.age' in hover, got: %s", content)
	}

	// Should show type (even if just (any))
	if !strings.Contains(content, ":") {
		t.Errorf("Expected type in hover, got: %s", content)
	}
}

func TestHover_FriendlyTypeNames(t *testing.T) {
	t.Parallel()

	// Test the friendlyTypeName function indirectly through hover
	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file - the types will come from schema when available
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `fn GetUser() ` + "`MATCH (u:User) RETURN u.name`" + `

GetUser {
	test "t" {
		u.name: "Alice"
	}
}
`,
		},
	})

	// Hover on the return field u.name
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 4, Character: 4}, // On "name"
		},
	})
	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected hover result")
	}

	content := result.Contents.Value
	t.Logf("Hover content:\n%s", content)

	// Should show property
	if !strings.Contains(content, "(property)") {
		t.Errorf("Expected property marker in hover, got: %s", content)
	}

	// Should have some type info (even if just (any))
	if !strings.Contains(content, ":") {
		t.Errorf("Expected type separator in hover, got: %s", content)
	}
}

func TestHover_DottedKey_ShowsTypeFromRETURN(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with a return field
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `fn GetUser() ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	test "t" {
		u.name: "Alice"
	}
}
`,
		},
	})

	// Hover on 'u' in 'u.name'
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 4, Character: 2}, // On "u"
		},
	})
	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected hover result")
	}

	content := result.Contents.Value
	t.Logf("Hover on 'u':\n%s", content)

	// Should show variable marker
	if !strings.Contains(content, "(variable)") {
		t.Errorf("Expected variable marker in hover, got: %s", content)
	}

	// Should show source hint
	if !strings.Contains(content, "from RETURN clause") {
		t.Errorf("Expected RETURN clause hint in hover, got: %s", content)
	}
}

// ============================================================================
// Hover edge case tests
// ============================================================================

func TestHover_Operator_NoContent(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with an assert expression containing operators
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `fn GetUser() ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	test "t" {
		assert (u.age > 0)
	}
}
`,
		},
	})

	// Hover on the '>' operator
	// Line 4: "\t\tassert (u.age > 0)"
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 4, Character: 16}, // On ">"
		},
	})
	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	// Hovering on operators should not crash
	// It may return nil or the parent node (assertion block)
	// The key is that it doesn't panic
	if result != nil {
		t.Logf("Hover on operator returned: %s", result.Contents.Value)
	} else {
		t.Log("Hover on operator returned nil (expected)")
	}
}

func TestHover_NumberLiteral_NoContent(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with a number literal in expression
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `fn GetUser() ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	test "t" {
		assert (u.age > 18)
	}
}
`,
		},
	})

	// Hover on the number '18'
	// Line 4: "\t\tassert (u.age > 18)"
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 4, Character: 18}, // On "18"
		},
	})
	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	// Hovering on number literals should not crash
	if result != nil {
		t.Logf("Hover on number returned: %s", result.Contents.Value)
	} else {
		t.Log("Hover on number returned nil (expected)")
	}
}

func TestHover_StringLiteral_NoContent(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with a string literal in expression
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `fn GetUser() ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	test "t" {
		assert (u.name == "Alice")
	}
}
`,
		},
	})

	// Hover on the string "Alice"
	// Line 4: "\t\tassert (u.name == \"Alice\")"
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 4, Character: 21}, // On "Alice"
		},
	})
	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	// Hovering on string literals should not crash
	if result != nil {
		t.Logf("Hover on string returned: %s", result.Contents.Value)
	} else {
		t.Log("Hover on string returned nil (expected)")
	}
}

func TestHover_FunctionCallInExpr(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with a function call in expression
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `fn GetUser() ` + "`MATCH (u:User) RETURN u, collect(u.name) as names`" + `

GetUser {
	test "t" {
		assert (len(names) > 0)
	}
}
`,
		},
	})

	// Hover on the function name "len"
	// Line 4: "\t\tassert (len(names) > 0)"
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 4, Character: 11}, // On "len"
		},
	})
	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	// Hovering on function calls should not crash
	// Currently returns nil since we don't have function docs
	if result != nil {
		t.Logf("Hover on function call returned: %s", result.Contents.Value)
	} else {
		t.Log("Hover on function call returned nil (expected for now)")
	}
}

func TestHover_NestedExprIdentifier(t *testing.T) {
	t.Parallel()

	server, _ := newTestServerWithDebug(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a file with nested expression
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `fn GetUser() ` + "`MATCH (u:User) RETURN u.age, u.score`" + `

GetUser {
	test "t" {
		assert ((u.age > 0) && (u.score > 0))
	}
}
`,
		},
	})

	// Hover on 'score' in the nested expression
	// Line 4: "\t\tassert ((u.age > 0) && (u.score > 0))"
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 4, Character: 29}, // On "score"
		},
	})
	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected hover result for nested expression identifier")
	}

	content := result.Contents.Value
	t.Logf("Hover on nested expr identifier:\n%s", content)

	// Should show property marker
	if !strings.Contains(content, "(property)") {
		t.Errorf("Expected property marker in hover, got: %s", content)
	}

	// Should show the full path
	if !strings.Contains(content, "u.score") {
		t.Errorf("Expected 'u.score' in hover, got: %s", content)
	}
}

// TestHover_AssertQueryScope tests that hover on identifiers inside an assert with
// its own query uses that query's scope, not the parent scope.
func TestHover_AssertQueryScope(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open file with two queries - parent scope is GetUser, assert uses CreatePost
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `fn GetUser() ` + "`MATCH (u:User) RETURN u.name, u.age`" + `
fn CreatePost() ` + "`CREATE (p:Post {title: $title}) RETURN p.title, p.views`" + `

GetUser {
	test "with assert query" {
		$id: 1
		assert CreatePost($title: "Test") {
			(p.title == "Test")
			(p.views == 0)
		}
	}
}
`,
		},
	})

	// Hover on 'p' in the assertion (from CreatePost, not GetUser)
	// Line 7: "\t\t\t(p.title == "Test")"
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 7, Character: 4}, // On "p"
		},
	})
	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected hover result for assert query identifier")
	}

	content := result.Contents.Value
	t.Logf("Hover on assert query identifier:\n%s", content)

	// Should show variable marker
	if !strings.Contains(content, "(variable)") {
		t.Errorf("Expected variable marker in hover, got: %s", content)
	}

	// Should show 'p' identifier
	if !strings.Contains(content, "`p`") {
		t.Errorf("Expected 'p' in hover, got: %s", content)
	}
}

// TestHover_AssertQueryScope_Property tests hover on property access in assert query.
func TestHover_AssertQueryScope_Property(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open file with two queries
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `fn GetUser() ` + "`MATCH (u:User) RETURN u.name, u.age`" + `
fn CreatePost() ` + "`CREATE (p:Post {title: $title}) RETURN p.title, p.views`" + `

GetUser {
	test "with assert query" {
		$id: 1
		assert CreatePost($title: "Test") {
			(p.title == "Test")
		}
	}
}
`,
		},
	})

	// Hover on 'title' property in the assertion
	// Line 7: "\t\t\t(p.title == "Test")"
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 7, Character: 6}, // On "title"
		},
	})
	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected hover result for property in assert query")
	}

	content := result.Contents.Value
	t.Logf("Hover on assert query property:\n%s", content)

	// Should show property marker
	if !strings.Contains(content, "(property)") {
		t.Errorf("Expected property marker in hover, got: %s", content)
	}

	// Should show full path
	if !strings.Contains(content, "p.title") {
		t.Errorf("Expected 'p.title' in hover, got: %s", content)
	}
}

// TestHover_AssertQueryScope_WithSchema tests hover shows types from schema.
func TestHover_AssertQueryScope_WithSchema(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Set up schema with Post model
	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"Post": {
				Name: "Post",
				Fields: []*analysis.Field{
					{Name: "title", Type: analysis.TypeString},
					{Name: "views", Type: analysis.TypeInt},
					{Name: "authorId", Type: analysis.TypeInt},
				},
			},
			"User": {
				Name: "User",
				Fields: []*analysis.Field{
					{Name: "name", Type: analysis.TypeString},
					{Name: "age", Type: analysis.TypeInt},
				},
			},
		},
	}
	server.SetSchemaForTesting(schema)

	// Open file with two queries
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text: `fn GetUser() ` + "`MATCH (u:User) RETURN u.name, u.age`" + `
fn CreatePost() ` + "`CREATE (p:Post {title: $title}) RETURN p.title, p.views, p.authorId`" + `

GetUser {
	test "with assert query" {
		$id: 1
		assert CreatePost($title: "Test") {
			(p.title == "Test")
			(p.authorId == 1)
		}
	}
}
`,
		},
	})

	// Hover on 'authorId' property in the assertion
	// Line 8: "\t\t\t(p.authorId == 1)"
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 8, Character: 6}, // On "authorId"
		},
	})
	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected hover result for property in assert query with schema")
	}

	content := result.Contents.Value
	t.Logf("Hover on assert query property with schema:\n%s", content)

	// Should show property marker
	if !strings.Contains(content, "(property)") {
		t.Errorf("Expected property marker in hover, got: %s", content)
	}

	// Should show full path
	if !strings.Contains(content, "p.authorId") {
		t.Errorf("Expected 'p.authorId' in hover, got: %s", content)
	}

	// Should show int type (from schema)
	if !strings.Contains(content, "int") {
		t.Errorf("Expected 'int' type in hover (from schema), got: %s", content)
	}
}
