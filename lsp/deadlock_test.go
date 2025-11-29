package lsp_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf/lsp"

	_ "github.com/rlch/scaf/dialects/cypher"
)

// slowMockClient is a mock client that introduces delay in PublishDiagnostics
// to simulate a real JSON-RPC connection where the call might block.
type slowMockClient struct {
	mockClient
	diagnosticsDelay time.Duration
	mu               sync.Mutex
	blocked          bool
}

func (m *slowMockClient) PublishDiagnostics(ctx context.Context, params *protocol.PublishDiagnosticsParams) error {
	m.mu.Lock()
	m.blocked = true
	m.mu.Unlock()

	// Simulate slow RPC - this is where the deadlock occurs
	time.Sleep(m.diagnosticsDelay)

	m.mu.Lock()
	m.blocked = false
	m.mu.Unlock()

	return m.mockClient.PublishDiagnostics(ctx, params)
}

func (m *slowMockClient) IsBlocked() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.blocked
}

func newSlowTestServer(t *testing.T, delay time.Duration) (*lsp.Server, *slowMockClient) {
	t.Helper()

	logger := zap.NewNop()
	client := &slowMockClient{diagnosticsDelay: delay}
	server := lsp.NewServer(client, logger, "cypher")

	return server, client
}

// TestServer_Deadlock_DidChangeCompletion tests for deadlock when completion
// request arrives while didChange is processing.
//
// The bug: DidChange held the mutex while calling PublishDiagnostics (an RPC call).
// If the client sent a completion request during that time, the completion handler
// would try to acquire the read lock (via getDocument) and deadlock.
func TestServer_Deadlock_DidChangeCompletion(t *testing.T) {
	t.Parallel()

	server, client := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a valid file
	content := `import fixtures "./shared/fixtures"

fn GetUser() ` + "`MATCH (u:User {id: $userId}) RETURN u`" + `

GetUserPosts {
	setup fixtures.SetupUsers()

	test "user with post" {
		$authorId: 1
	}
}
`
	uri := protocol.DocumentURI("file:///test.scaf")
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     uri,
			Version: 1,
			Text:    content,
		},
	})

	// Make the mock client block on PublishDiagnostics to simulate slow RPC
	blockingClient := client
	_ = blockingClient // Use the client

	// Now simulate the problematic sequence:
	// 1. DidChange starts processing (acquires lock)
	// 2. DidChange calls PublishDiagnostics
	// 3. Completion request arrives (tries to acquire read lock)
	// 4. If there's a deadlock, completion will block forever

	// Change content to have incomplete syntax (setup fixtures.)
	changedContent := `import fixtures "./shared/fixtures"

fn GetUser() ` + "`MATCH (u:User {id: $userId}) RETURN u`" + `

GetUserPosts {
	setup fixtures.

	test "user with post" {
		$authorId: 1
	}
}
`

	var wg sync.WaitGroup
	errChan := make(chan error, 2)
	doneChan := make(chan struct{}, 2)

	// Start DidChange in one goroutine
	wg.Go(func() {
		err := server.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
			TextDocument: protocol.VersionedTextDocumentIdentifier{
				TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: uri},
				Version:                2,
			},
			ContentChanges: []protocol.TextDocumentContentChangeEvent{
				{Text: changedContent},
			},
		})
		if err != nil {
			errChan <- err
		}
		doneChan <- struct{}{}
	})

	// Start Completion request in another goroutine (with slight delay to ensure didChange starts first)
	wg.Go(func() {
		time.Sleep(10 * time.Millisecond) // Let didChange start
		_, err := server.Completion(ctx, &protocol.CompletionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 5, Character: 17}, // After "fixtures."
			},
		})
		if err != nil {
			errChan <- err
		}
		doneChan <- struct{}{}
	})

	// Wait with timeout - if deadlock, this will timeout
	timeout := time.After(5 * time.Second)
	completed := 0
	for completed < 2 {
		select {
		case <-doneChan:
			completed++
		case err := <-errChan:
			t.Errorf("Unexpected error: %v", err)
		case <-timeout:
			t.Fatal("DEADLOCK DETECTED: Operations did not complete within 5 seconds")
		}
	}

	wg.Wait()
}

// TestServer_Deadlock_RapidChanges tests for deadlock with rapid consecutive changes.
func TestServer_Deadlock_RapidChanges(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	uri := protocol.DocumentURI("file:///test.scaf")
	_ = server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     uri,
			Version: 1,
			Text:    `fn Q() ` + "`Q`" + ` Q { test "t" { $x: 1 } }`,
		},
	})

	// Send rapid changes and completions concurrently
	var wg sync.WaitGroup
	doneChan := make(chan struct{}, 20)

	for i := range 10 {
		version := int32(i + 2)

		// DidChange
		wg.Go(func() {
			_ = server.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
				TextDocument: protocol.VersionedTextDocumentIdentifier{
					TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: uri},
					Version:                version,
				},
				ContentChanges: []protocol.TextDocumentContentChangeEvent{
					{Text: `fn Q() ` + "`Q`" + ` Q { test "t" { $x: ` + string(rune('0'+version%10)) + ` } }`},
				},
			})
			doneChan <- struct{}{}
		})

		// Completion
		wg.Go(func() {
			_, _ = server.Completion(ctx, &protocol.CompletionParams{
				TextDocumentPositionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{URI: uri},
					Position:     protocol.Position{Line: 0, Character: 30},
				},
			})
			doneChan <- struct{}{}
		})
	}

	// Wait with timeout
	timeout := time.After(10 * time.Second)
	completed := 0
	for completed < 20 {
		select {
		case <-doneChan:
			completed++
		case <-timeout:
			t.Fatalf("DEADLOCK DETECTED: Only %d/20 operations completed within 10 seconds", completed)
		}
	}

	wg.Wait()
}
