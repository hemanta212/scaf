package lsp_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"go.lsp.dev/protocol"
)

// TestHover_AllFeaturesFile tests hover on the actual all_features.cypher.scaf file
// to ensure hover works on all elements in a complex real-world file.
func TestHover_AllFeaturesFile(t *testing.T) {
	// Read the actual file
	content, err := os.ReadFile("../example/basic/all_features.cypher.scaf")
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	err = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///all_features.cypher.scaf",
			Version: 1,
			Text:    string(content),
		},
	})
	if err != nil {
		t.Fatalf("DidOpen() error: %v", err)
	}

	// Test positions based on the actual file content
	// Lines are 0-indexed (LSP convention)
	// NOTE: Tabs count as 1 character each
	testCases := []struct {
		name     string
		line     uint32
		char     uint32
		expected []string // expected substrings in hover content
	}{
		// Import - line 5 (0-indexed): "import fixtures "./shared/fixtures""
		{"import_fixtures", 5, 10, []string{"Import", "fixtures"}},

		// Query definition - line 10: "fn GetUser(userId) `"
		{"fn_GetUser", 10, 5, []string{"Query", "GetUser"}},

		// Query scope - line 41: "GetUser {" - hover anywhere shows scope
		{"scope_GetUser", 41, 3, []string{"Query", "GetUser"}},

		// Group - line 50: "\tgroup \"basic lookups\" {" - char 1 is the tab, char 2+ is group
		{"group_basic_lookups", 50, 2, []string{"Group", "basic lookups"}},

		// Test - line 51: "\t\ttest \"finds Alice by id\" {" - char 0-1 are tabs, char 2+ is test
		{"test_finds_alice", 51, 3, []string{"Test", "finds Alice"}},

		// Statement $userId - line 52: "\t\t\t$userId: 1" - tabs at 0-2, $ at char 3
		{"param_userId", 52, 3, []string{"$userId"}},

		// Statement u.name - line 54: "\t\t\tu.name: \"Alice\"" - tabs at 0-2, u at char 3
		{"output_u_name", 54, 3, []string{"u"}},

		// Assert block - line 87: "\t\t\tassert {" - tabs at 0-2, assert at char 3+
		{"assert_block", 87, 4, []string{"Assert"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := server.Hover(ctx, &protocol.HoverParams{
				TextDocumentPositionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{URI: "file:///all_features.cypher.scaf"},
					Position:     protocol.Position{Line: tc.line, Character: tc.char},
				},
			})
			if err != nil {
				t.Fatalf("Hover() error: %v", err)
			}

			if result == nil {
				// Show context
				lines := strings.Split(string(content), "\n")
				if int(tc.line) < len(lines) {
					t.Logf("Line %d: %q", tc.line, lines[tc.line])
					t.Logf("Position: char %d", tc.char)
				}
				t.Fatalf("Expected hover result, got nil")
			}

			hoverContent := result.Contents.Value
			t.Logf("Hover content:\n%s", hoverContent)

			for _, exp := range tc.expected {
				if !strings.Contains(strings.ToLower(hoverContent), strings.ToLower(exp)) {
					t.Errorf("Expected %q in hover content", exp)
				}
			}
		})
	}
}

// TestHover_DebugPositions helps debug hover position issues by testing every position on a line.
func TestHover_DebugPositions(t *testing.T) {
	t.Skip("Debug test - enable manually when investigating position issues")

	content, err := os.ReadFile("../example/basic/all_features.cypher.scaf")
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    string(content),
		},
	})

	lines := strings.Split(string(content), "\n")

	// Test multiple lines
	testLines := []uint32{41, 50, 51, 52, 54, 87}

	for _, lineNum := range testLines {
		line := lines[lineNum]
		t.Logf("\n=== Testing line %d: %q (len=%d) ===", lineNum, line, len(line))

		for col := uint32(0); col <= uint32(len(line)); col++ {
			result, _ := server.Hover(ctx, &protocol.HoverParams{
				TextDocumentPositionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
					Position:     protocol.Position{Line: lineNum, Character: col},
				},
			})

			if result != nil {
				first50 := result.Contents.Value
				if len(first50) > 50 {
					first50 = first50[:50] + "..."
				}
				first50 = strings.ReplaceAll(first50, "\n", "\\n")
				t.Logf("  Col %2d: %s", col, first50)
			} else {
				t.Logf("  Col %2d: nil", col)
			}
		}
	}
}
