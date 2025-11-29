package lsp_test

import (
	"context"
	"testing"
	"time"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf/lsp"

	_ "github.com/rlch/scaf/dialects/cypher"
)

// TestServer_FreezeRepro reproduces the freeze when typing "setup fixtures."
// This is the minimal reproduction case from the user.
func TestServer_FreezeRepro(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	client := &mockClient{}
	server := lsp.NewServer(client, logger, "cypher")
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{
		RootURI: "file:///test",
	})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Minimal reproduction case
	content := `import fixtures "./shared/fixtures"

setup fixtures.
`
	uri := protocol.DocumentURI("file:///test.scaf")

	// Test DidOpen with timeout
	t.Run("DidOpen", func(t *testing.T) {
		done := make(chan struct{})
		go func() {
			_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
				TextDocument: protocol.TextDocumentItem{
					URI:     uri,
					Version: 1,
					Text:    content,
				},
			})
			close(done)
		}()

		select {
		case <-done:
			t.Log("DidOpen completed")
		case <-time.After(5 * time.Second):
			t.Fatal("DidOpen FREEZE - timed out after 5 seconds")
		}
	})

	// Test Completion with timeout
	t.Run("Completion after fixtures.", func(t *testing.T) {
		done := make(chan struct{})
		var result *protocol.CompletionList
		var err error

		go func() {
			result, err = server.Completion(ctx, &protocol.CompletionParams{
				TextDocumentPositionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{URI: uri},
					Position:     protocol.Position{Line: 2, Character: 16}, // After "fixtures."
				},
				Context: &protocol.CompletionContext{
					TriggerKind:      protocol.CompletionTriggerKindTriggerCharacter,
					TriggerCharacter: ".",
				},
			})
			close(done)
		}()

		select {
		case <-done:
			if err != nil {
				t.Errorf("Completion error: %v", err)
			}
			t.Logf("Completion completed, items: %d", len(result.Items))
			for _, item := range result.Items {
				t.Logf("  - %s (%s)", item.Label, item.Kind)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Completion FREEZE - timed out after 5 seconds")
		}
	})

	// Test SignatureHelp with timeout
	t.Run("SignatureHelp after fixtures.", func(t *testing.T) {
		done := make(chan struct{})
		var result *protocol.SignatureHelp
		var err error

		go func() {
			result, err = server.SignatureHelp(ctx, &protocol.SignatureHelpParams{
				TextDocumentPositionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{URI: uri},
					Position:     protocol.Position{Line: 2, Character: 16}, // After "fixtures."
				},
			})
			close(done)
		}()

		select {
		case <-done:
			if err != nil {
				t.Errorf("SignatureHelp error: %v", err)
			}
			if result != nil {
				t.Logf("SignatureHelp completed, signatures: %d", len(result.Signatures))
			} else {
				t.Log("SignatureHelp returned nil (expected)")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("SignatureHelp FREEZE - timed out after 5 seconds")
		}
	})

	// Test Hover with timeout
	t.Run("Hover on fixtures.", func(t *testing.T) {
		done := make(chan struct{})
		var result *protocol.Hover
		var err error

		go func() {
			result, err = server.Hover(ctx, &protocol.HoverParams{
				TextDocumentPositionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{URI: uri},
					Position:     protocol.Position{Line: 2, Character: 10}, // On "fixtures"
				},
			})
			close(done)
		}()

		select {
		case <-done:
			if err != nil {
				t.Errorf("Hover error: %v", err)
			}
			if result != nil {
				t.Logf("Hover completed: %v", result.Contents)
			} else {
				t.Log("Hover returned nil (expected)")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Hover FREEZE - timed out after 5 seconds")
		}
	})

	// Test DidChange with timeout (simulates typing more after "fixtures.")
	t.Run("DidChange to add character", func(t *testing.T) {
		done := make(chan struct{})

		go func() {
			_ = server.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
				TextDocument: protocol.VersionedTextDocumentIdentifier{
					TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: uri},
					Version:                2,
				},
				ContentChanges: []protocol.TextDocumentContentChangeEvent{
					{Text: `import fixtures "./shared/fixtures"

setup fixtures.S
`},
				},
			})
			close(done)
		}()

		select {
		case <-done:
			t.Log("DidChange completed")
		case <-time.After(5 * time.Second):
			t.Fatal("DidChange FREEZE - timed out after 5 seconds")
		}
	})
}

// TestServer_FreezeRepro_WithRealFileLoader tests with actual file loading
// to see if cross-file resolution causes the freeze.
func TestServer_FreezeRepro_WithRealFileLoader(t *testing.T) {
	t.Parallel()

	logger, _ := zap.NewDevelopment()
	client := &mockClient{}
	server := lsp.NewServer(client, logger, "cypher")
	ctx := context.Background()

	// Use a real workspace path
	_, _ = server.Initialize(ctx, &protocol.InitializeParams{
		RootURI: "file:///Users/rjm/Coding/Personal/scaf/example/basic",
	})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Minimal reproduction case
	content := `import fixtures "./shared/fixtures"

setup fixtures.
`
	uri := protocol.DocumentURI("file:///Users/rjm/Coding/Personal/scaf/example/basic/test.scaf")

	// Open document
	done := make(chan struct{})
	go func() {
		_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{
				URI:     uri,
				Version: 1,
				Text:    content,
			},
		})
		close(done)
	}()

	select {
	case <-done:
		t.Log("DidOpen completed")
	case <-time.After(10 * time.Second):
		t.Fatal("DidOpen FREEZE - timed out after 10 seconds")
	}

	// Now test completion - this will try to load the actual fixtures file
	t.Run("Completion with real file loading", func(t *testing.T) {
		done := make(chan struct{})
		var result *protocol.CompletionList
		var err error

		go func() {
			result, err = server.Completion(ctx, &protocol.CompletionParams{
				TextDocumentPositionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{URI: uri},
					Position:     protocol.Position{Line: 2, Character: 16}, // After "fixtures."
				},
				Context: &protocol.CompletionContext{
					TriggerKind:      protocol.CompletionTriggerKindTriggerCharacter,
					TriggerCharacter: ".",
				},
			})
			close(done)
		}()

		select {
		case <-done:
			if err != nil {
				t.Errorf("Completion error: %v", err)
			}
			if result != nil {
				t.Logf("Completion completed, items: %d", len(result.Items))
				for _, item := range result.Items {
					t.Logf("  - %s (%s)", item.Label, item.Kind)
				}
			}
		case <-time.After(10 * time.Second):
			t.Fatal("Completion FREEZE - timed out after 10 seconds")
		}
	})
}
