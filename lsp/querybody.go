package lsp

import (
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
	"go.lsp.dev/protocol"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
)

// QueryBodyContext holds context when the cursor is inside a query body (backtick string).
type QueryBodyContext struct {
	// Query is the query string (content between backticks).
	Query string

	// Offset is the byte offset within the query string.
	Offset int

	// FunctionName is the enclosing function name (for fn definitions).
	FunctionName string

	// DeclaredParams are parameters declared in the function signature.
	DeclaredParams map[string]*scaf.TypeExpr

	// QueryBodyStart is the document position where the query body starts.
	QueryBodyStart protocol.Position

	// QueryBodyEnd is the document position where the query body ends.
	QueryBodyEnd protocol.Position
}

// queryBodyInfo holds information about a query body location found in the AST.
type queryBodyInfo struct {
	// body is the query body content (without backticks).
	body string
	// bodyToken is the RawString token containing the query body.
	bodyToken *lexer.Token
	// functionName is set if this is a function definition body.
	functionName string
	// params are declared parameters (for function definitions).
	params map[string]*scaf.TypeExpr
}

// getQueryBodyContext determines if the cursor position is inside a query body.
// Returns nil if not inside a query body.
//
// Query bodies are backtick-delimited strings in:
// - fn definitions: fn GetUser(id: string) `MATCH (u:User {id: $id}) RETURN u`
// - inline setup: setup `CREATE (:User)`
// - assert inline queries: assert `MATCH ...` { ... }
// - teardown: teardown `DELETE ...`
func (s *Server) getQueryBodyContext(doc *Document, pos protocol.Position) *QueryBodyContext {
	af := doc.Analysis
	if af == nil || af.Suite == nil {
		return nil
	}

	// Convert LSP position to lexer position (1-indexed)
	lexPos := analysis.PositionToLexer(pos.Line, pos.Character)

	// Find a query body that contains this position
	info := s.findQueryBodyAtPosition(af.Suite, lexPos)
	if info == nil {
		return nil
	}

	// Calculate offset within the query body
	// The token value includes backticks, so body starts at position + 1
	bodyStartOffset := info.bodyToken.Pos.Offset + 1 // skip opening backtick
	cursorOffset := s.positionToOffset(doc.Content, pos)
	queryOffset := cursorOffset - bodyStartOffset

	if queryOffset < 0 {
		queryOffset = 0
	}
	if queryOffset > len(info.body) {
		queryOffset = len(info.body)
	}

	// Calculate document positions for the body (inside the backticks)
	bodyStartPos := protocol.Position{
		Line:      uint32(info.bodyToken.Pos.Line - 1), // convert to 0-indexed
		Character: uint32(info.bodyToken.Pos.Column),   // column after opening backtick
	}

	// Calculate end position by walking through the body content
	bodyEndPos := s.offsetToPosition(doc.Content, bodyStartOffset+len(info.body))

	return &QueryBodyContext{
		Query:          info.body,
		Offset:         queryOffset,
		FunctionName:   info.functionName,
		DeclaredParams: info.params,
		QueryBodyStart: bodyStartPos,
		QueryBodyEnd:   bodyEndPos,
	}
}

// findQueryBodyAtPosition searches the AST for a query body containing the given position.
func (s *Server) findQueryBodyAtPosition(suite *scaf.File, pos lexer.Position) *queryBodyInfo {
	// Check function definitions
	for _, fn := range suite.Functions {
		if fn == nil {
			continue
		}
		if info := s.checkFunctionBody(fn, pos); info != nil {
			return info
		}
	}

	// Check global setup
	if suite.Setup != nil {
		if info := s.checkSetupClause(suite.Setup, pos); info != nil {
			return info
		}
	}

	// Check global teardown
	if suite.Teardown != nil {
		if info := s.checkTeardownBody(*suite.Teardown, suite.Tokens, pos); info != nil {
			return info
		}
	}

	// Check scopes
	for _, scope := range suite.Scopes {
		if scope == nil {
			continue
		}
		if info := s.checkScope(scope, pos); info != nil {
			return info
		}
	}

	return nil
}

// checkFunctionBody checks if position is inside a function's query body.
func (s *Server) checkFunctionBody(fn *scaf.Function, pos lexer.Position) *queryBodyInfo {
	// Find the RawString token in the function's tokens
	tok := s.findRawStringToken(fn.Tokens)
	if tok == nil {
		return nil
	}

	if !s.positionInRawString(tok, pos) {
		return nil
	}

	// Build params map
	params := make(map[string]*scaf.TypeExpr)
	for _, p := range fn.Params {
		if p != nil {
			params[p.Name] = p.Type
		}
	}

	return &queryBodyInfo{
		body:         fn.Body,
		bodyToken:    tok,
		functionName: fn.Name,
		params:       params,
	}
}

// checkSetupClause checks if position is inside a setup clause's inline query.
func (s *Server) checkSetupClause(setup *scaf.SetupClause, pos lexer.Position) *queryBodyInfo {
	if setup.Inline != nil {
		tok := s.findRawStringToken(setup.Tokens)
		if tok != nil && s.positionInRawString(tok, pos) {
			return &queryBodyInfo{
				body:      *setup.Inline,
				bodyToken: tok,
			}
		}
	}

	// Check block items
	for _, item := range setup.Block {
		if item == nil {
			continue
		}
		if info := s.checkSetupItem(item, pos); info != nil {
			return info
		}
	}

	return nil
}

// checkSetupItem checks if position is inside a setup item's inline query.
func (s *Server) checkSetupItem(item *scaf.SetupItem, pos lexer.Position) *queryBodyInfo {
	if item.Inline == nil {
		return nil
	}

	tok := s.findRawStringToken(item.Tokens)
	if tok == nil || !s.positionInRawString(tok, pos) {
		return nil
	}

	return &queryBodyInfo{
		body:      *item.Inline,
		bodyToken: tok,
	}
}

// checkTeardownBody checks if position is inside a teardown query body.
// Takes the body string from the AST (already has backticks stripped by participle.Unquote).
func (s *Server) checkTeardownBody(body string, tokens []lexer.Token, pos lexer.Position) *queryBodyInfo {
	// Find the RawString token after the teardown keyword
	tok := s.findRawStringTokenAfterKeyword(tokens, scaf.TokenTeardown)
	if tok == nil || !s.positionInRawString(tok, pos) {
		return nil
	}

	return &queryBodyInfo{
		body:      body,
		bodyToken: tok,
	}
}

// findRawStringTokenAfterKeyword finds a RawString token that appears after a specific keyword token.
func (s *Server) findRawStringTokenAfterKeyword(tokens []lexer.Token, keywordType lexer.TokenType) *lexer.Token {
	foundKeyword := false
	for i := range tokens {
		if tokens[i].Type == keywordType {
			foundKeyword = true
			continue
		}
		if foundKeyword && tokens[i].Type == scaf.TokenRawString {
			return &tokens[i]
		}
		// Skip whitespace and comments while looking
		if foundKeyword && tokens[i].Type != scaf.TokenWhitespace && tokens[i].Type != scaf.TokenComment {
			// Found something else after keyword, reset
			foundKeyword = false
		}
	}
	return nil
}

// checkScope checks if position is inside any query body within a scope.
func (s *Server) checkScope(scope *scaf.FunctionScope, pos lexer.Position) *queryBodyInfo {
	// Check scope setup
	if scope.Setup != nil {
		if info := s.checkSetupClause(scope.Setup, pos); info != nil {
			return info
		}
	}

	// Check scope teardown
	if scope.Teardown != nil {
		if info := s.checkTeardownBody(*scope.Teardown, scope.Tokens, pos); info != nil {
			return info
		}
	}

	// Check items (tests and groups)
	for _, item := range scope.Items {
		if item == nil {
			continue
		}
		if info := s.checkTestOrGroup(item, pos); info != nil {
			return info
		}
	}

	return nil
}

// checkTestOrGroup checks if position is inside any query body within a test or group.
func (s *Server) checkTestOrGroup(item *scaf.TestOrGroup, pos lexer.Position) *queryBodyInfo {
	if item.Test != nil {
		return s.checkTest(item.Test, pos)
	}
	if item.Group != nil {
		return s.checkGroup(item.Group, pos)
	}
	return nil
}

// checkTest checks if position is inside any query body within a test.
func (s *Server) checkTest(test *scaf.Test, pos lexer.Position) *queryBodyInfo {
	// Check test setup
	if test.Setup != nil {
		if info := s.checkSetupClause(test.Setup, pos); info != nil {
			return info
		}
	}

	// Check assert queries
	for _, assert := range test.Asserts {
		if assert == nil || assert.Query == nil {
			continue
		}
		if info := s.checkAssertQuery(assert.Query, pos); info != nil {
			return info
		}
	}

	return nil
}

// checkGroup checks if position is inside any query body within a group.
func (s *Server) checkGroup(group *scaf.Group, pos lexer.Position) *queryBodyInfo {
	// Check group setup
	if group.Setup != nil {
		if info := s.checkSetupClause(group.Setup, pos); info != nil {
			return info
		}
	}

	// Check group teardown
	if group.Teardown != nil {
		if info := s.checkTeardownBody(*group.Teardown, group.Tokens, pos); info != nil {
			return info
		}
	}

	// Check nested items
	for _, item := range group.Items {
		if item == nil {
			continue
		}
		if info := s.checkTestOrGroup(item, pos); info != nil {
			return info
		}
	}

	return nil
}

// checkAssertQuery checks if position is inside an assert query's inline query.
func (s *Server) checkAssertQuery(aq *scaf.AssertQuery, pos lexer.Position) *queryBodyInfo {
	if aq.Inline == nil {
		return nil
	}

	tok := s.findRawStringToken(aq.Tokens)
	if tok == nil || !s.positionInRawString(tok, pos) {
		return nil
	}

	return &queryBodyInfo{
		body:      *aq.Inline,
		bodyToken: tok,
	}
}

// findRawStringToken finds the first RawString token in a slice of tokens.
func (s *Server) findRawStringToken(tokens []lexer.Token) *lexer.Token {
	for i := range tokens {
		if tokens[i].Type == scaf.TokenRawString {
			return &tokens[i]
		}
	}
	return nil
}

// positionInRawString checks if a position is inside a raw string token's content.
// The position must be after the opening backtick and before the closing backtick.
func (s *Server) positionInRawString(tok *lexer.Token, pos lexer.Position) bool {
	if tok == nil || tok.Type != scaf.TokenRawString {
		return false
	}

	// Token value includes backticks, e.g., `MATCH ...`
	// Content starts at tok.Pos (the opening backtick)
	// We want to check if pos is strictly inside (not on the backticks)

	startLine := tok.Pos.Line
	startCol := tok.Pos.Column + 1 // after opening backtick

	// Calculate end position by walking through token value
	endLine := tok.Pos.Line
	endCol := tok.Pos.Column
	for _, ch := range tok.Value {
		if ch == '\n' {
			endLine++
			endCol = 1
		} else {
			endCol++
		}
	}
	endCol-- // before closing backtick

	// Check if position is after start
	if pos.Line < startLine || (pos.Line == startLine && pos.Column < startCol) {
		return false
	}

	// Check if position is before end
	if pos.Line > endLine || (pos.Line == endLine && pos.Column > endCol) {
		return false
	}

	return true
}

// positionToOffset converts an LSP Position to a byte offset in the content.
func (s *Server) positionToOffset(content string, pos protocol.Position) int {
	lines := strings.Split(content, "\n")
	offset := 0

	for i := 0; i < int(pos.Line) && i < len(lines); i++ {
		offset += len(lines[i]) + 1 // +1 for newline
	}

	if int(pos.Line) < len(lines) {
		offset += min(int(pos.Character), len(lines[pos.Line]))
	}

	return offset
}

// offsetToPosition converts a byte offset to an LSP Position.
func (s *Server) offsetToPosition(content string, offset int) protocol.Position {
	if offset < 0 {
		return protocol.Position{Line: 0, Character: 0}
	}

	line := uint32(0)
	col := uint32(0)

	for i := 0; i < offset && i < len(content); i++ {
		if content[i] == '\n' {
			line++
			col = 0
		} else {
			col++
		}
	}

	return protocol.Position{Line: line, Character: col}
}

// positionInSpan checks if an LSP position is within a scaf Span.
func (s *Server) positionInSpan(pos protocol.Position, span scaf.Span) bool {
	// Convert LSP 0-indexed to scaf 1-indexed
	line := int(pos.Line) + 1
	col := int(pos.Character) + 1

	// Check start boundary
	if line < span.Start.Line || (line == span.Start.Line && col < span.Start.Column) {
		return false
	}

	// Check end boundary
	if line > span.End.Line || (line == span.End.Line && col > span.End.Column) {
		return false
	}

	return true
}

// buildQueryLSPContext creates a QueryLSPContext for dialect LSP calls.
func (s *Server) buildQueryLSPContext(doc *Document, qbc *QueryBodyContext, triggerChar string) *scaf.QueryLSPContext {
	ctx := &scaf.QueryLSPContext{
		FunctionScope:    qbc.FunctionName,
		FilePath:         URIToPath(doc.URI),
		DeclaredParams:   qbc.DeclaredParams,
		TriggerCharacter: triggerChar,
		Schema:           s.getSchema(),
	}

	return ctx
}

// getDialectLSP returns the DialectLSP for the current dialect.
func (s *Server) getDialectLSP() scaf.DialectLSP { //nolint:ireturn
	return scaf.GetDialectLSP(s.dialectName)
}

// convertDialectCompletions converts dialect completions to LSP protocol completions.
func (s *Server) convertDialectCompletions(items []scaf.QueryCompletion) []protocol.CompletionItem {
	result := make([]protocol.CompletionItem, 0, len(items))

	for _, item := range items {
		lspItem := protocol.CompletionItem{
			Label:      item.Label,
			Kind:       s.convertCompletionKind(item.Kind),
			Detail:     item.Detail,
			InsertText: item.InsertText,
			Deprecated: item.Deprecated,
		}

		if item.Documentation != "" {
			lspItem.Documentation = &protocol.MarkupContent{
				Kind:  protocol.Markdown,
				Value: item.Documentation,
			}
		}

		if item.IsSnippet {
			lspItem.InsertTextFormat = protocol.InsertTextFormatSnippet
		}

		if item.SortText != "" {
			lspItem.SortText = item.SortText
		}

		if item.FilterText != "" {
			lspItem.FilterText = item.FilterText
		}

		result = append(result, lspItem)
	}

	return result
}

// convertCompletionKind converts dialect completion kind to LSP protocol kind.
func (s *Server) convertCompletionKind(kind scaf.QueryCompletionKind) protocol.CompletionItemKind {
	switch kind {
	case scaf.QueryCompletionKeyword:
		return protocol.CompletionItemKindKeyword
	case scaf.QueryCompletionFunction:
		return protocol.CompletionItemKindFunction
	case scaf.QueryCompletionLabel:
		return protocol.CompletionItemKindClass
	case scaf.QueryCompletionProperty:
		return protocol.CompletionItemKindProperty
	case scaf.QueryCompletionRelType:
		return protocol.CompletionItemKindInterface
	case scaf.QueryCompletionVariable:
		return protocol.CompletionItemKindVariable
	case scaf.QueryCompletionParameter:
		return protocol.CompletionItemKindVariable
	case scaf.QueryCompletionSnippet:
		return protocol.CompletionItemKindSnippet
	case scaf.QueryCompletionOperator:
		return protocol.CompletionItemKindOperator
	case scaf.QueryCompletionProcedure:
		return protocol.CompletionItemKindMethod
	default:
		return protocol.CompletionItemKindText
	}
}

// convertDialectHover converts dialect hover to LSP protocol hover.
func (s *Server) convertDialectHover(hover *scaf.QueryHover, qbc *QueryBodyContext) *protocol.Hover {
	if hover == nil {
		return nil
	}

	result := &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.Markdown,
			Value: hover.Contents,
		},
	}

	if hover.Range != nil {
		// Convert query range to document range
		rng := s.queryRangeToDocRange(hover.Range, qbc)
		result.Range = &rng
	}

	return result
}

// queryRangeToDocRange converts a query-relative range to a document range.
// It handles multi-line queries by counting newlines from the query body start.
func (s *Server) queryRangeToDocRange(qr *scaf.QueryRange, qbc *QueryBodyContext) protocol.Range {
	return protocol.Range{
		Start: s.queryOffsetToDocPos(qbc, qr.Start),
		End:   s.queryOffsetToDocPos(qbc, qr.End),
	}
}

// queryOffsetToDocPos converts a byte offset within a query body to a document position.
// It handles multi-line queries by counting newlines from the query body start.
func (s *Server) queryOffsetToDocPos(qbc *QueryBodyContext, offset int) protocol.Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(qbc.Query) {
		offset = len(qbc.Query)
	}

	// Count newlines and characters to get to the offset
	line := qbc.QueryBodyStart.Line
	char := qbc.QueryBodyStart.Character

	for i := 0; i < offset && i < len(qbc.Query); i++ {
		if qbc.Query[i] == '\n' {
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

// convertQueryDiagnosticSeverity converts dialect diagnostic severity to LSP protocol severity.
func (s *Server) convertQueryDiagnosticSeverity(sev scaf.QueryDiagnosticSeverity) protocol.DiagnosticSeverity {
	switch sev {
	case scaf.QueryDiagnosticError:
		return protocol.DiagnosticSeverityError
	case scaf.QueryDiagnosticWarning:
		return protocol.DiagnosticSeverityWarning
	case scaf.QueryDiagnosticInfo:
		return protocol.DiagnosticSeverityInformation
	case scaf.QueryDiagnosticHint:
		return protocol.DiagnosticSeverityHint
	default:
		return protocol.DiagnosticSeverityError
	}
}

// convertDialectSignatureHelp converts dialect signature help to LSP protocol.
func (s *Server) convertDialectSignatureHelp(sh *scaf.QuerySignatureHelp) *protocol.SignatureHelp {
	if sh == nil {
		return nil
	}

	sigs := make([]protocol.SignatureInformation, 0, len(sh.Signatures))
	for _, sig := range sh.Signatures {
		params := make([]protocol.ParameterInformation, 0, len(sig.Parameters))
		for _, p := range sig.Parameters {
			params = append(params, protocol.ParameterInformation{
				Label: p.Label,
				Documentation: &protocol.MarkupContent{
					Kind:  protocol.Markdown,
					Value: p.Documentation,
				},
			})
		}

		var doc any
		if sig.Documentation != "" {
			doc = &protocol.MarkupContent{
				Kind:  protocol.Markdown,
				Value: sig.Documentation,
			}
		}

		sigs = append(sigs, protocol.SignatureInformation{
			Label:         sig.Label,
			Documentation: doc,
			Parameters:    params,
		})
	}

	return &protocol.SignatureHelp{
		Signatures:      sigs,
		ActiveSignature: uint32(sh.ActiveSignature), //nolint:gosec
		ActiveParameter: uint32(sh.ActiveParameter), //nolint:gosec
	}
}
