package lsp

import (
	"context"

	"github.com/alecthomas/participle/v2/lexer"
	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
)

// publishDiagnostics converts analysis diagnostics to LSP format and publishes them.
func (s *Server) publishDiagnostics(ctx context.Context, doc *Document) {
	s.logger.Debug("publishDiagnostics: START",
		zap.String("uri", string(doc.URI)),
		zap.Int32("version", doc.Version))

	if doc.Analysis == nil {
		s.logger.Debug("publishDiagnostics: no analysis, returning early")
		return
	}

	s.logger.Debug("publishDiagnostics: converting diagnostics",
		zap.Int("count", len(doc.Analysis.Diagnostics)))

	diagnostics := make([]protocol.Diagnostic, 0, len(doc.Analysis.Diagnostics))

	for i, d := range doc.Analysis.Diagnostics {
		lspDiag := convertDiagnostic(d)
		s.logger.Debug("publishDiagnostics: converted diagnostic",
			zap.Int("index", i),
			zap.Int("span.start.line", d.Span.Start.Line),
			zap.Int("span.start.col", d.Span.Start.Column),
			zap.Uint32("lsp.start.line", lspDiag.Range.Start.Line),
			zap.Uint32("lsp.start.char", lspDiag.Range.Start.Character),
			zap.String("message", d.Message),
			zap.String("code", d.Code))
		diagnostics = append(diagnostics, lspDiag)
	}

	// Add dialect diagnostics for query bodies
	dialectDiags := s.collectDialectDiagnostics(doc)
	diagnostics = append(diagnostics, dialectDiags...)

	s.logger.Debug("publishDiagnostics: calling client.PublishDiagnostics RPC",
		zap.Int("diagnosticCount", len(diagnostics)))

	err := s.client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
		URI:         doc.URI,
		Version:     uint32(doc.Version), //nolint:gosec // LSP version numbers are always non-negative
		Diagnostics: diagnostics,
	})

	if err != nil {
		s.logger.Error("publishDiagnostics: RPC failed", zap.Error(err))
	} else {
		s.logger.Debug("publishDiagnostics: RPC completed successfully")
	}

	s.logger.Debug("publishDiagnostics: END")
}

// convertDiagnostic converts an analysis.Diagnostic to an LSP protocol.Diagnostic.
func convertDiagnostic(d analysis.Diagnostic) protocol.Diagnostic {
	return protocol.Diagnostic{
		Range:    spanToRange(d.Span),
		Severity: convertSeverity(d.Severity),
		Code:     d.Code,
		Source:   d.Source,
		Message:  d.Message,
	}
}

// convertSeverity converts analysis severity to LSP severity.
func convertSeverity(sev analysis.DiagnosticSeverity) protocol.DiagnosticSeverity {
	switch sev {
	case analysis.SeverityError:
		return protocol.DiagnosticSeverityError
	case analysis.SeverityWarning:
		return protocol.DiagnosticSeverityWarning
	case analysis.SeverityInformation:
		return protocol.DiagnosticSeverityInformation
	case analysis.SeverityHint:
		return protocol.DiagnosticSeverityHint
	default:
		return protocol.DiagnosticSeverityError
	}
}

// queryBodyForDiagnostics holds info needed to run diagnostics on a query body.
type queryBodyForDiagnostics struct {
	body         string
	bodyToken    *lexer.Token
	functionName string
	params       map[string]*scaf.TypeExpr
}

// collectDialectDiagnostics runs the dialect's diagnostic checks on all query bodies in the document.
func (s *Server) collectDialectDiagnostics(doc *Document) []protocol.Diagnostic {
	dialectLSP := s.getDialectLSP()
	if dialectLSP == nil {
		s.logger.Debug("collectDialectDiagnostics: no dialect LSP available")
		return nil
	}

	if doc.Analysis == nil || doc.Analysis.Suite == nil {
		s.logger.Debug("collectDialectDiagnostics: no analysis available")
		return nil
	}

	// Collect all query bodies from the AST
	queryBodies := s.collectAllQueryBodies(doc.Analysis.Suite)

	s.logger.Debug("collectDialectDiagnostics: found query bodies",
		zap.Int("count", len(queryBodies)))

	var diagnostics []protocol.Diagnostic

	for _, qb := range queryBodies {
		if qb.body == "" || qb.bodyToken == nil {
			continue
		}

		s.logger.Debug("collectDialectDiagnostics: processing query body",
			zap.String("functionName", qb.functionName),
			zap.Int("bodyLen", len(qb.body)),
			zap.Int("tokenOffset", qb.bodyToken.Pos.Offset))

		// Build a query context
		qbc := &QueryBodyContext{
			Query:          qb.body,
			FunctionName:   qb.functionName,
			DeclaredParams: qb.params,
		}

		// Get position from token (after opening backtick)
		qbc.QueryBodyStart = protocol.Position{
			Line:      uint32(qb.bodyToken.Pos.Line - 1),   //nolint:gosec
			Character: uint32(qb.bodyToken.Pos.Column - 1), //nolint:gosec
		}
		// Adjust for opening backtick
		qbc.QueryBodyStart.Character++

		queryCtx := s.buildQueryLSPContext(doc, qbc, "")
		dialectDiags := dialectLSP.Diagnostics(qb.body, queryCtx)

		s.logger.Debug("collectDialectDiagnostics: got dialect diagnostics",
			zap.Int("count", len(dialectDiags)))

		// Convert to LSP diagnostics with adjusted positions
		for i, d := range dialectDiags {
			lspDiag := s.convertQueryDiagnosticWithContext(d, qbc, qb.body)
			s.logger.Debug("collectDialectDiagnostics: converted diagnostic",
				zap.Int("index", i),
				zap.String("code", d.Code),
				zap.Int("queryRangeStart", d.Range.Start),
				zap.Int("queryRangeEnd", d.Range.End),
				zap.Uint32("lspStartLine", lspDiag.Range.Start.Line),
				zap.Uint32("lspStartChar", lspDiag.Range.Start.Character),
				zap.Uint32("lspEndLine", lspDiag.Range.End.Line),
				zap.Uint32("lspEndChar", lspDiag.Range.End.Character))
			diagnostics = append(diagnostics, lspDiag)
		}
	}

	return diagnostics
}

// collectAllQueryBodies walks the AST and collects all query bodies for diagnostics.
func (s *Server) collectAllQueryBodies(suite *scaf.File) []queryBodyForDiagnostics {
	var bodies []queryBodyForDiagnostics

	// Collect from function definitions
	for _, fn := range suite.Functions {
		if fn == nil || fn.Body == "" {
			continue
		}
		tok := s.findRawStringToken(fn.Tokens)
		if tok == nil {
			continue
		}

		params := make(map[string]*scaf.TypeExpr)
		for _, p := range fn.Params {
			if p != nil {
				params[p.Name] = p.Type
			}
		}

		bodies = append(bodies, queryBodyForDiagnostics{
			body:         fn.Body,
			bodyToken:    tok,
			functionName: fn.Name,
			params:       params,
		})
	}

	// Collect from global setup
	if suite.Setup != nil {
		bodies = append(bodies, s.collectQueryBodiesFromSetup(suite.Setup)...)
	}

	// Collect from global teardown
	if suite.Teardown != nil {
		if tok := s.findRawStringTokenAfterKeyword(suite.Tokens, scaf.TokenTeardown); tok != nil {
			bodies = append(bodies, queryBodyForDiagnostics{
				body:      *suite.Teardown,
				bodyToken: tok,
			})
		}
	}

	// Collect from scopes
	for _, scope := range suite.Scopes {
		if scope == nil {
			continue
		}
		bodies = append(bodies, s.collectQueryBodiesFromScope(scope)...)
	}

	return bodies
}

// collectQueryBodiesFromSetup collects query bodies from a setup clause.
func (s *Server) collectQueryBodiesFromSetup(setup *scaf.SetupClause) []queryBodyForDiagnostics {
	var bodies []queryBodyForDiagnostics

	if setup.Inline != nil {
		if tok := s.findRawStringToken(setup.Tokens); tok != nil {
			bodies = append(bodies, queryBodyForDiagnostics{
				body:      *setup.Inline,
				bodyToken: tok,
			})
		}
	}

	for _, item := range setup.Block {
		if item == nil || item.Inline == nil {
			continue
		}
		if tok := s.findRawStringToken(item.Tokens); tok != nil {
			bodies = append(bodies, queryBodyForDiagnostics{
				body:      *item.Inline,
				bodyToken: tok,
			})
		}
	}

	return bodies
}

// collectQueryBodiesFromScope collects query bodies from a function scope.
func (s *Server) collectQueryBodiesFromScope(scope *scaf.FunctionScope) []queryBodyForDiagnostics {
	var bodies []queryBodyForDiagnostics

	// Scope setup
	if scope.Setup != nil {
		bodies = append(bodies, s.collectQueryBodiesFromSetup(scope.Setup)...)
	}

	// Scope teardown
	if scope.Teardown != nil {
		if tok := s.findRawStringTokenAfterKeyword(scope.Tokens, scaf.TokenTeardown); tok != nil {
			bodies = append(bodies, queryBodyForDiagnostics{
				body:      *scope.Teardown,
				bodyToken: tok,
			})
		}
	}

	// Items (tests and groups)
	for _, item := range scope.Items {
		if item == nil {
			continue
		}
		bodies = append(bodies, s.collectQueryBodiesFromTestOrGroup(item)...)
	}

	return bodies
}

// collectQueryBodiesFromTestOrGroup collects query bodies from a test or group.
func (s *Server) collectQueryBodiesFromTestOrGroup(item *scaf.TestOrGroup) []queryBodyForDiagnostics {
	if item.Test != nil {
		return s.collectQueryBodiesFromTest(item.Test)
	}
	if item.Group != nil {
		return s.collectQueryBodiesFromGroup(item.Group)
	}
	return nil
}

// collectQueryBodiesFromTest collects query bodies from a test.
func (s *Server) collectQueryBodiesFromTest(test *scaf.Test) []queryBodyForDiagnostics {
	var bodies []queryBodyForDiagnostics

	// Test setup
	if test.Setup != nil {
		bodies = append(bodies, s.collectQueryBodiesFromSetup(test.Setup)...)
	}

	// Assert queries
	for _, assert := range test.Asserts {
		if assert == nil || assert.Query == nil || assert.Query.Inline == nil {
			continue
		}
		if tok := s.findRawStringToken(assert.Query.Tokens); tok != nil {
			bodies = append(bodies, queryBodyForDiagnostics{
				body:      *assert.Query.Inline,
				bodyToken: tok,
			})
		}
	}

	return bodies
}

// collectQueryBodiesFromGroup collects query bodies from a group.
func (s *Server) collectQueryBodiesFromGroup(group *scaf.Group) []queryBodyForDiagnostics {
	var bodies []queryBodyForDiagnostics

	// Group setup
	if group.Setup != nil {
		bodies = append(bodies, s.collectQueryBodiesFromSetup(group.Setup)...)
	}

	// Group teardown
	if group.Teardown != nil {
		if tok := s.findRawStringTokenAfterKeyword(group.Tokens, scaf.TokenTeardown); tok != nil {
			bodies = append(bodies, queryBodyForDiagnostics{
				body:      *group.Teardown,
				bodyToken: tok,
			})
		}
	}

	// Nested items
	for _, item := range group.Items {
		if item == nil {
			continue
		}
		bodies = append(bodies, s.collectQueryBodiesFromTestOrGroup(item)...)
	}

	return bodies
}

// convertQueryDiagnosticWithContext converts a dialect query diagnostic to LSP format
// using the precise query body context.
func (s *Server) convertQueryDiagnosticWithContext(d scaf.QueryDiagnostic, qbc *QueryBodyContext, queryBody string) protocol.Diagnostic {
	// The diagnostic range (d.Range) is in byte offsets within the query body.
	// We need to map these offsets to document positions.

	// Calculate start position: QueryBodyStart + offset from query start
	startPos := s.queryOffsetToDocPosition(qbc, d.Range.Start, queryBody)
	endPos := s.queryOffsetToDocPosition(qbc, d.Range.End, queryBody)

	return protocol.Diagnostic{
		Range: protocol.Range{
			Start: startPos,
			End:   endPos,
		},
		Severity: s.convertQueryDiagnosticSeverity(d.Severity),
		Code:     d.Code,
		Source:   s.dialectName,
		Message:  d.Message,
	}
}

// queryOffsetToDocPosition converts a byte offset within a query body to a document position.
// It handles multi-line queries by counting newlines from the query body start.
func (s *Server) queryOffsetToDocPosition(qbc *QueryBodyContext, offset int, queryBody string) protocol.Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(queryBody) {
		offset = len(queryBody)
	}

	// Count newlines and characters to get to the offset
	line := qbc.QueryBodyStart.Line
	char := qbc.QueryBodyStart.Character

	for i := 0; i < offset && i < len(queryBody); i++ {
		if queryBody[i] == '\n' {
			line++
			char = 0
		} else {
			char++
		}
	}

	return protocol.Position{
		Line:      line,
		Character: char,
	}
}
