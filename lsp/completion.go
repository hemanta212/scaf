package lsp

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/alecthomas/participle/v2/lexer"
	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
)

// completionTimeout is the maximum time for completion to prevent editor freezes.
const completionTimeout = 5 * time.Second

// Completion handles textDocument/completion requests.
// Following gopls pattern: single path, token-based dispatch.
func (s *Server) Completion(ctx context.Context, params *protocol.CompletionParams) (*protocol.CompletionList, error) {
	start := time.Now()
	s.logger.Debug("Completion request started",
		zap.String("uri", string(params.TextDocument.URI)),
		zap.Uint32("line", params.Position.Line),
		zap.Uint32("character", params.Position.Character))

	// Log completion of request
	defer func() {
		s.logger.Debug("Completion request finished",
			zap.Duration("elapsed", time.Since(start)))
	}()

	// Add timeout to prevent editor freezes
	ctx, cancel := context.WithTimeout(ctx, completionTimeout)
	_ = ctx // TODO: pass ctx to completion methods for cancellation support
	defer cancel()

	doc, ok := s.getDocument(params.TextDocument.URI)
	if !ok {
		s.logger.Debug("Completion: document not found")
		return nil, nil //nolint:nilnil
	}

	// Get trigger character from LSP context
	var triggerChar string
	if params.Context != nil && params.Context.TriggerCharacter != "" {
		triggerChar = params.Context.TriggerCharacter
	}

	// Check if we're inside a query body (backtick string)
	// If so, delegate to the dialect's LSP implementation
	if qbc := s.getQueryBodyContext(doc, params.Position); qbc != nil {
		s.logger.Debug("Completion: inside query body",
			zap.String("function", qbc.FunctionName),
			zap.Int("offset", qbc.Offset))

		if dialectLSP := s.getDialectLSP(); dialectLSP != nil {
			queryCtx := s.buildQueryLSPContext(doc, qbc, triggerChar)
			dialectItems := dialectLSP.Complete(qbc.Query, qbc.Offset, queryCtx)
			return &protocol.CompletionList{
				IsIncomplete: false,
				Items:        s.convertDialectCompletions(dialectItems),
			}, nil
		}
	}

	// Build completion context and determine what completions to offer
	cc := s.buildCompletionContext(doc, params.Position, triggerChar)
	s.logger.Debug("Completion context",
		zap.String("kind", string(cc.Kind)),
		zap.String("prefix", cc.Prefix),
		zap.String("moduleAlias", cc.ModuleAlias),
		zap.String("inScope", cc.InScope),
		zap.Bool("inTest", cc.InTest),
		zap.Bool("inExpr", cc.InExpr),
		zap.Bool("inAssert", cc.InAssert),
		zap.String("assertQueryName", cc.AssertQueryName),
		zap.String("assertQueryBody", cc.AssertQueryBody))

	// Dispatch to appropriate completion handler
	var items []protocol.CompletionItem
	switch cc.Kind {
	case CompletionKindNone:
		// No completions available
	case CompletionKindQueryName:
		items = s.completeQueryNames(doc, cc)
	case CompletionKindKeyword:
		items = s.completeKeywords(cc)
	case CompletionKindParameter:
		items = s.completeParameters(doc, cc)
	case CompletionKindReturnField:
		items = s.completeReturnFields(doc, cc)
	case CompletionKindImportAlias:
		items = s.completeImportAliases(doc, cc)
	case CompletionKindSetupFunction:
		items = s.completeSetupFunctions(doc, cc)
	case CompletionKindTypeAnnotation:
		items = s.completeTypeAnnotations(cc)
	case CompletionKindExprVariable:
		items = s.completeExprVariables(doc, cc)
	}

	// Filter by prefix
	// Skip filtering for return fields and expr variables when prefix contains a dot (e.g., "u.")
	// because completeReturnFields and completeExprVariables already handle prefix matching internally
	skipPrefixFilter := (cc.Kind == CompletionKindReturnField || cc.Kind == CompletionKindExprVariable) && strings.Contains(cc.Prefix, ".")
	if cc.Prefix != "" && !skipPrefixFilter {
		items = filterByPrefix(items, cc.Prefix)
	}

	return &protocol.CompletionList{
		IsIncomplete: false,
		Items:        items,
	}, nil
}

// CompletionKind indicates what kind of completion is expected at a position.
type CompletionKind string

const (
	CompletionKindNone           CompletionKind = "none"
	CompletionKindQueryName      CompletionKind = "query_name"
	CompletionKindKeyword        CompletionKind = "keyword"
	CompletionKindParameter      CompletionKind = "parameter"
	CompletionKindReturnField    CompletionKind = "return_field"
	CompletionKindImportAlias    CompletionKind = "import_alias"
	CompletionKindSetupFunction  CompletionKind = "setup_function"
	CompletionKindTypeAnnotation CompletionKind = "type_annotation"
	CompletionKindExprVariable   CompletionKind = "expr_variable"
)

// CompletionContext holds information about where completion was triggered.
type CompletionContext struct {
	Kind             CompletionKind
	Prefix           string // Text being typed (for filtering)
	InScope          string // Name of enclosing QueryScope
	InTest           bool   // Inside a test body
	InSetup          bool   // Inside a setup clause
	InAssert         bool   // Inside an assert block
	InExpr           bool   // Inside a parenthesized expression (assert or where clause)
	AssertQueryName  string // Name of the assert's query (if any) - takes precedence over InScope for expression variables
	AssertQueryBody  string // Inline query body (if any) - for inline assert queries
	ModuleAlias      string // Import alias for module.function completion
	TriggerChar      string // The trigger character (., $)
}

// buildCompletionContext analyzes the document and returns completion context.
// Single-path approach: inspect tokens before cursor, dispatch based on what we find.
func (s *Server) buildCompletionContext(doc *Document, pos protocol.Position, triggerChar string) *CompletionContext {
	cc := &CompletionContext{
		Kind:        CompletionKindNone,
		TriggerChar: triggerChar,
	}

	// Get document content and line text
	content := doc.Content
	lines := strings.Split(content, "\n")
	if int(pos.Line) >= len(lines) {
		return cc
	}
	lineText := lines[pos.Line]
	col := min(int(pos.Character), len(lineText))
	textBeforeCursor := lineText[:col]

	// Extract prefix (identifier being typed)
	cc.Prefix = extractPrefix(textBeforeCursor)

	// Detect trigger character from text if not provided by LSP
	if triggerChar == "" && col > 0 {
		lastChar := lineText[col-1]
		if lastChar == '$' || lastChar == '.' {
			cc.TriggerChar = string(lastChar)
		}
	}

	// Get analysis - prefer current, fall back to last valid for symbol lookup
	af := doc.Analysis
	if af == nil {
		return cc
	}

	// For symbol lookups, use last valid analysis if current has parse errors
	symbolsAnalysis := af
	if af.ParseError != nil && doc.LastValidAnalysis != nil {
		symbolsAnalysis = doc.LastValidAnalysis
	}

	// Convert to lexer position (1-indexed)
	lexPos := analysis.PositionToLexer(pos.Line, pos.Character)

	// Determine positional context (InScope, InTest, InSetup, InAssert)
	s.determinePositionalContext(cc, symbolsAnalysis, lexPos)

	// === SINGLE DISPATCH: Look at token before cursor ===
	// This is the gopls approach: one decision tree based on prev token
	cc.Kind = s.determineCompletionKind(cc, doc, symbolsAnalysis, lexPos, textBeforeCursor)

	return cc
}

// determinePositionalContext sets InScope, InTest, InSetup, InAssert, InExpr based on AST position.
func (s *Server) determinePositionalContext(cc *CompletionContext, af *analysis.AnalyzedFile, pos lexer.Position) {
	if af == nil || af.Suite == nil {
		return
	}

	for _, scope := range af.Suite.Scopes {
		if !containsLexerPosition(scope.Span(), pos) {
			continue
		}
		cc.InScope = scope.FunctionName

		// Check scope-level setup
		if scope.Setup != nil && containsLexerPosition(scope.Setup.Span(), pos) {
			cc.InSetup = true
		}

		// Check items (tests and groups)
		for _, item := range scope.Items {
			if item.Test != nil && containsLexerPosition(item.Test.Span(), pos) {
				cc.InTest = true
				if item.Test.Setup != nil && containsLexerPosition(item.Test.Setup.Span(), pos) {
					cc.InSetup = true
				}
				for _, assert := range item.Test.Asserts {
					if containsLexerPosition(assert.Span(), pos) {
						cc.InAssert = true
						// Capture the assert's query scope if present
						if assert.Query != nil {
							if assert.Query.QueryName != nil {
								cc.AssertQueryName = *assert.Query.QueryName
							} else if assert.Query.Inline != nil {
								cc.AssertQueryBody = *assert.Query.Inline
							}
						}
						// Check if inside a condition expression
						for _, cond := range assert.AllConditions() {
							if cond != nil && containsLexerPosition(cond.Span(), pos) {
								cc.InExpr = true
							}
						}
					}
				}
				// Check if inside a where clause expression in statements
				for _, stmt := range item.Test.Statements {
					if stmt != nil && stmt.Value != nil {
						// Check statement expression
						if stmt.Value.Expr != nil && containsLexerPosition(stmt.Value.Expr.Span(), pos) {
							cc.InExpr = true
						}
						// Check where clause
						if stmt.Value.Where != nil && containsLexerPosition(stmt.Value.Where.Span(), pos) {
							cc.InExpr = true
						}
					}
				}
			}
			if item.Group != nil && containsLexerPosition(item.Group.Span(), pos) {
				s.checkGroupContext(cc, item.Group, pos)
			}
		}
	}
}

// checkGroupContext recursively checks context within a group.
func (s *Server) checkGroupContext(cc *CompletionContext, group *scaf.Group, pos lexer.Position) {
	if group.Setup != nil && containsLexerPosition(group.Setup.Span(), pos) {
		cc.InSetup = true
	}
	for _, item := range group.Items {
		if item.Test != nil && containsLexerPosition(item.Test.Span(), pos) {
			cc.InTest = true
			if item.Test.Setup != nil && containsLexerPosition(item.Test.Setup.Span(), pos) {
				cc.InSetup = true
			}
			// Check asserts for expression context
			for _, assert := range item.Test.Asserts {
				if containsLexerPosition(assert.Span(), pos) {
					cc.InAssert = true
					// Capture the assert's query scope if present
					if assert.Query != nil {
						if assert.Query.QueryName != nil {
							cc.AssertQueryName = *assert.Query.QueryName
						} else if assert.Query.Inline != nil {
							cc.AssertQueryBody = *assert.Query.Inline
						}
					}
					for _, cond := range assert.AllConditions() {
						if cond != nil && containsLexerPosition(cond.Span(), pos) {
							cc.InExpr = true
						}
					}
				}
			}
			// Check statements for expression context
			for _, stmt := range item.Test.Statements {
				if stmt != nil && stmt.Value != nil {
					if stmt.Value.Expr != nil && containsLexerPosition(stmt.Value.Expr.Span(), pos) {
						cc.InExpr = true
					}
					if stmt.Value.Where != nil && containsLexerPosition(stmt.Value.Where.Span(), pos) {
						cc.InExpr = true
					}
				}
			}
		}
		if item.Group != nil && containsLexerPosition(item.Group.Span(), pos) {
			s.checkGroupContext(cc, item.Group, pos)
		}
	}
}

// determineCompletionKind is the single dispatch point.
// It looks at the token/text before cursor and decides what completion to offer.
// Uses both token-based and text-based detection for robustness.
func (s *Server) determineCompletionKind(cc *CompletionContext, doc *Document, af *analysis.AnalyzedFile, pos lexer.Position, textBeforeCursor string) CompletionKind {
	// Find token before cursor position
	prevToken := s.findPrevToken(doc, af, pos)
	trimmedBefore := strings.TrimSpace(textBeforeCursor)

	// === DISPATCH BASED ON TRIGGER CHARACTER AND CONTEXT ===

	// Case 0: Type annotation trigger - after colon in function parameter list
	// Check via AST first (more reliable), fall back to text-based detection
	if s.isInFunctionParamTypePosition(af, pos, prevToken, textBeforeCursor) {
		return CompletionKindTypeAnnotation
	}

	// Case 1: Dot trigger - module.function completion
	if cc.TriggerChar == "." {
		return s.handleDotCompletion(cc, doc, af, pos, prevToken)
	}

	// Case 2: Dollar trigger - parameter completion
	if cc.TriggerChar == "$" || strings.HasPrefix(cc.Prefix, "$") {
		if cc.InTest || cc.InSetup || cc.InAssert || cc.InScope != "" {
			return CompletionKindParameter
		}
	}

	// Case 3: After 'setup' keyword - import alias completion
	// Check both token and text (text is more reliable when file has parse errors)
	if prevToken != nil && prevToken.Type == scaf.TokenSetup {
		return CompletionKindImportAlias
	}
	if strings.HasSuffix(trimmedBefore, "setup") || strings.HasSuffix(trimmedBefore, "setup ") {
		return CompletionKindImportAlias
	}
	// Also check if we're typing after "setup " with a partial identifier
	if strings.Contains(trimmedBefore, "setup ") {
		// Extract what comes after "setup "
		idx := strings.LastIndex(trimmedBefore, "setup ")
		afterSetup := trimmedBefore[idx+6:] // len("setup ") = 6
		if afterSetup == "" || isIdentifierPrefix(afterSetup) {
			return CompletionKindImportAlias
		}
	}

	// Case 4: Inside expression (assert or where clause) - expression variable completion
	// This takes priority over general test body completion when inside parens
	if cc.InExpr || s.isInsideExpressionContext(textBeforeCursor) {
		cc.InExpr = true // Ensure it's set for the handler
		// Try to extract assert query name from text if not already set via AST
		if cc.AssertQueryName == "" && cc.AssertQueryBody == "" {
			cc.AssertQueryName, cc.AssertQueryBody = s.extractAssertQueryFromText(textBeforeCursor)
		}
		return CompletionKindExprVariable
	}

	// Case 5: Inside test body
	if cc.InTest {
		// After colon - value position, no completion
		if prevToken != nil && prevToken.Type == scaf.TokenColon {
			return CompletionKindNone
		}
		// Dollar prefix - parameters
		if strings.HasPrefix(cc.Prefix, "$") {
			return CompletionKindParameter
		}
		// At identifier position - return fields
		if cc.Prefix != "" || prevToken == nil || prevToken.Type == scaf.TokenLBrace ||
			prevToken.Type == scaf.TokenSemi || prevToken.Type == scaf.TokenString ||
			prevToken.Type == scaf.TokenNumber || prevToken.Type == scaf.TokenRBrace {
			// If typing an identifier (not $), offer return fields
			if !strings.HasPrefix(cc.Prefix, "$") {
				return CompletionKindReturnField
			}
		}
		return CompletionKindKeyword
	}

	// Case 6: Top level - query names or keywords
	if cc.InScope == "" {
		if startsWithUpper(cc.Prefix) {
			return CompletionKindQueryName
		}
		return CompletionKindKeyword
	}

	// Case 7: Inside scope but not in test - keywords
	return CompletionKindKeyword
}

// isIdentifierPrefix checks if s looks like an identifier being typed.
func isIdentifierPrefix(s string) bool {
	if s == "" {
		return true
	}
	for _, c := range s {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' {
			return false
		}
	}
	return true
}

// isInsideExpressionContext checks if the cursor appears to be inside a parenthesized
// expression context (e.g., after "assert (" or "where (").
// This is a text-based fallback when AST detection fails during typing.
func (s *Server) isInsideExpressionContext(textBeforeCursor string) bool {
	trimmed := strings.TrimSpace(textBeforeCursor)

	// Count open vs closed parens - if more open, we're inside an expr
	openParens := 0
	for _, c := range trimmed {
		if c == '(' {
			openParens++
		} else if c == ')' {
			openParens--
		}
	}

	if openParens <= 0 {
		return false
	}

	// Check if the unclosed paren is part of an assert or where context
	// Look for patterns like "assert (" or "where ("
	// Find the last unclosed open paren
	lastOpenParen := strings.LastIndex(trimmed, "(")
	if lastOpenParen < 0 {
		return false
	}

	// Get text before the open paren
	beforeParen := strings.TrimSpace(trimmed[:lastOpenParen])

	// Check if it ends with "assert" or "where"
	if strings.HasSuffix(beforeParen, "assert") || strings.HasSuffix(beforeParen, "where") {
		return true
	}

	// Check for assert block with query: "assert QueryName() { ("
	// Look for the pattern: assert ... { (
	if idx := strings.LastIndex(beforeParen, "{"); idx >= 0 {
		beforeBrace := strings.TrimSpace(beforeParen[:idx])
		// Check if "assert" appears before the brace (might have a query call in between)
		if strings.Contains(beforeBrace, "assert ") {
			return true
		}
	}

	// Also check for patterns like "assert (u." or "where (u.name >"
	// where we're deep in the expression
	// Look backwards for assert or where before the last unmatched paren
	for i := lastOpenParen - 1; i >= 0; i-- {
		if trimmed[i] == ')' {
			// Skip matched parens
			depth := 1
			for j := i - 1; j >= 0 && depth > 0; j-- {
				if trimmed[j] == ')' {
					depth++
				} else if trimmed[j] == '(' {
					depth--
				}
				i = j
			}
			continue
		}
		if trimmed[i] == '(' {
			// Found another open paren, check what's before it
			beforeThis := strings.TrimSpace(trimmed[:i])
			if strings.HasSuffix(beforeThis, "assert") || strings.HasSuffix(beforeThis, "where") {
				return true
			}
		}
	}

	return false
}

// extractAssertQueryFromText extracts the assert query name or inline body from text.
// Returns (queryName, queryBody) - one or both may be empty.
// Used as fallback when AST parsing fails during typing.
func (s *Server) extractAssertQueryFromText(textBeforeCursor string) (string, string) {
	trimmed := strings.TrimSpace(textBeforeCursor)

	// Look for pattern: assert QueryName(...) { (
	// or: assert `inline query` { (

	// Find the last "assert " in the text
	assertIdx := strings.LastIndex(trimmed, "assert ")
	if assertIdx < 0 {
		return "", ""
	}

	afterAssert := trimmed[assertIdx+7:] // len("assert ") = 7

	// Check for inline query (backtick)
	if strings.HasPrefix(afterAssert, "`") {
		// Find closing backtick
		endBacktick := strings.Index(afterAssert[1:], "`")
		if endBacktick >= 0 {
			inlineQuery := afterAssert[1 : endBacktick+1]
			return "", inlineQuery
		}
	}

	// Check for named query call: QueryName(...)
	// Find opening paren
	parenIdx := strings.Index(afterAssert, "(")
	if parenIdx > 0 {
		queryName := strings.TrimSpace(afterAssert[:parenIdx])
		// Validate it looks like an identifier (starts with letter, contains only alphanum/_)
		if len(queryName) > 0 && unicode.IsLetter(rune(queryName[0])) {
			valid := true
			for _, c := range queryName {
				if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' {
					valid = false
					break
				}
			}
			if valid {
				return queryName, ""
			}
		}
	}

	return "", ""
}

// handleDotCompletion handles the "module." completion case.
// Finds the identifier before the dot and checks if it's an import alias.
func (s *Server) handleDotCompletion(cc *CompletionContext, doc *Document, af *analysis.AnalyzedFile, pos lexer.Position, prevToken *lexer.Token) CompletionKind {
	if af == nil || af.Symbols == nil {
		return CompletionKindNone
	}

	// Find the identifier before the dot
	// prevToken might be the dot itself, or the identifier
	var identValue string

	if prevToken != nil {
		switch prevToken.Type {
		case scaf.TokenDot:
			// Token before cursor is the dot - find token before the dot
			tokenBeforeDot := s.findPrevToken(doc, af, prevToken.Pos)
			if tokenBeforeDot != nil && tokenBeforeDot.Type == scaf.TokenIdent {
				identValue = tokenBeforeDot.Value
			}
		case scaf.TokenIdent:
			// Token before cursor is an identifier (cursor might be on the dot)
			identValue = prevToken.Value
		}
	}

	// Fallback: extract from text before cursor
	// Note: pos is lexer.Position (1-indexed), convert to 0-indexed for text lookup
	if identValue == "" {
		identValue = s.extractIdentifierBeforeDot(doc.Content, pos.Line-1, pos.Column-1)
	}

	// Check if it's an import alias - for module.function completion in setup
	if identValue != "" {
		if _, ok := af.Symbols.Imports[identValue]; ok {
			cc.ModuleAlias = identValue
			cc.Prefix = "" // Reset prefix - we're after the dot
			return CompletionKindSetupFunction
		}
	}

	// Get text before cursor for text-based context detection (fallback when AST fails)
	content := doc.Content
	lines := strings.Split(content, "\n")
	var textBeforeCursor string
	if int(pos.Line-1) < len(lines) {
		lineText := lines[pos.Line-1]
		col := min(int(pos.Column-1), len(lineText))
		textBeforeCursor = lineText[:col]
	}

	// If we're in an expression context (assert or where), use expression variable completion
	// Check both AST-based (cc.InExpr/cc.InAssert) and text-based detection
	inExprContext := cc.InExpr || cc.InAssert || s.isInsideExpressionContext(textBeforeCursor)
	if inExprContext && identValue != "" {
		cc.Prefix = identValue + "."
		cc.InExpr = true
		// Try to extract assert query name from text if not already set
		if cc.AssertQueryName == "" && cc.AssertQueryBody == "" {
			cc.AssertQueryName, cc.AssertQueryBody = s.extractAssertQueryFromText(textBeforeCursor)
		}
		return CompletionKindExprVariable
	}

	// If we're in a test, dot could be for return field property access (e.g., "u." -> "u.name")
	// Set the prefix to include the identifier and dot so completeReturnFields can handle it
	if cc.InTest && identValue != "" {
		cc.Prefix = identValue + "."
		return CompletionKindReturnField
	}

	return CompletionKindNone
}

// findPrevToken finds the non-whitespace token immediately before pos.
func (s *Server) findPrevToken(doc *Document, af *analysis.AnalyzedFile, pos lexer.Position) *lexer.Token {
	// Try from current analysis
	if af != nil {
		if tok := analysis.PrevTokenAtPosition(af, pos); tok != nil {
			return tok
		}
	}
	// Try from last valid analysis
	if doc.LastValidAnalysis != nil {
		return analysis.PrevTokenAtPosition(doc.LastValidAnalysis, pos)
	}
	return nil
}

// extractIdentifierBeforeDot extracts the identifier before a dot from text.
// This is a fallback when token lookup fails (e.g., during typing).
func (s *Server) extractIdentifierBeforeDot(content string, line, col int) string {
	lines := strings.Split(content, "\n")
	if line >= len(lines) {
		return ""
	}
	lineText := lines[line]
	if col > len(lineText) {
		col = len(lineText)
	}

	// Find the dot position (should be at col-1 or nearby)
	dotPos := -1
	for i := col - 1; i >= 0; i-- {
		if lineText[i] == '.' {
			dotPos = i
			break
		}
		// Stop if we hit non-identifier chars (except dot)
		c := lineText[i]
		if !unicode.IsLetter(rune(c)) && !unicode.IsDigit(rune(c)) && c != '_' && c != '.' {
			break
		}
	}

	if dotPos < 0 {
		return ""
	}

	// Extract identifier before the dot
	end := dotPos
	start := end
	for i := end - 1; i >= 0; i-- {
		c := rune(lineText[i])
		if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' {
			start = i
		} else {
			break
		}
	}

	if start < end {
		return lineText[start:end]
	}
	return ""
}

// completeQueryNames returns completion items for query names.
func (s *Server) completeQueryNames(doc *Document, _ *CompletionContext) []protocol.CompletionItem {
	af := s.getSymbolsAnalysis(doc)
	if af == nil || af.Symbols == nil {
		return nil
	}

	items := make([]protocol.CompletionItem, 0, len(af.Symbols.Queries))
	for name, q := range af.Symbols.Queries {
		item := protocol.CompletionItem{
			Label:            name,
			Kind:             protocol.CompletionItemKindFunction,
			Detail:           "query",
			InsertText:       name + " {\n\t$0\n}",
			InsertTextFormat: protocol.InsertTextFormatSnippet,
		}

		if q.Body != "" {
			preview := strings.TrimSpace(q.Body)
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			item.Documentation = &protocol.MarkupContent{
				Kind:  protocol.Markdown,
				Value: s.markdownCodeBlock(preview),
			}
		}
		items = append(items, item)
	}
	return items
}

// keywordSnippet defines a keyword completion with its snippet.
type keywordSnippet struct {
	label      string
	detail     string
	snippet    string
	doc        string
}

// completeKeywords returns keyword completions based on context.
func (s *Server) completeKeywords(cc *CompletionContext) []protocol.CompletionItem {
	var snippets []keywordSnippet

	if cc.InScope == "" {
		// Top level keywords with snippets
		snippets = []keywordSnippet{
			{
				label:   "query",
				detail:  "Define a new query",
				snippet: "query ${1:QueryName} `${2:MATCH (n) RETURN n}`",
				doc:     "Defines a named database query that can be tested.",
			},
			{
				label:   "import",
				detail:  "Import a module",
				snippet: "import ${1:alias} \"${2:./path/to/module}\"",
				doc:     "Imports queries and setup functions from another .scaf file.",
			},
			{
				label:   "setup",
				detail:  "Global setup",
				snippet: "setup ${1|`query`,$module,$module.Query()|}",
				doc:     "Setup to run before all tests in this file.",
			},
			{
				label:   "teardown",
				detail:  "Global teardown",
				snippet: "teardown `${1:MATCH (n) DETACH DELETE n}`",
				doc:     "Teardown to run after all tests in this file.",
			},
		}
	} else if cc.InTest {
		// Inside test
		snippets = []keywordSnippet{
			{
				label:   "setup",
				detail:  "Test-specific setup",
				snippet: "setup ${1|$module.Query(),$module|}",
				doc:     "Setup to run before this specific test.",
			},
			{
				label:   "assert",
				detail:  "Add assertion query",
				snippet: "assert ${1:QueryName}(${2:params}) {\n\t($0)\n}",
				doc:     "Assert the results of another query after the main query runs. Conditions must be in parentheses.",
			},
		}
	} else {
		// Inside scope, not in test
		snippets = []keywordSnippet{
			{
				label:   "test",
				detail:  "Define a test case",
				snippet: "test \"${1:test name}\" {\n\t${2:\\$param: value}\n\t${3:field: expected}\n}",
				doc:     "Defines a test case with inputs and expected outputs.",
			},
			{
				label:   "group",
				detail:  "Group related tests",
				snippet: "group \"${1:group name}\" {\n\t${0}\n}",
				doc:     "Groups related test cases together. Can have its own setup/teardown.",
			},
			{
				label:   "setup",
				detail:  "Scope-level setup",
				snippet: "setup ${1|$module.Query(),$module|}",
				doc:     "Setup to run before all tests in this scope.",
			},
		}
	}

	items := make([]protocol.CompletionItem, 0, len(snippets))
	for _, ks := range snippets {
		item := protocol.CompletionItem{
			Label:            ks.label,
			Kind:             protocol.CompletionItemKindKeyword,
			Detail:           ks.detail,
			InsertText:       ks.snippet,
			InsertTextFormat: protocol.InsertTextFormatSnippet,
		}
		if ks.doc != "" {
			item.Documentation = &protocol.MarkupContent{
				Kind:  protocol.Markdown,
				Value: ks.doc,
			}
		}
		items = append(items, item)
	}
	return items
}

// completeParameters returns parameter completions from the query in scope.
func (s *Server) completeParameters(doc *Document, cc *CompletionContext) []protocol.CompletionItem {
	af := s.getSymbolsAnalysis(doc)
	if af == nil || af.Symbols == nil || cc.InScope == "" {
		return nil
	}

	q, ok := af.Symbols.Queries[cc.InScope]
	if !ok {
		return nil
	}

	items := make([]protocol.CompletionItem, 0, len(q.Params))
	for _, param := range q.Params {
		items = append(items, protocol.CompletionItem{
			Label:      "$" + param,
			Kind:       protocol.CompletionItemKindVariable,
			Detail:     "parameter",
			InsertText: "$" + param + ": ",
		})
	}
	return items
}

// completeReturnFields returns return field completions from the query in scope.
func (s *Server) completeReturnFields(doc *Document, cc *CompletionContext) []protocol.CompletionItem {
	af := s.getSymbolsAnalysis(doc)
	if af == nil || af.Symbols == nil || cc.InScope == "" {
		return nil
	}

	q, ok := af.Symbols.Queries[cc.InScope]
	if !ok || q.Body == "" {
		return nil
	}

	if s.queryAnalyzer == nil {
		return nil
	}

	metadata, err := s.queryAnalyzer.AnalyzeQuery(q.Body)
	if err != nil {
		s.logger.Debug("Failed to analyze query for return fields", zap.Error(err))
		return nil
	}

	// Check if the prefix contains a dot (e.g., "u." or "u.na")
	// If so, we need to match the prefix and show only the property part
	var prefixBase string // e.g., "u" from "u." or "u.na"
	var prefixProp string // e.g., "" from "u." or "na" from "u.na"
	if dotIdx := strings.LastIndex(cc.Prefix, "."); dotIdx >= 0 {
		prefixBase = cc.Prefix[:dotIdx]
		prefixProp = cc.Prefix[dotIdx+1:]
	}

	items := make([]protocol.CompletionItem, 0, len(metadata.Returns))
	for _, ret := range metadata.Returns {
		// Use the full expression (e.g., "u.name") as the base
		// If there's an alias, use that instead (it's the actual column name)
		fullExpr := ret.Expression
		if ret.Alias != "" {
			fullExpr = ret.Alias
		}

		// Determine label and insertText based on whether user typed a prefix with dot
		var label, insertText string
		if prefixBase != "" {
			// User typed something like "u." - check if this expression starts with "u."
			if !strings.HasPrefix(fullExpr, prefixBase+".") {
				continue // This field doesn't match the prefix base
			}
			// Extract just the property part after the prefix
			propPart := fullExpr[len(prefixBase)+1:]
			// Filter by property prefix if any
			if prefixProp != "" && !strings.HasPrefix(strings.ToLower(propPart), strings.ToLower(prefixProp)) {
				continue
			}
			label = propPart
			insertText = propPart + ": "
		} else {
			// No dot prefix - show full expression
			label = fullExpr
			insertText = fullExpr + ": "
		}

		item := protocol.CompletionItem{
			Label:      label,
			Kind:       protocol.CompletionItemKindField,
			Detail:     "return field",
			InsertText: insertText,
		}
		if ret.Alias != "" && ret.Expression != ret.Alias {
			item.Documentation = &protocol.MarkupContent{
				Kind:  protocol.Markdown,
				Value: "Expression: `" + ret.Expression + "`",
			}
		}
		if ret.IsAggregate {
			item.Detail = "aggregate field"
		}
		items = append(items, item)
	}
	return items
}

// completeExprVariables returns expression variable completions for assert/where contexts.
// Unlike completeReturnFields, this doesn't add ": " suffix since these are expression variables.
// When inside an assert block with a query (e.g., assert SomeQuery() { ... }), uses that query's
// returns instead of the parent scope's query.
func (s *Server) completeExprVariables(doc *Document, cc *CompletionContext) []protocol.CompletionItem {
	af := s.getSymbolsAnalysis(doc)
	if s.queryAnalyzer == nil {
		return s.completeExprBuiltinFunctions(cc.Prefix)
	}

	// Determine which query to use for variable completions:
	// 1. If inside an assert with an inline query, analyze that inline query
	// 2. If inside an assert with a named query reference, use that query
	// 3. Otherwise use the parent scope's query
	var metadata *scaf.QueryMetadata
	var err error

	if cc.AssertQueryBody != "" {
		// Inline assert query: assert `MATCH (c:Comment) RETURN c` { (c.id > 0) }
		metadata, err = s.queryAnalyzer.AnalyzeQuery(cc.AssertQueryBody)
		if err != nil {
			s.logger.Debug("Failed to analyze inline assert query", zap.Error(err))
		}
	} else if cc.AssertQueryName != "" {
		// Named assert query: assert SomeQuery() { (c.id > 0) }
		if af != nil && af.Symbols != nil {
			if q, ok := af.Symbols.Queries[cc.AssertQueryName]; ok && q.Body != "" {
				metadata, err = s.queryAnalyzer.AnalyzeQuery(q.Body)
				if err != nil {
					s.logger.Debug("Failed to analyze assert query", zap.Error(err), zap.String("query", cc.AssertQueryName))
				}
			}
		}
	} else if cc.InScope != "" && af != nil && af.Symbols != nil {
		// Fall back to parent scope query
		if q, ok := af.Symbols.Queries[cc.InScope]; ok && q.Body != "" {
			metadata, err = s.queryAnalyzer.AnalyzeQuery(q.Body)
			if err != nil {
				s.logger.Debug("Failed to analyze scope query", zap.Error(err))
			}
		}
	}

	if metadata == nil {
		return s.completeExprBuiltinFunctions(cc.Prefix)
	}

	// Check if the prefix contains a dot (e.g., "u." or "u.na")
	// If so, show property completions after the dot
	var prefixBase string // e.g., "u" from "u." or "u.na"
	var prefixProp string // e.g., "" from "u." or "na" from "u.na"
	if dotIdx := strings.LastIndex(cc.Prefix, "."); dotIdx >= 0 {
		prefixBase = cc.Prefix[:dotIdx]
		prefixProp = cc.Prefix[dotIdx+1:]
	}

	// Track entity prefixes (e.g., "u" from "u.name") for offering them as completions
	entityPrefixes := make(map[string]bool)
	items := make([]protocol.CompletionItem, 0, len(metadata.Returns)+len(exprBuiltinFunctions)+4)

	for _, ret := range metadata.Returns {
		// Use the full expression (e.g., "u.name") or alias
		fullExpr := ret.Expression
		if ret.Alias != "" {
			fullExpr = ret.Alias
		}

		// Track entity prefix
		if parts := strings.SplitN(ret.Expression, ".", 2); len(parts) == 2 {
			entityPrefixes[parts[0]] = true
		}

		// If prefix contains a dot, filter and show only properties after it
		if prefixBase != "" {
			// Check if this expression starts with the prefix base
			if !strings.HasPrefix(fullExpr, prefixBase+".") {
				continue
			}
			// Extract just the property part
			propPart := fullExpr[len(prefixBase)+1:]
			// Filter by property prefix if any
			if prefixProp != "" && !strings.HasPrefix(strings.ToLower(propPart), strings.ToLower(prefixProp)) {
				continue
			}

			item := protocol.CompletionItem{
				Label:      propPart,
				Kind:       protocol.CompletionItemKindField,
				Detail:     "expression variable",
				InsertText: propPart, // No ": " suffix for expression variables
			}
			if ret.Type != nil {
				item.Detail = ret.Type.Name + " variable"
			}
			items = append(items, item)
		} else {
			// No dot prefix - offer full expressions
			item := protocol.CompletionItem{
				Label:      fullExpr,
				Kind:       protocol.CompletionItemKindField,
				Detail:     "expression variable",
				InsertText: fullExpr, // No ": " suffix for expression variables
			}
			if ret.Alias != "" && ret.Expression != ret.Alias {
				item.Documentation = &protocol.MarkupContent{
					Kind:  protocol.Markdown,
					Value: "Expression: `" + ret.Expression + "`",
				}
			}
			if ret.Type != nil {
				item.Detail = ret.Type.Name + " variable"
			}
			items = append(items, item)
		}
	}

	// If no dot prefix, also offer entity prefixes as completions (e.g., "u")
	if prefixBase == "" {
		for prefix := range entityPrefixes {
			items = append(items, protocol.CompletionItem{
				Label:      prefix,
				Kind:       protocol.CompletionItemKindVariable,
				Detail:     "entity",
				InsertText: prefix,
			})
		}

		// Add built-in expr-lang functions
		items = append(items, s.completeExprBuiltinFunctions(cc.Prefix)...)
	}

	return items
}

// exprBuiltinFunction represents a built-in expr-lang function.
type exprBuiltinFunction struct {
	Name       string
	Signature  string
	Doc        string
	ReturnType string
}

// exprBuiltinFunctions contains commonly used expr-lang built-in functions.
// See: https://expr-lang.org/docs/language-definition#built-in-functions
var exprBuiltinFunctions = []exprBuiltinFunction{
	// Collection functions
	{Name: "len", Signature: "len(v)", Doc: "Returns the length of an array, map, or string.", ReturnType: "int"},
	{Name: "all", Signature: "all(array, predicate)", Doc: "Returns true if all elements satisfy the predicate.", ReturnType: "bool"},
	{Name: "any", Signature: "any(array, predicate)", Doc: "Returns true if any element satisfies the predicate.", ReturnType: "bool"},
	{Name: "none", Signature: "none(array, predicate)", Doc: "Returns true if no elements satisfy the predicate.", ReturnType: "bool"},
	{Name: "one", Signature: "one(array, predicate)", Doc: "Returns true if exactly one element satisfies the predicate.", ReturnType: "bool"},
	{Name: "filter", Signature: "filter(array, predicate)", Doc: "Returns elements that satisfy the predicate.", ReturnType: "array"},
	{Name: "map", Signature: "map(array, mapper)", Doc: "Transforms each element using the mapper function.", ReturnType: "array"},
	{Name: "count", Signature: "count(array, predicate)", Doc: "Counts elements that satisfy the predicate.", ReturnType: "int"},
	{Name: "find", Signature: "find(array, predicate)", Doc: "Returns the first element that satisfies the predicate.", ReturnType: "any"},
	{Name: "findIndex", Signature: "findIndex(array, predicate)", Doc: "Returns index of first matching element, or -1.", ReturnType: "int"},
	{Name: "findLast", Signature: "findLast(array, predicate)", Doc: "Returns the last element that satisfies the predicate.", ReturnType: "any"},
	{Name: "findLastIndex", Signature: "findLastIndex(array, predicate)", Doc: "Returns index of last matching element, or -1.", ReturnType: "int"},
	{Name: "groupBy", Signature: "groupBy(array, key)", Doc: "Groups elements by the key function.", ReturnType: "map"},
	{Name: "sortBy", Signature: "sortBy(array, key)", Doc: "Sorts elements by the key function.", ReturnType: "array"},
	{Name: "reduce", Signature: "reduce(array, reducer, initial)", Doc: "Reduces array to single value using reducer.", ReturnType: "any"},
	{Name: "sum", Signature: "sum(array)", Doc: "Returns sum of numeric array elements.", ReturnType: "number"},
	{Name: "mean", Signature: "mean(array)", Doc: "Returns arithmetic mean of array elements.", ReturnType: "number"},
	{Name: "median", Signature: "median(array)", Doc: "Returns median value of array elements.", ReturnType: "number"},
	{Name: "min", Signature: "min(a, b) or min(array)", Doc: "Returns minimum value.", ReturnType: "any"},
	{Name: "max", Signature: "max(a, b) or max(array)", Doc: "Returns maximum value.", ReturnType: "any"},
	{Name: "first", Signature: "first(array)", Doc: "Returns first element of array.", ReturnType: "any"},
	{Name: "last", Signature: "last(array)", Doc: "Returns last element of array.", ReturnType: "any"},
	{Name: "take", Signature: "take(array, n)", Doc: "Returns first n elements.", ReturnType: "array"},
	{Name: "reverse", Signature: "reverse(array)", Doc: "Returns reversed array.", ReturnType: "array"},
	{Name: "flatten", Signature: "flatten(array)", Doc: "Flattens nested arrays one level.", ReturnType: "array"},
	{Name: "unique", Signature: "unique(array)", Doc: "Returns array with duplicates removed.", ReturnType: "array"},

	// String functions
	{Name: "contains", Signature: "contains(str, substr)", Doc: "Returns true if str contains substr.", ReturnType: "bool"},
	{Name: "startsWith", Signature: "startsWith(str, prefix)", Doc: "Returns true if str starts with prefix.", ReturnType: "bool"},
	{Name: "endsWith", Signature: "endsWith(str, suffix)", Doc: "Returns true if str ends with suffix.", ReturnType: "bool"},
	{Name: "lower", Signature: "lower(str)", Doc: "Converts string to lowercase.", ReturnType: "string"},
	{Name: "upper", Signature: "upper(str)", Doc: "Converts string to uppercase.", ReturnType: "string"},
	{Name: "trim", Signature: "trim(str)", Doc: "Removes leading/trailing whitespace.", ReturnType: "string"},
	{Name: "trimPrefix", Signature: "trimPrefix(str, prefix)", Doc: "Removes prefix from string if present.", ReturnType: "string"},
	{Name: "trimSuffix", Signature: "trimSuffix(str, suffix)", Doc: "Removes suffix from string if present.", ReturnType: "string"},
	{Name: "split", Signature: "split(str, sep)", Doc: "Splits string by separator.", ReturnType: "array"},
	{Name: "replace", Signature: "replace(str, old, new)", Doc: "Replaces all occurrences of old with new.", ReturnType: "string"},
	{Name: "repeat", Signature: "repeat(str, n)", Doc: "Repeats string n times.", ReturnType: "string"},
	{Name: "join", Signature: "join(array, sep)", Doc: "Joins array elements with separator.", ReturnType: "string"},
	{Name: "indexOf", Signature: "indexOf(str, substr)", Doc: "Returns index of substr, or -1.", ReturnType: "int"},
	{Name: "lastIndexOf", Signature: "lastIndexOf(str, substr)", Doc: "Returns last index of substr, or -1.", ReturnType: "int"},
	{Name: "hasPrefix", Signature: "hasPrefix(str, prefix)", Doc: "Returns true if str starts with prefix.", ReturnType: "bool"},
	{Name: "hasSuffix", Signature: "hasSuffix(str, suffix)", Doc: "Returns true if str ends with suffix.", ReturnType: "bool"},

	// Type functions
	{Name: "type", Signature: "type(v)", Doc: "Returns the type of v as string.", ReturnType: "string"},
	{Name: "int", Signature: "int(v)", Doc: "Converts value to integer.", ReturnType: "int"},
	{Name: "float", Signature: "float(v)", Doc: "Converts value to float.", ReturnType: "float"},
	{Name: "string", Signature: "string(v)", Doc: "Converts value to string.", ReturnType: "string"},
	{Name: "toJSON", Signature: "toJSON(v)", Doc: "Converts value to JSON string.", ReturnType: "string"},
	{Name: "fromJSON", Signature: "fromJSON(str)", Doc: "Parses JSON string to value.", ReturnType: "any"},

	// Utility functions
	{Name: "keys", Signature: "keys(map)", Doc: "Returns array of map keys.", ReturnType: "array"},
	{Name: "values", Signature: "values(map)", Doc: "Returns array of map values.", ReturnType: "array"},
	{Name: "abs", Signature: "abs(n)", Doc: "Returns absolute value.", ReturnType: "number"},
	{Name: "ceil", Signature: "ceil(n)", Doc: "Rounds up to nearest integer.", ReturnType: "int"},
	{Name: "floor", Signature: "floor(n)", Doc: "Rounds down to nearest integer.", ReturnType: "int"},
	{Name: "round", Signature: "round(n)", Doc: "Rounds to nearest integer.", ReturnType: "int"},
	{Name: "now", Signature: "now()", Doc: "Returns current time.", ReturnType: "time"},
	{Name: "duration", Signature: "duration(str)", Doc: "Parses duration string (e.g., '1h30m').", ReturnType: "duration"},
	{Name: "date", Signature: "date(str)", Doc: "Parses date string.", ReturnType: "time"},
}

// completeExprBuiltinFunctions returns completion items for expr-lang built-in functions.
func (s *Server) completeExprBuiltinFunctions(prefix string) []protocol.CompletionItem {
	items := make([]protocol.CompletionItem, 0, len(exprBuiltinFunctions))
	prefixLower := strings.ToLower(prefix)

	for _, fn := range exprBuiltinFunctions {
		// Filter by prefix if provided
		if prefixLower != "" && !strings.HasPrefix(strings.ToLower(fn.Name), prefixLower) {
			continue
		}

		item := protocol.CompletionItem{
			Label:            fn.Name,
			Kind:             protocol.CompletionItemKindFunction,
			Detail:           fn.Signature,
			InsertText:       fn.Name + "(${1})",
			InsertTextFormat: protocol.InsertTextFormatSnippet,
			Documentation: &protocol.MarkupContent{
				Kind:  protocol.Markdown,
				Value: fn.Doc + "\n\n**Returns:** `" + fn.ReturnType + "`",
			},
		}
		items = append(items, item)
	}

	return items
}

// completeImportAliases returns import alias completions.
func (s *Server) completeImportAliases(doc *Document, _ *CompletionContext) []protocol.CompletionItem {
	af := s.getSymbolsAnalysis(doc)
	if af == nil || af.Symbols == nil {
		return nil
	}

	items := make([]protocol.CompletionItem, 0, len(af.Symbols.Imports))
	for alias, imp := range af.Symbols.Imports {
		items = append(items, protocol.CompletionItem{
			Label:  alias,
			Kind:   protocol.CompletionItemKindModule,
			Detail: imp.Path,
		})
	}
	return items
}

// completeSetupFunctions returns setup function completions from imported module.
func (s *Server) completeSetupFunctions(doc *Document, cc *CompletionContext) []protocol.CompletionItem {
	start := time.Now()
	s.logger.Debug("completeSetupFunctions: starting",
		zap.String("moduleAlias", cc.ModuleAlias))
	defer func() {
		s.logger.Debug("completeSetupFunctions: finished",
			zap.Duration("elapsed", time.Since(start)))
	}()

	if cc.ModuleAlias == "" {
		s.logger.Debug("completeSetupFunctions: no module alias")
		return nil
	}

	af := s.getSymbolsAnalysis(doc)
	if af == nil || af.Symbols == nil {
		s.logger.Debug("completeSetupFunctions: no analysis or symbols")
		return nil
	}

	if s.fileLoader == nil {
		s.logger.Debug("completeSetupFunctions: FileLoader not available")
		return nil
	}

	imp, ok := af.Symbols.Imports[cc.ModuleAlias]
	if !ok {
		s.logger.Debug("completeSetupFunctions: import not found",
			zap.String("alias", cc.ModuleAlias),
			zap.Int("availableImports", len(af.Symbols.Imports)))
		return nil
	}

	// Resolve and load the imported file
	docPath := URIToPath(doc.URI)
	s.logger.Debug("completeSetupFunctions: resolving import",
		zap.String("docPath", docPath),
		zap.String("importPath", imp.Path))

	importedPath := s.fileLoader.ResolveImportPath(docPath, imp.Path)

	s.logger.Debug("completeSetupFunctions: loading imported file",
		zap.String("resolvedPath", importedPath))

	importedFile, err := s.fileLoader.LoadAndAnalyze(importedPath)
	if err != nil {
		s.logger.Debug("completeSetupFunctions: failed to load imported file",
			zap.String("path", importedPath),
			zap.Error(err))
		return nil
	}

	if importedFile.Symbols == nil {
		s.logger.Debug("completeSetupFunctions: imported file has no symbols")
		return nil
	}

	s.logger.Debug("completeSetupFunctions: building completions",
		zap.Int("queryCount", len(importedFile.Symbols.Queries)))

	var items []protocol.CompletionItem

	// Add queries from imported file as setup functions
	for name, q := range importedFile.Symbols.Queries {
		item := protocol.CompletionItem{
			Label:  name,
			Kind:   protocol.CompletionItemKindFunction,
			Detail: "query from " + cc.ModuleAlias,
		}

		if q.Body != "" {
			preview := strings.TrimSpace(q.Body)
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			item.Documentation = &protocol.MarkupContent{
				Kind:  protocol.Markdown,
				Value: s.markdownCodeBlock(preview),
			}
		}

		// Build snippet with parameter placeholders
		if len(q.Params) > 0 {
			var sb strings.Builder
			sb.WriteString(name)
			sb.WriteByte('(')
			for i, p := range q.Params {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteByte('$')
				sb.WriteString(p)
				sb.WriteString(": ${")
				sb.WriteByte('1' + byte(i))
				sb.WriteByte('}')
			}
			sb.WriteByte(')')
			item.InsertText = sb.String()
			item.InsertTextFormat = protocol.InsertTextFormatSnippet
		} else {
			item.InsertText = name + "()"
		}

		items = append(items, item)
	}

	return items
}

// getSymbolsAnalysis returns the best analysis for symbol lookup.
func (s *Server) getSymbolsAnalysis(doc *Document) *analysis.AnalyzedFile {
	if doc.Analysis != nil && doc.Analysis.ParseError == nil {
		return doc.Analysis
	}
	if doc.LastValidAnalysis != nil {
		return doc.LastValidAnalysis
	}
	return doc.Analysis
}

// filterByPrefix filters completion items by prefix.
func filterByPrefix(items []protocol.CompletionItem, prefix string) []protocol.CompletionItem {
	if prefix == "" {
		return items
	}

	prefix = strings.ToLower(prefix)
	filtered := make([]protocol.CompletionItem, 0, len(items))
	for _, item := range items {
		if strings.HasPrefix(strings.ToLower(item.Label), prefix) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// extractPrefix extracts the identifier prefix being typed.
func extractPrefix(text string) string {
	end := len(text)
	start := end

	for i := end - 1; i >= 0; i-- {
		c := rune(text[i])
		// Include dots to support "u.name" style prefixes for return field completion
		if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' || c == '$' || c == '.' {
			start = i
		} else {
			break
		}
	}
	return text[start:end]
}

// startsWithUpper checks if a string starts with an uppercase letter.
func startsWithUpper(s string) bool {
	if s == "" {
		return false
	}
	if s[0] == '$' && len(s) > 1 {
		s = s[1:]
	}
	return unicode.IsUpper(rune(s[0]))
}

// containsLexerPosition checks if a span contains a lexer position.
func containsLexerPosition(span scaf.Span, pos lexer.Position) bool {
	if pos.Line < span.Start.Line || (pos.Line == span.Start.Line && pos.Column < span.Start.Column) {
		return false
	}
	if pos.Line > span.End.Line || (pos.Line == span.End.Line && pos.Column > span.End.Column) {
		return false
	}
	return true
}

// markdownCodeBlock wraps code in a markdown code block.
func (s *Server) markdownCodeBlock(code string) string {
	lang := scaf.MarkdownLanguage(s.dialectName)
	return "```" + lang + "\n" + code + "\n```"
}

// isInFunctionParamTypePosition checks if the cursor is in a type annotation position
// within a function parameter list. Uses AST when available, falls back to text-based detection.
func (s *Server) isInFunctionParamTypePosition(af *analysis.AnalyzedFile, pos lexer.Position, prevToken *lexer.Token, textBeforeCursor string) bool {
	// Method 1: Check via AST - are we inside a Function node but before the body?
	if af != nil && af.Suite != nil {
		for _, fn := range af.Suite.Functions {
			if fn == nil {
				continue
			}
			// Check if position is within the function's span
			if !analysis.ContainsPosition(fn.Span(), pos) {
				continue
			}
			// We're inside the function. Check if we're in the parameter list (before the body).
			// The body starts with a backtick, so if we're before that, we're in params area.
			// Check if prev token is a colon - that means we're in type position
			if prevToken != nil && prevToken.Type == scaf.TokenColon {
				return true
			}
			// Also check if we're after a type (for partial typing)
			// If we're inside a FnParam and after the colon, offer completions
			for _, param := range fn.Params {
				if param == nil {
					continue
				}
				// If the param has a type, check if we're inside that type's span
				if param.Type != nil && analysis.ContainsPosition(param.Type.Span(), pos) {
					return true
				}
				// If param has no type but we're after a colon on the same param
				// This is when user typed "param:" but no type yet
				paramSpan := param.Span()
				if pos.Line == paramSpan.End.Line && pos.Column > paramSpan.End.Column {
					// Position is after the param - check if there's a colon between
					if prevToken != nil && prevToken.Type == scaf.TokenColon {
						return true
					}
				}
			}
		}
	}

	// Method 2: Fall back to text-based detection for incomplete parses
	return s.isInFunctionParamTypePositionText(textBeforeCursor)
}

// isInFunctionParamTypePositionText is the text-based fallback for detecting type position.
func (s *Server) isInFunctionParamTypePositionText(textBeforeCursor string) bool {
	// Look for "fn " followed by function name and opening paren
	fnIdx := strings.LastIndex(textBeforeCursor, "fn ")
	if fnIdx < 0 {
		return false
	}

	afterFn := textBeforeCursor[fnIdx+3:] // after "fn "

	// Find the opening paren
	parenIdx := strings.Index(afterFn, "(")
	if parenIdx < 0 {
		return false
	}

	// Check we haven't closed the paren yet (no matching close paren after open)
	afterParen := afterFn[parenIdx+1:]
	openCount := 1
	for _, c := range afterParen {
		if c == '(' {
			openCount++
		} else if c == ')' {
			openCount--
			if openCount == 0 {
				return false // Paren is closed, not in param list
			}
		}
	}

	// We're inside the param list. Check if we're after a colon (type position)
	// Find the last colon that's not inside brackets
	lastColon := -1
	bracketDepth := 0
	braceDepth := 0
	for i, c := range afterParen {
		switch c {
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		case '{':
			braceDepth++
		case '}':
			braceDepth--
		case ':':
			if bracketDepth == 0 && braceDepth == 0 {
				lastColon = i
			}
		case ',':
			// Reset - we're on a new parameter
			if bracketDepth == 0 && braceDepth == 0 {
				lastColon = -1
			}
		}
	}

	return lastColon >= 0
}

// completeTypeAnnotations returns type annotation completions.
func (s *Server) completeTypeAnnotations(cc *CompletionContext) []protocol.CompletionItem {
	// Define available types with their documentation
	types := []struct {
		name string
		doc  string
	}{
		{"string", "Text value"},
		{"int", "Integer number"},
		{"int32", "32-bit integer"},
		{"int64", "64-bit integer"},
		{"float32", "32-bit floating point"},
		{"float64", "64-bit floating point"},
		{"bool", "Boolean (true/false)"},
		{"any", "Any type (no type checking)"},
	}

	items := make([]protocol.CompletionItem, 0, len(types)+3)

	// Add primitive types
	for _, t := range types {
		items = append(items, protocol.CompletionItem{
			Label:  t.name,
			Kind:   protocol.CompletionItemKindTypeParameter,
			Detail: "type",
			Documentation: &protocol.MarkupContent{
				Kind:  protocol.Markdown,
				Value: t.doc,
			},
		})
	}

	// Add composite type snippets
	items = append(items,
		protocol.CompletionItem{
			Label:            "[...]",
			Kind:             protocol.CompletionItemKindTypeParameter,
			Detail:           "array type",
			InsertText:       "[${1:string}]",
			InsertTextFormat: protocol.InsertTextFormatSnippet,
			Documentation: &protocol.MarkupContent{
				Kind:  protocol.Markdown,
				Value: "Array/slice type, e.g. `[string]` for `[]string`",
			},
		},
		protocol.CompletionItem{
			Label:            "{...: ...}",
			Kind:             protocol.CompletionItemKindTypeParameter,
			Detail:           "map type",
			InsertText:       "{${1:string}: ${2:any}}",
			InsertTextFormat: protocol.InsertTextFormatSnippet,
			Documentation: &protocol.MarkupContent{
				Kind:  protocol.Markdown,
				Value: "Map type, e.g. `{string: int}` for `map[string]int`",
			},
		},
	)

	return items
}
