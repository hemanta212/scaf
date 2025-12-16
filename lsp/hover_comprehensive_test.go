package lsp_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/rlch/scaf/analysis"
	"go.lsp.dev/protocol"
)

// TestHover_AllSymbols is a comprehensive test that verifies hover works for all symbol types.
func TestHover_AllSymbols(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Set up schema for type inference
	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"User": {
				Name: "User",
				Fields: []*analysis.Field{
					{Name: "id", Type: analysis.TypeInt},
					{Name: "name", Type: analysis.TypeString},
					{Name: "age", Type: analysis.TypeInt},
					{Name: "verified", Type: analysis.TypeBool},
				},
			},
			"Post": {
				Name: "Post",
				Fields: []*analysis.Field{
					{Name: "title", Type: analysis.TypeString},
					{Name: "authorId", Type: analysis.TypeInt},
					{Name: "views", Type: analysis.TypeInt},
				},
			},
		},
	}
	server.SetSchemaForTesting(schema)

	// Comprehensive test file
	content := `import fixtures "./shared/fixtures"

fn GetUser(userId: int) ` + "`" + `
MATCH (u:User {id: $userId})
RETURN u.name, u.age, u.verified
` + "`" + `

fn CreatePost(title: string, authorId: int) ` + "`" + `
CREATE (p:Post {title: $title, authorId: $authorId, views: 0})
RETURN p.title, p.authorId, p.views
` + "`" + `

GetUser {
	test "finds user" {
		$userId: 1
		u.name: "Alice"
		u.age: 30
		
		assert (u.age > 18)
	}
	
	group "with assertions" {
		test "assert with query" {
			$userId: 1
			
			assert CreatePost($title: "Test", $authorId: 1) {
				(p.title == "Test")
				(p.authorId == 1)
			}
		}
	}
}
`

	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    content,
		},
	})

	// Test cases: line, char, description, expected substrings
	testCases := []struct {
		name     string
		line     uint32
		char     uint32
		expected []string
	}{
		// Import
		{"import_alias", 0, 8, []string{"import", "fixtures"}},

		// Query definition
		{"query_name", 2, 4, []string{"Function", "GetUser"}},

		// Scope - shows as Query (the referenced query info)
		{"scope_name", 12, 2, []string{"Query", "GetUser"}},

		// Test
		{"test_name", 13, 8, []string{"Test", "finds user"}},

		// Parameter in test
		{"param_key", 14, 4, []string{"parameter", "$userId"}},

		// Return field key (dotted)
		{"return_field_var", 15, 3, []string{"u"}},
		{"return_field_prop", 15, 5, []string{"name"}},

		// Literal value
		{"literal_value", 15, 12, []string{"literal", "Alice"}},

		// Assert expression - variable
		{"assert_expr_var", 18, 11, []string{"variable", "u"}},
		
		// Assert expression - property
		{"assert_expr_prop", 18, 13, []string{"property", "u.age"}},

		// Group
		{"group_name", 21, 8, []string{"Group", "assertions"}},

		// Assert with query - should use CreatePost's scope
		{"assert_query_var", 26, 6, []string{"variable", "p"}},
		{"assert_query_prop", 26, 8, []string{"property", "p.title"}},
		// Line 27: "\t\t\t\t(p.authorId == 1)" - authorId starts at col 7
		{"assert_query_prop2", 27, 7, []string{"property", "p.authorId"}},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			result, err := server.Hover(ctx, &protocol.HoverParams{
				TextDocumentPositionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
					Position:     protocol.Position{Line: tc.line, Character: tc.char},
				},
			})

			if err != nil {
				t.Fatalf("Hover() error: %v", err)
			}

			if result == nil {
				t.Fatalf("Expected hover result, got nil")
			}

			content := result.Contents.Value
			t.Logf("Hover content:\n%s", content)

			for _, exp := range tc.expected {
				if !strings.Contains(strings.ToLower(content), strings.ToLower(exp)) {
					t.Errorf("Expected %q in hover content, got: %s", exp, content)
				}
			}
		})
	}
}

// TestHover_AssertQueryTypes tests that types are correctly inferred from assert query scope.
func TestHover_AssertQueryTypes(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Set up schema
	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"User": {
				Name: "User",
				Fields: []*analysis.Field{
					{Name: "name", Type: analysis.TypeString},
					{Name: "age", Type: analysis.TypeInt},
				},
			},
			"Post": {
				Name: "Post",
				Fields: []*analysis.Field{
					{Name: "title", Type: analysis.TypeString},
					{Name: "views", Type: analysis.TypeInt},
				},
			},
		},
	}
	server.SetSchemaForTesting(schema)

	content := `fn GetUser() ` + "`MATCH (u:User) RETURN u.name, u.age`" + `
fn CreatePost() ` + "`CREATE (p:Post {title: $t}) RETURN p.title, p.views`" + `

GetUser {
	test "t" {
		assert CreatePost() {
			(p.views == 0)
		}
	}
}
`

	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    content,
		},
	})

	// Hover on p.views in assert - should show int type from Post schema
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 6, Character: 7}, // On "views"
		},
	})

	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected hover result")
	}

	content_str := result.Contents.Value
	t.Logf("Hover content:\n%s", content_str)

	// Should show int type (from schema)
	if !strings.Contains(content_str, "int") {
		t.Errorf("Expected 'int' type in hover (from schema), got: %s", content_str)
	}

	// Should show property marker
	if !strings.Contains(content_str, "property") {
		t.Errorf("Expected 'property' marker in hover, got: %s", content_str)
	}

	// Should show full path
	if !strings.Contains(content_str, "p.views") {
		t.Errorf("Expected 'p.views' in hover, got: %s", content_str)
	}
}

// TestHover_InlineAssertQuery tests hover for inline assert queries.
func TestHover_InlineAssertQuery(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Set up schema
	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"User": {
				Name: "User",
				Fields: []*analysis.Field{
					{Name: "name", Type: analysis.TypeString},
				},
			},
			"Comment": {
				Name: "Comment",
				Fields: []*analysis.Field{
					{Name: "text", Type: analysis.TypeString},
					{Name: "likes", Type: analysis.TypeInt},
				},
			},
		},
	}
	server.SetSchemaForTesting(schema)

	content := `fn GetUser() ` + "`MATCH (u:User) RETURN u.name`" + `

GetUser {
	test "t" {
		assert ` + "`MATCH (c:Comment) RETURN c.text, c.likes`" + ` {
			(c.likes > 0)
		}
	}
}
`

	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    content,
		},
	})

	// Hover on c.likes in inline assert
	result, err := server.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
			Position:     protocol.Position{Line: 5, Character: 7}, // On "likes"
		},
	})

	if err != nil {
		t.Fatalf("Hover() error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected hover result")
	}

	content_str := result.Contents.Value
	t.Logf("Hover content:\n%s", content_str)

	// Should show int type (from schema via inline query)
	if !strings.Contains(content_str, "int") {
		t.Errorf("Expected 'int' type from inline query schema, got: %s", content_str)
	}
}

// Print all lines with line numbers for debugging positions
func printWithLineNumbers(content string) string {
	lines := strings.Split(content, "\n")
	var sb strings.Builder
	for i, line := range lines {
		sb.WriteString(fmt.Sprintf("%2d: %s\n", i, line))
	}
	return sb.String()
}
