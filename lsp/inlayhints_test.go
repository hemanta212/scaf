package lsp_test

import (
	"context"
	"testing"

	"go.lsp.dev/protocol"

	"github.com/rlch/scaf/analysis"
	"github.com/rlch/scaf/lsp"
)

func TestServer_InlayHints_InferredParameterType(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	// Initialize the server
	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Set up a schema with Person model that has a name field of type string
	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"Person": {
				Name: "Person",
				Fields: []*analysis.Field{
					{Name: "name", Type: analysis.TypeString},
					{Name: "age", Type: analysis.TypeInt},
				},
			},
			"Movie": {
				Name: "Movie",
				Fields: []*analysis.Field{
					{Name: "title", Type: analysis.TypeString},
					{Name: "year", Type: analysis.TypeInt},
				},
			},
		},
	}
	server.SetSchemaForTesting(schema)

	// Open a document with a function that has an untyped parameter
	// The parameter 'name' should get an inlay hint ': string' because
	// it's used in WHERE p.name = $name where p:Person
	//
	// fn FindActors(name) `
	//   MATCH (p:Person)-[r:ACTED_IN]->(m:Movie)
	//   WHERE p.name = $name
	//   RETURN p.name, m.title, r.roles
	// `
	doc := `fn FindActors(name) ` + "`" + `
  MATCH (p:Person)-[r:ACTED_IN]->(m:Movie)
  WHERE p.name = $name
  RETURN p.name, m.title
` + "`"

	err := server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    doc,
		},
	})
	if err != nil {
		t.Fatalf("DidOpen() error: %v", err)
	}

	// Request inlay hints for the document
	hints, err := server.InlayHint(ctx, &lsp.InlayHintParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: 5, Character: 0},
		},
	})
	if err != nil {
		t.Fatalf("InlayHint() error: %v", err)
	}

	// Should have one hint for the 'name' parameter
	if len(hints) != 1 {
		t.Fatalf("Expected 1 inlay hint, got %d", len(hints))
	}

	hint := hints[0]

	// Check the hint label
	if hint.Label != ": string" {
		t.Errorf("Expected hint label ': string', got %q", hint.Label)
	}

	// Check the hint kind
	if hint.Kind != lsp.InlayHintKindType {
		t.Errorf("Expected hint kind Type, got %d", hint.Kind)
	}

	// Check the hint position (should be after 'name' in the function signature)
	// fn FindActors(name) `
	//    ^         ^
	//    0         14 (position after 'name')
	// Line 0, Character 18 (after "name" in "fn FindActors(name)")
	if hint.Position.Line != 0 {
		t.Errorf("Expected hint on line 0, got line %d", hint.Position.Line)
	}
	// "fn FindActors(name)" - the 'n' of 'name' is at character 14, length is 4
	// So end position should be 14 + 4 = 18
	if hint.Position.Character != 18 {
		t.Errorf("Expected hint at character 18, got character %d", hint.Position.Character)
	}
}

func TestServer_InlayHints_NoHintForExplicitType(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	// Initialize the server
	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Set up schema
	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"Person": {
				Name: "Person",
				Fields: []*analysis.Field{
					{Name: "name", Type: analysis.TypeString},
				},
			},
		},
	}
	server.SetSchemaForTesting(schema)

	// Open a document with a function that has an EXPLICIT type annotation
	// No inlay hint should be shown
	doc := `fn FindPerson(name: string) ` + "`" + `
  MATCH (p:Person {name: $name})
  RETURN p
` + "`"

	err := server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    doc,
		},
	})
	if err != nil {
		t.Fatalf("DidOpen() error: %v", err)
	}

	// Request inlay hints
	hints, err := server.InlayHint(ctx, &lsp.InlayHintParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: 4, Character: 0},
		},
	})
	if err != nil {
		t.Fatalf("InlayHint() error: %v", err)
	}

	// Should have NO hints since the parameter already has an explicit type
	if len(hints) != 0 {
		t.Fatalf("Expected 0 inlay hints (explicit type provided), got %d", len(hints))
	}
}

func TestServer_InlayHints_MultipleParameters(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	// Initialize the server
	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Set up schema
	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"Person": {
				Name: "Person",
				Fields: []*analysis.Field{
					{Name: "name", Type: analysis.TypeString},
					{Name: "age", Type: analysis.TypeInt},
				},
			},
		},
	}
	server.SetSchemaForTesting(schema)

	// Open a document with multiple parameters, some typed and some untyped
	// name: no type -> should get hint (: string)
	// minAge: has explicit type -> no hint
	doc := `fn FindPeople(name, minAge: int) ` + "`" + `
  MATCH (p:Person)
  WHERE p.name = $name AND p.age >= $minAge
  RETURN p
` + "`"

	err := server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    doc,
		},
	})
	if err != nil {
		t.Fatalf("DidOpen() error: %v", err)
	}

	// Request inlay hints
	hints, err := server.InlayHint(ctx, &lsp.InlayHintParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: 5, Character: 0},
		},
	})
	if err != nil {
		t.Fatalf("InlayHint() error: %v", err)
	}

	// Should have one hint for 'name' only (minAge has explicit type)
	if len(hints) != 1 {
		t.Fatalf("Expected 1 inlay hint, got %d", len(hints))
	}

	// Verify it's for the name parameter with string type
	if hints[0].Label != ": string" {
		t.Errorf("Expected hint for name with ': string', got %q", hints[0].Label)
	}
}

func TestServer_InlayHints_NoSchema(t *testing.T) {
	t.Parallel()

	server, _ := newTestServer(t)
	ctx := context.Background()

	// Initialize the server - NO schema set
	_, _ = server.Initialize(ctx, &protocol.InitializeParams{})
	_ = server.Initialized(ctx, &protocol.InitializedParams{})

	// Open a document with an untyped parameter
	doc := `fn FindPerson(name) ` + "`" + `
  MATCH (p:Person {name: $name})
  RETURN p
` + "`"

	err := server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     "file:///test.scaf",
			Version: 1,
			Text:    doc,
		},
	})
	if err != nil {
		t.Fatalf("DidOpen() error: %v", err)
	}

	// Request inlay hints
	hints, err := server.InlayHint(ctx, &lsp.InlayHintParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.scaf"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: 4, Character: 0},
		},
	})
	if err != nil {
		t.Fatalf("InlayHint() error: %v", err)
	}

	// Should have NO hints since there's no schema to infer types from
	if len(hints) != 0 {
		t.Fatalf("Expected 0 inlay hints (no schema), got %d", len(hints))
	}
}
