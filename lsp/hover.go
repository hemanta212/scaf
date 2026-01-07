package lsp

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
	"go.lsp.dev/protocol"
	"go.uber.org/zap"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
)

// markdownQueryBlock wraps a query body in a markdown code block with the appropriate language.
func (s *Server) markdownQueryBlock(queryBody string) string {
	lang := scaf.MarkdownLanguage(s.dialectName)
	return "```" + lang + "\n" + strings.TrimSpace(queryBody) + "\n```"
}

// Hover handles textDocument/hover requests.
func (s *Server) Hover(_ context.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	defer s.traceHandler("Hover")()
	s.logger.Debug("Hover",
		zap.String("uri", string(params.TextDocument.URI)),
		zap.Uint32("line", params.Position.Line),
		zap.Uint32("character", params.Position.Character))

	doc, ok := s.getDocument(params.TextDocument.URI)
	if !ok {
		return nil, nil //nolint:nilnil
	}

	// Check if we're inside a query body (backtick string)
	// If so, delegate to the dialect's LSP implementation
	if qbc := s.getQueryBodyContext(doc, params.Position); qbc != nil {
		if dialectLSP := s.getDialectLSP(); dialectLSP != nil {
			queryCtx := s.buildQueryLSPContext(doc, qbc, "")
			hover := dialectLSP.Hover(qbc.Query, qbc.Offset, queryCtx)
			return s.convertDialectHover(hover, qbc), nil
		}
	}

	if doc.Analysis == nil || doc.Analysis.Suite == nil {
		return nil, nil //nolint:nilnil
	}

	pos := analysis.PositionToLexer(params.Position.Line, params.Position.Character)

	// Get token context for precise information
	tokenCtx := analysis.GetTokenContext(doc.Analysis, pos)

	// Find the node at this position
	node := analysis.NodeAtPosition(doc.Analysis, pos)
	if node == nil {
		return nil, nil //nolint:nilnil
	}

	// Generate hover content based on node type
	content, rng := s.hoverContent(doc, doc.Analysis, node, tokenCtx, pos)
	if content == "" {
		return nil, nil //nolint:nilnil
	}

	return &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.Markdown,
			Value: content,
		},
		Range: rng,
	}, nil
}

// hoverContent generates hover markdown for a node.
func (s *Server) hoverContent(doc *Document, f *analysis.AnalyzedFile, node scaf.Node, tokenCtx *analysis.TokenContext, pos lexer.Position) (string, *protocol.Range) {
	switch n := node.(type) {
	case *scaf.Query:
		return s.hoverQuery(n), rangePtr(spanToRange(n.Span()))

	case *scaf.Import:
		return s.hoverImport(n), rangePtr(spanToRange(n.Span()))

	case *scaf.QueryScope:
		// When hovering over a scope, show info about the referenced query
		if q, ok := f.Symbols.Queries[n.FunctionName]; ok {
			return s.hoverQueryScope(n, q), rangePtr(spanToRange(n.Span()))
		}

		return fmt.Sprintf("**Query Scope:** `%s` (undefined)", n.FunctionName), rangePtr(spanToRange(n.Span()))

	case *scaf.Test:
		return s.hoverTest(n), rangePtr(spanToRange(n.Span()))

	case *scaf.Group:
		return s.hoverGroup(n), rangePtr(spanToRange(n.Span()))

	case *scaf.Statement:
		return s.hoverStatement(f, n, tokenCtx, pos)

	case *scaf.SetupCall:
		return s.hoverSetupCall(doc, f, n, tokenCtx), rangePtr(spanToRange(n.Span()))

	case *scaf.SetupClause:
		return s.hoverSetupClause(doc, f, n, tokenCtx), rangePtr(spanToRange(n.Span()))

	case *scaf.SetupItem:
		return s.hoverSetupItem(doc, f, n, tokenCtx), rangePtr(spanToRange(n.Span()))

	case *scaf.AssertQuery:
		return s.hoverAssertQuery(f, n), rangePtr(spanToRange(n.Span()))

	case *scaf.Assert:
		return s.hoverAssert(f, n, tokenCtx, pos)

	case *scaf.FnParam:
		return s.hoverFnParam(f, n, tokenCtx), rangePtr(spanToRange(n.Span()))

	default:
		return "", nil
	}
}

// hoverFnParam generates hover content for a function parameter.
func (s *Server) hoverFnParam(f *analysis.AnalyzedFile, p *scaf.FnParam, tokenCtx *analysis.TokenContext) string {
	var b strings.Builder

	// Show doc comment if present
	writeDocComment(&b, p.LeadingComments)

	// Show parameter info
	typeStr := "any"
	if p.Type != nil {
		typeStr = p.Type.ToGoType()
	}
	b.WriteString(fmt.Sprintf("(parameter) `%s`: %s", p.Name, friendlyTypeName(typeStr)))

	// If we know which function this parameter belongs to, show that
	if tokenCtx != nil && tokenCtx.QueryScope != "" {
		if q, ok := f.Symbols.Queries[tokenCtx.QueryScope]; ok && q.Node != nil {
			b.WriteString(fmt.Sprintf("\n\n_in function `%s`_", q.Name))
		}
	}

	return b.String()
}

// extractDocComment extracts documentation from leading comments.
// Returns the documentation text (without // prefix) or empty string if none.
// Comments that start with "// " (single space) are considered doc comments.
// Blank lines separate doc comments from regular comments.
func extractDocComment(leadingComments []string) string {
	if len(leadingComments) == 0 {
		return ""
	}

	var docLines []string

	for _, comment := range leadingComments {
		// Strip // prefix
		text := strings.TrimPrefix(comment, "//")

		// Handle "// text" (with space) and "//text" (without space)
		if trimmed, ok := strings.CutPrefix(text, " "); ok {
			text = trimmed
		}

		docLines = append(docLines, text)
	}

	if len(docLines) == 0 {
		return ""
	}

	return strings.Join(docLines, "\n")
}

// writeDocComment writes a doc comment block to the builder if present.
func writeDocComment(b *strings.Builder, leadingComments []string) {
	doc := extractDocComment(leadingComments)
	if doc != "" {
		b.WriteString(doc)
		b.WriteString("\n\n---\n\n")
	}
}

// hoverQuery generates hover content for a query definition.
func (s *Server) hoverQuery(q *scaf.Query) string {
	var b strings.Builder

	// Show doc comment if present
	writeDocComment(&b, q.LeadingComments)

	b.WriteString(fmt.Sprintf("**Function:** `%s`\n\n", q.Name))

	// Show parameters with their types and doc comments
	if len(q.Params) > 0 {
		b.WriteString("**Parameters:**\n")
		for _, p := range q.Params {
			typeStr := "any"
			if p.Type != nil {
				typeStr = p.Type.ToGoType()
			}
			b.WriteString(fmt.Sprintf("- `%s`: %s", p.Name, friendlyTypeName(typeStr)))

			// Add inline doc comment if present
			doc := extractDocComment(p.LeadingComments)
			if doc != "" {
				b.WriteString(" — ")
				// Take first line of doc for inline display
				if idx := strings.Index(doc, "\n"); idx > 0 {
					b.WriteString(doc[:idx])
				} else {
					b.WriteString(doc)
				}
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(s.markdownQueryBlock(q.Body))

	return b.String()
}

// hoverQueryScope generates hover content for a query scope, including its own doc comments.
func (s *Server) hoverQueryScope(scope *scaf.QueryScope, q *analysis.QuerySymbol) string {
	var b strings.Builder

	// Show scope's own doc comment if present
	writeDocComment(&b, scope.LeadingComments)

	// Show query info
	b.WriteString(fmt.Sprintf("**Query:** `%s`\n\n", q.Name))

	if len(q.Params) > 0 {
		b.WriteString("**Parameters:** ")

		for i, p := range q.Params {
			if i > 0 {
				b.WriteString(", ")
			}

			b.WriteString("`$" + p + "`")
		}

		b.WriteString("\n\n")
	}

	b.WriteString(s.markdownQueryBlock(q.Body))

	return b.String()
}

// hoverImport generates hover content for an import.
func (s *Server) hoverImport(imp *scaf.Import) string {
	var b strings.Builder

	// Show doc comment if present
	writeDocComment(&b, imp.LeadingComments)

	b.WriteString("**Import**\n\n")
	b.WriteString(fmt.Sprintf("**Path:** `%s`\n", imp.Path))

	if imp.Alias != nil {
		b.WriteString(fmt.Sprintf("**Alias:** `%s`\n", *imp.Alias))
	}

	return b.String()
}

// hoverTest generates hover content for a test.
func (s *Server) hoverTest(t *scaf.Test) string {
	var b strings.Builder

	// Show doc comment if present
	writeDocComment(&b, t.LeadingComments)

	b.WriteString(fmt.Sprintf("**Test:** `%s`\n\n", t.Name))

	// Count inputs and outputs
	var inputs, outputs int

	for _, stmt := range t.Statements {
		if strings.HasPrefix(stmt.Key(), "$") {
			inputs++
		} else {
			outputs++
		}
	}

	b.WriteString(fmt.Sprintf("- **Inputs:** %d\n", inputs))
	b.WriteString(fmt.Sprintf("- **Outputs:** %d\n", outputs))
	b.WriteString(fmt.Sprintf("- **Assertions:** %d\n", len(t.Asserts)))

	if t.Setup != nil {
		b.WriteString("- **Has Setup:** yes\n")
	}

	return b.String()
}

// hoverGroup generates hover content for a group.
func (s *Server) hoverGroup(g *scaf.Group) string {
	var b strings.Builder

	// Show doc comment if present
	writeDocComment(&b, g.LeadingComments)

	b.WriteString(fmt.Sprintf("**Group:** `%s`\n\n", g.Name))

	// Count items
	tests, groups := countItems(g.Items)
	b.WriteString(fmt.Sprintf("- **Tests:** %d\n", tests))
	b.WriteString(fmt.Sprintf("- **Nested Groups:** %d\n", groups))

	if g.Setup != nil {
		b.WriteString("- **Has Setup:** yes\n")
	}

	if g.Teardown != nil {
		b.WriteString("- **Has Teardown:** yes\n")
	}

	return b.String()
}

// countItems counts tests and groups in an item list.
func countItems(items []*scaf.TestOrGroup) (int, int) {
	var tests, groups int

	for _, item := range items {
		if item.Test != nil {
			tests++
		}

		if item.Group != nil {
			groups++
		}
	}

	return tests, groups
}

// rangePtr returns a pointer to a Range.
func rangePtr(r protocol.Range) *protocol.Range {
	return &r
}

// hoverStatement generates hover content for a statement ($param or return field).
// Returns position-aware hovers for the key, value expression, or where clause.
func (s *Server) hoverStatement(f *analysis.AnalyzedFile, stmt *scaf.Statement, tokenCtx *analysis.TokenContext, pos lexer.Position) (string, *protocol.Range) {
	key := stmt.Key()
	if key == "" {
		return "", nil
	}

	// Find the enclosing query for context
	var querySymbol *analysis.QuerySymbol
	if tokenCtx.QueryScope != "" {
		if q, ok := f.Symbols.Queries[tokenCtx.QueryScope]; ok {
			querySymbol = q
		}
	}

	// Check which part of the statement we're hovering over
	// Order matters: check most specific (where, expr) before key

	// Check where clause first
	if stmt.Value != nil && stmt.Value.Where != nil {
		if analysis.ContainsPosition(stmt.Value.Where.Span(), pos) {
			return s.hoverWhereClause(stmt.Value.Where), rangePtr(spanToRange(stmt.Value.Where.Span()))
		}
	}

	// Check expression
	if stmt.Value != nil && stmt.Value.Expr != nil {
		if analysis.ContainsPosition(stmt.Value.Expr.Span(), pos) {
			return s.hoverExpression(stmt.Value.Expr), rangePtr(spanToRange(stmt.Value.Expr.Span()))
		}
	}

	// Check literal value
	if stmt.Value != nil && stmt.Value.Literal != nil {
		if analysis.ContainsPosition(stmt.Value.Literal.Span(), pos) {
			return s.hoverLiteralValue(stmt.Value.Literal), rangePtr(spanToRange(stmt.Value.Literal.Span()))
		}
	}

	// Hovering on key - find which part of the dotted identifier
	if strings.HasPrefix(key, "$") {
		return s.hoverParameterKey(key, stmt, querySymbol), rangePtr(spanToRange(stmt.KeyParts.Span()))
	}

	// For dotted identifiers like u.name, find which part cursor is on
	return s.hoverDottedKey(stmt.KeyParts, pos, querySymbol)
}

// hoverParameterKey generates hover for a parameter key ($param).
// Shows the parameter name with its value inline (like Go's LSP).
func (s *Server) hoverParameterKey(key string, stmt *scaf.Statement, q *analysis.QuerySymbol) string {
	paramName := key[1:] // Remove $ prefix

	// Simple format: (parameter) $id = 1
	var value string
	if stmt.Value != nil {
		if stmt.Value.Expr != nil {
			value = "(" + stmt.Value.Expr.String() + ")"
		} else if stmt.Value.Literal != nil {
			value = stmt.Value.Literal.String()
		}
	}

	if value != "" {
		return fmt.Sprintf("(parameter) `%s` = `%s`", key, value)
	}

	// Check if parameter exists in query
	if q != nil && !slices.Contains(q.Params, paramName) {
		return fmt.Sprintf("(parameter) `%s` — ⚠️ not found in query", key)
	}

	return fmt.Sprintf("(parameter) `%s`", key)
}

// hoverDottedKey generates hover for a dotted identifier like u.name.
// Finds which part the cursor is on and shows type info for that part.
func (s *Server) hoverDottedKey(key *scaf.DottedIdent, pos lexer.Position, q *analysis.QuerySymbol) (string, *protocol.Range) {
	// Find which identifier token the cursor is on
	var hoveredPart string
	var hoveredIndex int = -1
	var partRange protocol.Range

	partIndex := 0
	for _, tok := range key.Tokens {
		// Skip whitespace and dots
		if tok.Type == scaf.TokenWhitespace || tok.Type == scaf.TokenDot {
			continue
		}
		if tok.Type == scaf.TokenIdent {
			tokSpan := scaf.Span{
				Start: tok.Pos,
				End:   lexer.Position{Line: tok.Pos.Line, Column: tok.Pos.Column + len(tok.Value)},
			}
			if analysis.ContainsPosition(tokSpan, pos) {
				hoveredPart = tok.Value
				hoveredIndex = partIndex
				partRange = spanToRange(tokSpan)
				break
			}
			partIndex++
		}
	}

	// Fallback if no specific part found
	if hoveredIndex == -1 {
		return fmt.Sprintf("(field) `%s`", key.String()), rangePtr(spanToRange(key.Span()))
	}

	// Build prefix for the hovered part (e.g., for "name" in "u.name", prefix is "u")
	var prefix string
	if hoveredIndex > 0 {
		prefix = strings.Join(key.Parts[:hoveredIndex], ".") + "."
	}
	fullPath := prefix + hoveredPart

	// Get type information from query analyzer (with schema if available)
	var typeInfo string
	var isOptional bool
	var sourceHint string
	if q != nil && s.queryAnalyzer != nil && q.Body != "" {
		metadata := s.analyzeQueryWithSchema(q.Body)
		if metadata != nil {
			typeInfo = s.getTypeForPath(metadata, key.Parts, hoveredIndex)
			// Add source hint for first identifier
			if hoveredIndex == 0 {
				sourceHint = s.getVariableSourceHint(metadata, hoveredPart)
			}
			// Check if property is optional (for nullable indicator)
			if hoveredIndex > 0 && metadata.Bindings != nil {
				isOptional = s.isPropertyOptional(metadata.Bindings, key.Parts, hoveredIndex)
			}
		}
	}

	// Format type info with friendly descriptions
	friendlyType := friendlyTypeName(typeInfo)
	if isOptional && friendlyType != "" {
		friendlyType += " | null"
	}

	// Format the hover
	if hoveredIndex == 0 {
		// First part is a variable (e.g., "u" in "u.name")
		var b strings.Builder
		b.WriteString(fmt.Sprintf("(variable) `%s`", hoveredPart))
		if friendlyType != "" {
			b.WriteString(": ")
			b.WriteString(friendlyType)
		}
		if sourceHint != "" {
			b.WriteString("\n\n")
			b.WriteString(sourceHint)
		}
		return b.String(), rangePtr(partRange)
	}

	// Subsequent parts are property accesses
	var b strings.Builder
	b.WriteString(fmt.Sprintf("(property) `%s`", fullPath))
	if friendlyType != "" {
		b.WriteString(": ")
		b.WriteString(friendlyType)
	}
	return b.String(), rangePtr(partRange)
}

// getTypeForPath extracts type info from query metadata for a dotted path.
func (s *Server) getTypeForPath(metadata *scaf.QueryMetadata, parts []string, hoveredIndex int) string {
	if metadata == nil || len(parts) == 0 {
		return ""
	}

	fullPath := strings.Join(parts, ".")
	hoveredPath := strings.Join(parts[:hoveredIndex+1], ".")

	// When hovering on a variable (index 0), we need to find the variable's type,
	// not the type of a return field that happens to contain this variable.
	// E.g., for "u.name: false" hovering on "u", we want User type, not string.

	if hoveredIndex == 0 {
		varName := parts[0]

		// First check for direct variable return (RETURN u)
		for _, ret := range metadata.Returns {
			if ret.Expression == varName || ret.Name == varName || ret.Alias == varName {
				if ret.Type != nil {
					return ret.Type.String()
				}
			}
		}

		// Check bindings from MATCH clause (e.g., MATCH (u:User) -> u is bound to User)
		if labels, ok := metadata.Bindings[varName]; ok && len(labels) > 0 {
			// Return the first label as the type (most common case)
			// For multiple labels like (u:Person:Actor), join them
			if len(labels) == 1 {
				return labels[0]
			}
			return strings.Join(labels, " & ")
		}

		return ""
	}

	// For property access (hoveredIndex > 0), look for exact match first
	for _, ret := range metadata.Returns {
		// Check if hovering exactly on this return expression
		if ret.Expression == hoveredPath || ret.Name == hoveredPath || ret.Alias == hoveredPath {
			if ret.Type != nil {
				return ret.Type.String()
			}
		}
	}

	// Check full path match for deeper property access
	for _, ret := range metadata.Returns {
		if ret.Expression == fullPath || ret.Name == fullPath || ret.Alias == fullPath {
			if ret.Type != nil {
				return ret.Type.String()
			}
		}
	}

	return ""
}

// isPropertyOptional checks if a property is optional (required: false) in the schema.
// It uses bindings to find the model type, then looks up the field.
func (s *Server) isPropertyOptional(bindings map[string][]string, parts []string, hoveredIndex int) bool {
	if s.schema == nil || len(parts) < 2 || hoveredIndex < 1 {
		return false
	}

	// Get the variable name (first part, e.g., "u" in "u.name")
	varName := parts[0]

	// Get the labels for this variable from bindings
	labels, ok := bindings[varName]
	if !ok || len(labels) == 0 {
		return false
	}

	// Get the property name being hovered
	propName := parts[hoveredIndex]

	// Look up the field in the schema
	for _, label := range labels {
		if model, ok := s.schema.Models[label]; ok {
			for _, field := range model.Fields {
				if field.Name == propName {
					// Field is optional if not required
					return !field.Required
				}
			}
		}
	}

	return false
}

// getPropertyType resolves the type of a property path on a base type using the schema.
func (s *Server) getPropertyType(baseType *scaf.Type, props []string) string {
	if s.schema == nil || baseType == nil || len(props) == 0 {
		return ""
	}

	// Get the model name from the type
	modelName := ""
	switch baseType.Kind {
	case scaf.TypeKindPointer:
		if baseType.Elem != nil {
			modelName = baseType.Elem.Name
		}
	case scaf.TypeKindNamed:
		modelName = baseType.Name
	default:
		return ""
	}

	if modelName == "" {
		return ""
	}

	// Look up the model
	model, ok := s.schema.Models[modelName]
	if !ok {
		return ""
	}

	// Traverse the property path
	currentModel := model
	var resultType *scaf.Type
	for _, prop := range props {
		found := false
		for _, field := range currentModel.Fields {
			if field.Name == prop {
				resultType = field.Type
				found = true
				// If there are more props, try to follow the type
				if field.Type != nil {
					nextModelName := ""
					switch field.Type.Kind {
					case scaf.TypeKindPointer:
						if field.Type.Elem != nil {
							nextModelName = field.Type.Elem.Name
						}
					case scaf.TypeKindNamed:
						nextModelName = field.Type.Name
					}
					if nextModel, ok := s.schema.Models[nextModelName]; ok {
						currentModel = nextModel
					}
				}
				break
			}
		}
		if !found {
			return ""
		}
	}

	if resultType != nil {
		return resultType.String()
	}
	return ""
}

// schemaAwareAnalyzer is an interface for analyzers that support schema-aware type inference.
type schemaAwareAnalyzer interface {
	AnalyzeQueryWithSchema(query string, schema *analysis.TypeSchema) (*scaf.QueryMetadata, error)
}

// analyzeQueryWithSchema analyzes a query, using schema-aware type inference if available.
func (s *Server) analyzeQueryWithSchema(query string) *scaf.QueryMetadata {
	if s.queryAnalyzer == nil {
		return nil
	}

	// Try schema-aware analyzer first
	if schemaAnalyzer, ok := s.queryAnalyzer.(schemaAwareAnalyzer); ok && s.schema != nil {
		metadata, err := schemaAnalyzer.AnalyzeQueryWithSchema(query, s.schema)
		if err == nil {
			return metadata
		}
	}

	// Fall back to basic analysis
	metadata, err := s.queryAnalyzer.AnalyzeQuery(query)
	if err != nil {
		return nil
	}
	return metadata
}

// hoverWhereClause generates hover for a where constraint.
func (s *Server) hoverWhereClause(where *scaf.ParenExpr) string {
	return fmt.Sprintf("(constraint) `where (%s)` → bool", where.String())
}

// hoverExpression generates hover for an expression value.
// Shows expression type and context about evaluation.
func (s *Server) hoverExpression(expr *scaf.ParenExpr) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("(expression) `(%s)`\n\n", expr.String()))
	b.WriteString("**Type:** evaluated at runtime\n\n")
	b.WriteString("_Expression values are computed using [expr-lang](https://expr-lang.org) at test execution time._")
	return b.String()
}

// hoverLiteralValue generates hover for a literal value.
func (s *Server) hoverLiteralValue(val *scaf.Value) string {
	return fmt.Sprintf("(literal) `%s`", val.String())
}

// hoverSetupCall generates hover content for a setup call (module.Query()).
func (s *Server) hoverSetupCall(doc *Document, f *analysis.AnalyzedFile, call *scaf.SetupCall, tokenCtx *analysis.TokenContext) string {
	var b strings.Builder

	// Check if hovering on the module name or query name
	if tokenCtx.Token != nil {
		if tokenCtx.Token.Value == call.Module {
			// Hovering on module name - show import info
			return s.hoverModuleRef(f, call.Module)
		}
	}

	// Show info about the query being called
	b.WriteString(fmt.Sprintf("**Setup Call:** `%s.%s`\n\n", call.Module, call.Query))

	// Try to load the imported module and get query info
	if s.fileLoader != nil {
		if imp, ok := f.Symbols.Imports[call.Module]; ok {
			docPath := URIToPath(doc.URI)
			importedPath := s.fileLoader.ResolveImportPath(docPath, imp.Path)

			// Check if the imported file is currently open in the editor
			// If so, use the in-memory version instead of the disk version
			importedURI := PathToURI(importedPath)
			if openDoc, ok := s.getDocument(importedURI); ok && openDoc.Analysis != nil {
				// Use the open document's analysis
				return s.hoverSetupCallWithAnalysis(call, openDoc.Analysis, &b)
			}

			// Otherwise load from disk
			importedFile, err := s.fileLoader.LoadAndAnalyze(importedPath)
			if err != nil {
				s.logger.Debug("Failed to load imported file for hover",
					zap.String("path", importedPath),
					zap.Error(err))
				b.WriteString(fmt.Sprintf("⚠️ Could not load module `%s`\n\n", call.Module))
				b.WriteString(fmt.Sprintf("**Path:** `%s`\n", imp.Path))
				b.WriteString("\n_Tip: Make sure the imported file exists and is saved._\n")
				return b.String()
			}

			return s.hoverSetupCallWithAnalysis(call, importedFile, &b)
		} else {
			s.logger.Debug("Import not found for module",
				zap.String("module", call.Module))
			b.WriteString(fmt.Sprintf("⚠️ Module `%s` not found in imports\n", call.Module))
		}
	} else {
		s.logger.Debug("FileLoader not available for hover")
	}

	return b.String()
}

// hoverSetupCallWithAnalysis generates hover content using a loaded/analyzed file.
func (s *Server) hoverSetupCallWithAnalysis(call *scaf.SetupCall, importedFile *analysis.AnalyzedFile, b *strings.Builder) string {
	if importedFile.Symbols == nil {
		fmt.Fprintf(b, "⚠️ Module `%s` could not be analyzed\n", call.Module)
		return b.String()
	}

	if q, ok := importedFile.Symbols.Queries[call.Query]; ok {
		if len(q.Params) > 0 {
			b.WriteString("**Parameters:** ")
			for i, p := range q.Params {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString("`$" + p + "`")
			}
			b.WriteString("\n\n")
		}
		b.WriteString(s.markdownQueryBlock(q.Body))
		return b.String()
	}

	// Query not found - provide helpful diagnostics
	var queryNames []string
	for name := range importedFile.Symbols.Queries {
		queryNames = append(queryNames, name)
	}
	s.logger.Debug("Query not found in imported file",
		zap.String("queryName", call.Query),
		zap.Strings("availableQueries", queryNames))

	fmt.Fprintf(b, "⚠️ Query `%s` not found in module `%s`\n\n", call.Query, call.Module)

	if len(queryNames) > 0 {
		b.WriteString("**Available queries:**\n")
		for _, name := range queryNames {
			fmt.Fprintf(b, "- `%s`\n", name)
		}
	} else {
		b.WriteString("_No queries found in this module._\n")
	}

	return b.String()
}

// hoverSetupClause generates hover content for a setup clause.
func (s *Server) hoverSetupClause(doc *Document, f *analysis.AnalyzedFile, clause *scaf.SetupClause, tokenCtx *analysis.TokenContext) string {
	// If it's a module reference (setup fixtures), show module info
	if clause.Module != nil {
		return s.hoverModuleRef(f, *clause.Module)
	}

	// If it's an inline query, show it
	if clause.Inline != nil {
		var b strings.Builder
		b.WriteString("**Inline Setup Query**\n\n")
		b.WriteString(s.markdownQueryBlock(*clause.Inline))
		return b.String()
	}

	// If it's a block, show count
	if len(clause.Block) > 0 {
		return fmt.Sprintf("**Setup Block:** %d items", len(clause.Block))
	}

	return ""
}

// hoverSetupItem generates hover content for a setup item in a block.
func (s *Server) hoverSetupItem(doc *Document, f *analysis.AnalyzedFile, item *scaf.SetupItem, tokenCtx *analysis.TokenContext) string {
	// If it's a module reference, show module info
	if item.Module != nil {
		return s.hoverModuleRef(f, *item.Module)
	}

	// If it's an inline query, show it
	if item.Inline != nil {
		var b strings.Builder
		b.WriteString("**Inline Setup Query**\n\n")
		b.WriteString(s.markdownQueryBlock(*item.Inline))
		return b.String()
	}

	return ""
}

// hoverModuleRef generates hover content for a module reference.
func (s *Server) hoverModuleRef(f *analysis.AnalyzedFile, moduleName string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("**Module:** `%s`\n\n", moduleName))

	if imp, ok := f.Symbols.Imports[moduleName]; ok {
		b.WriteString(fmt.Sprintf("**Path:** `%s`\n", imp.Path))

		// If we can load the module, show its contents
		if s.fileLoader != nil {
			// Note: we'd need the document path here for proper resolution
			// For now just show the import path
		}
	} else {
		b.WriteString("⚠️ Module not found in imports\n")
	}

	return b.String()
}

// hoverAssertQuery generates hover content for an assert query reference.
func (s *Server) hoverAssertQuery(f *analysis.AnalyzedFile, aq *scaf.AssertQuery) string {
	var b strings.Builder

	// Check if it's a named query reference or inline
	if aq.QueryName != nil {
		b.WriteString(fmt.Sprintf("**Assert Query:** `%s`\n\n", *aq.QueryName))

		if q, ok := f.Symbols.Queries[*aq.QueryName]; ok {
			if len(q.Params) > 0 {
				b.WriteString("**Parameters:** ")
				for i, p := range q.Params {
					if i > 0 {
						b.WriteString(", ")
					}
					b.WriteString("`$" + p + "`")
				}
				b.WriteString("\n\n")
			}
			b.WriteString(s.markdownQueryBlock(q.Body))
		} else {
			b.WriteString("⚠️ Query not found\n")
		}
	} else if aq.Inline != nil {
		b.WriteString("**Inline Assert Query**\n\n")
		b.WriteString(s.markdownQueryBlock(*aq.Inline))
	}

	return b.String()
}

// hoverAssert generates hover content for an assert block.
// Finds identifiers within assert expressions and shows type info.
// When hovering on the assert keyword or block, shows summary info.
func (s *Server) hoverAssert(f *analysis.AnalyzedFile, assert *scaf.Assert, tokenCtx *analysis.TokenContext, pos lexer.Position) (string, *protocol.Range) {
	// Determine which query provides the type context:
	// 1. If assert has its own query (named or inline), use that
	// 2. Otherwise use the parent scope's query
	var queryBody string
	if assert.Query != nil {
		if assert.Query.Inline != nil {
			// Inline query: assert `MATCH (c:Comment) RETURN c` { ... }
			queryBody = *assert.Query.Inline
		} else if assert.Query.QueryName != nil {
			// Named query reference: assert SomeQuery() { ... }
			if q, ok := f.Symbols.Queries[*assert.Query.QueryName]; ok {
				queryBody = q.Body
			}
		}
	}
	// Fall back to parent scope query if no assert query
	if queryBody == "" && tokenCtx.QueryScope != "" {
		if q, ok := f.Symbols.Queries[tokenCtx.QueryScope]; ok {
			queryBody = q.Body
		}
	}

	// Check all conditions (shorthand or block form)
	for _, cond := range assert.AllConditions() {
		if !analysis.ContainsPosition(cond.Span(), pos) {
			continue
		}
		// Find the identifier at this position within the expression
		return s.hoverExprIdentifierWithQuery(cond, pos, queryBody)
	}

	// Not hovering on a specific condition - show assert block summary
	conditions := assert.AllConditions()
	var b strings.Builder

	if assert.IsShorthand() {
		b.WriteString("**Assertion** (shorthand)\n\n")
		b.WriteString(fmt.Sprintf("Condition: `(%s)` → bool", conditions[0].String()))
	} else {
		b.WriteString(fmt.Sprintf("**Assertion block:** %d condition(s)\n\n", len(conditions)))
		if assert.Query != nil {
			if assert.Query.Inline != nil {
				b.WriteString("Query: inline\n")
			} else if assert.Query.QueryName != nil {
				b.WriteString(fmt.Sprintf("Query: `%s`\n", *assert.Query.QueryName))
			}
		}
		for i, cond := range conditions {
			if i < 3 { // Show first 3 conditions
				b.WriteString(fmt.Sprintf("- `(%s)` → bool\n", cond.String()))
			}
		}
		if len(conditions) > 3 {
			b.WriteString(fmt.Sprintf("- ... and %d more\n", len(conditions)-3))
		}
	}

	return b.String(), rangePtr(spanToRange(assert.Span()))
}

// hoverExprIdentifier finds an identifier in an expression and shows its type.
// Shows clearer type info with friendly descriptions and context hints.
func (s *Server) hoverExprIdentifier(expr *scaf.ParenExpr, pos lexer.Position, q *analysis.QuerySymbol) (string, *protocol.Range) {
	var queryBody string
	if q != nil {
		queryBody = q.Body
	}
	return s.hoverExprIdentifierWithQuery(expr, pos, queryBody)
}

// hoverExprIdentifierWithQuery finds an identifier in an expression and shows its type.
// Takes the query body directly for flexibility (used by assert queries).
func (s *Server) hoverExprIdentifierWithQuery(expr *scaf.ParenExpr, pos lexer.Position, queryBody string) (string, *protocol.Range) {
	if expr == nil {
		return "", nil
	}

	// Find dotted paths in the expression tokens
	// Look for patterns like: ident, dot, ident, dot, ident
	var idents []identInfo
	s.collectDottedIdents(expr.Tokens, nil, &idents)

	// Find which identifier the cursor is on
	for _, info := range idents {
		tok := info.tok
		tokSpan := scaf.Span{
			Start: tok.Pos,
			End:   lexer.Position{Line: tok.Pos.Line, Column: tok.Pos.Column + len(*tok.Ident)},
		}
		if !analysis.ContainsPosition(tokSpan, pos) {
			continue
		}

		// Get type info (with schema if available)
		var typeInfo string
		var sourceHint string
		if s.queryAnalyzer != nil && queryBody != "" {
			metadata := s.analyzeQueryWithSchema(queryBody)
			if metadata != nil {
				typeInfo = s.getTypeForPath(metadata, info.parts, info.index)
				// Add source hint for first identifier
				if info.index == 0 && len(info.parts) > 0 {
					sourceHint = s.getVariableSourceHint(metadata, info.parts[0])
				}
			}
		}

		// Format type info with friendly descriptions
		friendlyType := friendlyTypeName(typeInfo)

		// Determine if variable or property
		if info.index == 0 {
			var b strings.Builder
			b.WriteString(fmt.Sprintf("(variable) `%s`", *tok.Ident))
			if friendlyType != "" {
				b.WriteString(": ")
				b.WriteString(friendlyType)
			}
			if sourceHint != "" {
				b.WriteString("\n\n")
				b.WriteString(sourceHint)
			}
			return b.String(), rangePtr(spanToRange(tokSpan))
		}

		fullPath := strings.Join(info.parts, ".")
		var b strings.Builder
		b.WriteString(fmt.Sprintf("(property) `%s`", fullPath))
		if friendlyType != "" {
			b.WriteString(": ")
			b.WriteString(friendlyType)
		}
		return b.String(), rangePtr(spanToRange(tokSpan))
	}

	return "", nil
}

// friendlyTypeName converts Go-style types to friendlier descriptions.
func friendlyTypeName(goType string) string {
	if goType == "" {
		return "(any)"
	}

	// Handle pointer types
	if strings.HasPrefix(goType, "*") {
		inner := friendlyTypeName(goType[1:])
		if inner == "(any)" {
			return "(any)"
		}
		return inner + "?"
	}

	// Handle slice types
	if strings.HasPrefix(goType, "[]") {
		inner := friendlyTypeName(goType[2:])
		return "[" + inner + "]"
	}

	// Handle map types
	if strings.HasPrefix(goType, "map[") {
		// Find the key and value types
		rest := goType[4:]
		bracketCount := 1
		keyEnd := 0
		for i, c := range rest {
			if c == '[' {
				bracketCount++
			} else if c == ']' {
				bracketCount--
				if bracketCount == 0 {
					keyEnd = i
					break
				}
			}
		}
		keyType := friendlyTypeName(rest[:keyEnd])
		valueType := friendlyTypeName(rest[keyEnd+1:])
		return "{" + keyType + ": " + valueType + "}"
	}

	// Common type mappings
	switch goType {
	case "int64", "int32", "int":
		return "integer"
	case "float64", "float32":
		return "number"
	case "string":
		return "string"
	case "bool":
		return "boolean"
	case "any", "interface{}":
		return "(any)"
	default:
		// Keep named types as-is (e.g., "User", "Node")
		return goType
	}
}

// getVariableSourceHint returns a hint about where a variable comes from.
func (s *Server) getVariableSourceHint(metadata *scaf.QueryMetadata, varName string) string {
	if metadata == nil {
		return ""
	}

	// Check if variable is in RETURN clause
	for _, ret := range metadata.Returns {
		if ret.Name == varName || ret.Expression == varName || ret.Alias == varName {
			return "_from RETURN clause_"
		}
	}

	return ""
}

// collectDottedIdents collects identifiers with their dotted path context.
func (s *Server) collectDottedIdents(tokens []*scaf.BalancedExprToken, currentPath []string, out *[]identInfo) {
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok == nil {
			continue
		}

		if tok.Ident != nil {
			// Check if next token is a dot
			if i+1 < len(tokens) && tokens[i+1] != nil && tokens[i+1].Dot {
				// Start or continue a dotted path
				newPath := append(currentPath, *tok.Ident)
				*out = append(*out, identInfo{
					tok:   tok,
					parts: slices.Clone(newPath),
					index: len(newPath) - 1,
				})
				// Skip the dot, continue with the path
				i++
				// Recursively collect the rest with the current path
				s.collectDottedIdents(tokens[i+1:], newPath, out)
				return
			} else {
				// End of path or standalone identifier
				newPath := append(currentPath, *tok.Ident)
				*out = append(*out, identInfo{
					tok:   tok,
					parts: slices.Clone(newPath),
					index: len(newPath) - 1,
				})
				// Reset path for next identifier
				currentPath = nil
			}
		} else if tok.NestedParen != nil {
			// Recurse into nested parens with fresh path
			s.collectDottedIdents(tok.NestedParen.Tokens, nil, out)
		}
	}
}

type identInfo struct {
	tok   *scaf.BalancedExprToken
	parts []string
	index int
}
