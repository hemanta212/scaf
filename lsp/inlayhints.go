package lsp

import (
	"context"
	"encoding/json"

	"go.lsp.dev/protocol"

	"github.com/rlch/scaf"
)

// InlayHint types for LSP 3.17+ support.
// These are defined locally since go.lsp.dev/protocol v0.12.0 doesn't include them.

// InlayHintParams represents the params for textDocument/inlayHint request.
type InlayHintParams struct {
	// TextDocument is the document to request inlay hints for.
	TextDocument protocol.TextDocumentIdentifier `json:"textDocument"`

	// Range is the visible document range for which inlay hints should be computed.
	Range protocol.Range `json:"range"`
}

// InlayHint represents an inlay hint shown inline in the editor.
type InlayHint struct {
	// Position is where the hint should be shown.
	Position protocol.Position `json:"position"`

	// Label is the hint text to display.
	Label string `json:"label"`

	// Kind is the type of hint (Type or Parameter).
	Kind InlayHintKind `json:"kind,omitempty"`

	// Tooltip provides additional information on hover.
	Tooltip *MarkupContentOrString `json:"tooltip,omitempty"`

	// PaddingLeft adds padding before the hint.
	PaddingLeft bool `json:"paddingLeft,omitempty"`

	// PaddingRight adds padding after the hint.
	PaddingRight bool `json:"paddingRight,omitempty"`
}

// MarkupContentOrString can be either a string or MarkupContent.
type MarkupContentOrString struct {
	Value string `json:"value,omitempty"`
	Kind  string `json:"kind,omitempty"`
}

// InlayHintKind defines the type of inlay hint.
type InlayHintKind int

const (
	// InlayHintKindType is a hint that shows parameter or return types.
	InlayHintKindType InlayHintKind = 1

	// InlayHintKindParameter is a hint that shows parameter names.
	InlayHintKindParameter InlayHintKind = 2
)

// InlayHint handles textDocument/inlayHint requests.
// Returns inlay hints for inferred parameter types in function signatures.
func (s *Server) InlayHint(ctx context.Context, params *InlayHintParams) ([]InlayHint, error) {
	s.mu.RLock()
	doc, ok := s.documents[params.TextDocument.URI]
	s.mu.RUnlock()

	if !ok {
		return nil, nil
	}

	var hints []InlayHint

	// Get dialect LSP for query body hints
	dialectLSP := s.getDialectLSP()
	if dialectLSP == nil {
		return hints, nil
	}

	// Check if we have analysis
	if doc.Analysis == nil || doc.Analysis.Suite == nil {
		return hints, nil
	}

	// Process each function to find parameters needing type hints
	for _, fn := range doc.Analysis.Suite.Functions {
		if fn == nil {
			continue
		}

		// Check if function is in the visible range
		fnStart := protocol.Position{
			Line:      uint32(fn.Pos.Line - 1),   //nolint:gosec
			Character: uint32(fn.Pos.Column - 1), //nolint:gosec
		}
		fnEnd := protocol.Position{
			Line:      uint32(fn.EndPos.Line - 1),   //nolint:gosec
			Character: uint32(fn.EndPos.Column - 1), //nolint:gosec
		}

		if !rangesOverlap(params.Range, protocol.Range{Start: fnStart, End: fnEnd}) {
			continue
		}

		// Build declared params map (nil type means no explicit type annotation)
		declaredParams := make(map[string]*scaf.TypeExpr)
		for _, p := range fn.Params {
			if p != nil {
				declaredParams[p.Name] = p.Type
			}
		}

		// Build query LSP context
		qctx := &scaf.QueryLSPContext{
			Schema:         s.getSchema(),
			FunctionScope:  fn.Name,
			FilePath:       URIToPath(doc.URI),
			DeclaredParams: declaredParams,
		}

		// Get inlay hints from dialect
		dialectHints := dialectLSP.InlayHints(fn.Body, qctx)

		// Convert dialect hints to LSP hints, positioning at the parameter declarations
		for _, dh := range dialectHints {
			// Find the parameter position in the function signature
			paramPos := s.findParameterPosition(fn, dh.ParameterName)
			if paramPos == nil {
				continue
			}

			hint := InlayHint{
				Position:    *paramPos,
				Label:       dh.Label,
				Kind:        InlayHintKindType,
				PaddingLeft: false,
			}

			if dh.Tooltip != "" {
				hint.Tooltip = &MarkupContentOrString{
					Kind:  "markdown",
					Value: dh.Tooltip,
				}
			}

			hints = append(hints, hint)
		}
	}

	return hints, nil
}

// findParameterPosition finds the position after a parameter name in a function signature.
// Returns the position where the type hint should be inserted (after the param name).
func (s *Server) findParameterPosition(fn *scaf.Function, paramName string) *protocol.Position {
	for _, p := range fn.Params {
		if p == nil || p.Name != paramName {
			continue
		}

		// Skip if parameter already has a type annotation
		if p.Type != nil {
			return nil
		}

		// Position is at the end of the parameter name
		// p.Pos is 1-indexed, LSP is 0-indexed
		pos := protocol.Position{
			Line:      uint32(p.Pos.Line - 1),                       //nolint:gosec
			Character: uint32(p.Pos.Column - 1 + len(p.Name)), //nolint:gosec
		}
		return &pos
	}
	return nil
}

// handleInlayHintRequest handles the textDocument/inlayHint request.
// This is called from Request() since the protocol library doesn't have inlay hint types.
func (s *Server) handleInlayHintRequest(ctx context.Context, params any) (any, error) {
	// Convert params to InlayHintParams
	data, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	var ihParams InlayHintParams
	if err := json.Unmarshal(data, &ihParams); err != nil {
		return nil, err
	}

	return s.InlayHint(ctx, &ihParams)
}
