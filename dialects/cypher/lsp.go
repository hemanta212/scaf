package cypher

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
	cyphergrammar "github.com/rlch/scaf/dialects/cypher/grammar"
)

// Ensure Dialect implements DialectLSP.
var _ scaf.DialectLSP = (*Dialect)(nil)

// cypherKeywords are the Cypher language keywords.
var cypherKeywords = map[string]string{
	// Reading clauses
	"MATCH":    "Specify patterns to search for in the database",
	"OPTIONAL": "Used with MATCH to return null for missing matches",
	"WHERE":    "Filter results based on conditions",
	"RETURN":   "Specify what to include in query results",
	"WITH":     "Pass results to subsequent query parts",
	"UNWIND":   "Expand a list into individual rows",
	"CALL":     "Call a procedure",
	"YIELD":    "Specify which procedure results to use",

	// Writing clauses
	"CREATE":  "Create nodes and relationships",
	"MERGE":   "Create or match nodes and relationships",
	"DELETE":  "Delete nodes and relationships",
	"DETACH":  "Used with DELETE to remove relationships first",
	"SET":     "Set properties on nodes and relationships",
	"REMOVE":  "Remove properties or labels",
	"FOREACH": "Update data within a list",

	// Subquery
	"EXISTS": "Test for existence of a pattern",

	// Ordering and pagination
	"ORDER": "Order results",
	"BY":    "Specify ordering criteria",
	"ASC":   "Ascending order",
	"DESC":  "Descending order",
	"SKIP":  "Skip a number of results",
	"LIMIT": "Limit number of results",

	// Logical operators
	"AND":  "Logical AND",
	"OR":   "Logical OR",
	"XOR":  "Logical exclusive OR",
	"NOT":  "Logical NOT",
	"IN":   "Check if value is in list",
	"IS":   "Used with NULL check",
	"NULL": "Null value literal",

	// Pattern elements
	"AS": "Alias for expressions",

	// Aggregation
	"DISTINCT": "Remove duplicate values",

	// CASE expression
	"CASE": "Conditional expression",
	"WHEN": "Case condition",
	"THEN": "Case result",
	"ELSE": "Default case result",
	"END":  "End of CASE expression",

	// Literals
	"TRUE":  "Boolean true",
	"FALSE": "Boolean false",

	// MERGE actions
	"ON": "Specify MERGE actions",

	// UNION
	"UNION": "Combine query results",
	"ALL":   "Include duplicates in UNION",

	// String predicates
	"STARTS":   "String starts with predicate",
	"ENDS":     "String ends with predicate",
	"CONTAINS": "String contains predicate",

	// Quantified predicates
	"ANY":    "Test if any element matches",
	"NONE":   "Test if no element matches",
	"SINGLE": "Test if exactly one element matches",
}

// Complete provides completions for a position within a Cypher query.
func (d *Dialect) Complete(query string, offset int, ctx *scaf.QueryLSPContext) []scaf.QueryCompletion {
	// Parse query to understand context
	parsed, _ := cyphergrammar.Parse(query)

	// Get the text before cursor to understand what we're completing
	textBefore := ""
	if offset > 0 && offset <= len(query) {
		textBefore = query[:offset]
	}

	// Determine completion context
	compCtx := d.analyzeCompletionContext(textBefore, parsed, offset, ctx)

	var items []scaf.QueryCompletion

	switch compCtx.kind {
	case completionContextKeyword:
		items = d.completeKeywords(compCtx)
	case completionContextFunction:
		items = d.completeFunctions(compCtx)
	case completionContextLabel:
		items = d.completeLabels(compCtx, ctx)
	case completionContextRelType:
		items = d.completeRelationshipTypes(compCtx, ctx)
	case completionContextProperty:
		items = d.completeProperties(compCtx, ctx)
	case completionContextVariable:
		items = d.completeVariables(compCtx, parsed)
		items = append(items, d.completeFunctions(compCtx)...)
	case completionContextParameter:
		items = d.completeParameters(compCtx, ctx)
	default:
		items = append(items, d.completeKeywords(compCtx)...)
		items = append(items, d.completeFunctions(compCtx)...)
	}

	// Filter by prefix if we have one
	if compCtx.prefix != "" {
		items = filterCompletions(items, compCtx.prefix)
	}

	return items
}

type completionContextKind int

const (
	completionContextUnknown completionContextKind = iota
	completionContextKeyword
	completionContextFunction
	completionContextLabel
	completionContextRelType
	completionContextProperty
	completionContextVariable
	completionContextParameter
)

type completionContext struct {
	kind   completionContextKind
	prefix string // Text being typed

	// Contextual information
	afterColon     bool   // After : in pattern (expecting label/type)
	afterDot       bool   // After . (expecting property)
	afterDollar    bool   // After $ (expecting parameter)
	afterMatch     bool   // After MATCH
	afterReturn    bool   // After RETURN
	afterWhere     bool   // After WHERE
	inNodePattern  bool   // Inside (...)
	inRelPattern   bool   // Inside [...]
	inFunctionCall bool   // Inside function call
	variableName   string // Variable name when completing properties

	// Relationship pattern context (for context-aware rel type completions)
	leftNodeLabels  []string // Labels on the node to the left of the relationship
	rightNodeLabels []string // Labels on the node to the right (if known)
	relDirectionOut bool     // Relationship points right: -[:]->
	relDirectionIn  bool     // Relationship points left: <-[:]-
}

func (d *Dialect) analyzeCompletionContext(textBefore string, parsed *cyphergrammar.Script, offset int, ctx *scaf.QueryLSPContext) *completionContext {
	cc := &completionContext{
		kind: completionContextUnknown,
	}

	if len(textBefore) == 0 {
		cc.kind = completionContextKeyword
		return cc
	}

	// Extract what's being typed (prefix)
	cc.prefix = extractCypherPrefix(textBefore)

	// Check last non-whitespace character(s) for context
	trimmed := strings.TrimRightFunc(textBefore, unicode.IsSpace)
	if len(trimmed) == 0 {
		cc.kind = completionContextKeyword
		return cc
	}

	lastChar := trimmed[len(trimmed)-1]

	// Check for specific triggers
	switch lastChar {
	case ':':
		cc.afterColon = true
		if d.isInNodePattern(trimmed) {
			cc.inNodePattern = true
			cc.kind = completionContextLabel
		} else if d.isInRelPattern(trimmed) {
			cc.inRelPattern = true
			cc.kind = completionContextRelType
			// Extract relationship pattern context for smarter completions
			d.extractRelPatternContext(trimmed, parsed, cc)
		}
		return cc

	case '.':
		cc.afterDot = true
		cc.kind = completionContextProperty
		cc.variableName = extractVariableBeforeDot(trimmed)
		return cc

	case '$':
		cc.afterDollar = true
		cc.kind = completionContextParameter
		return cc

	case '(':
		beforeParen := strings.TrimRightFunc(trimmed[:len(trimmed)-1], unicode.IsSpace)
		if d.looksLikeFunctionName(beforeParen) {
			cc.inFunctionCall = true
			cc.kind = completionContextVariable
		} else {
			cc.inNodePattern = true
			cc.kind = completionContextVariable
		}
		return cc

	case '[':
		cc.inRelPattern = true
		cc.kind = completionContextVariable
		return cc
	}

	// Check for keyword context
	upperTrimmed := strings.ToUpper(trimmed)

	if strings.HasSuffix(upperTrimmed, "MATCH") || strings.HasSuffix(upperTrimmed, "OPTIONAL MATCH") {
		cc.afterMatch = true
		cc.kind = completionContextKeyword
	} else if strings.HasSuffix(upperTrimmed, "RETURN") || strings.HasSuffix(upperTrimmed, "WITH") {
		cc.afterReturn = true
		cc.kind = completionContextVariable
	} else if strings.HasSuffix(upperTrimmed, "WHERE") {
		cc.afterWhere = true
		cc.kind = completionContextVariable
	} else {
		cc.kind = completionContextKeyword
	}

	return cc
}

func (d *Dialect) isInNodePattern(text string) bool {
	opens := strings.Count(text, "(")
	closes := strings.Count(text, ")")
	return opens > closes
}

func (d *Dialect) isInRelPattern(text string) bool {
	opens := strings.Count(text, "[")
	closes := strings.Count(text, "]")
	return opens > closes
}

// extractRelPatternContext extracts context about the relationship pattern for smarter completions.
func (d *Dialect) extractRelPatternContext(text string, parsed *cyphergrammar.Script, cc *completionContext) {
	// Find the '[' that starts the current relationship pattern
	bracketPos := strings.LastIndex(text, "[")
	if bracketPos < 0 {
		return
	}

	// Look at what's before the bracket to determine direction
	beforeBracket := text[:bracketPos]
	beforeBracket = strings.TrimRightFunc(beforeBracket, unicode.IsSpace)

	if len(beforeBracket) >= 2 && beforeBracket[len(beforeBracket)-2:] == "<-" {
		cc.relDirectionIn = true
	}

	// Find the preceding node pattern
	searchFrom := len(beforeBracket)
	if cc.relDirectionIn && len(beforeBracket) >= 2 {
		searchFrom = len(beforeBracket) - 2
	} else if len(beforeBracket) >= 1 {
		searchFrom = len(beforeBracket) - 1
	}

	nodeEndPos := strings.LastIndex(beforeBracket[:searchFrom], ")")
	if nodeEndPos < 0 {
		return
	}

	// Find the matching '(' for this node pattern
	parenDepth := 1
	nodeStartPos := -1
	for i := nodeEndPos - 1; i >= 0; i-- {
		switch beforeBracket[i] {
		case ')':
			parenDepth++
		case '(':
			parenDepth--
			if parenDepth == 0 {
				nodeStartPos = i
			}
		}
		if nodeStartPos >= 0 {
			break
		}
	}

	if nodeStartPos < 0 {
		return
	}

	// Extract the node content (between parentheses)
	nodeContent := beforeBracket[nodeStartPos+1 : nodeEndPos]

	// Extract labels from node content
	cc.leftNodeLabels = extractLabelsFromNodeContent(nodeContent)

	// Try to use AST for variable label inference if no explicit labels found
	if parsed != nil && len(cc.leftNodeLabels) == 0 {
		cc.leftNodeLabels = d.findLabelsForVariable(parsed, nodeContent)
	}
}

// extractLabelsFromNodeContent extracts labels from a node pattern content string.
func extractLabelsFromNodeContent(content string) []string {
	var labels []string
	content = strings.TrimSpace(content)

	colonIdx := strings.Index(content, ":")
	if colonIdx < 0 {
		return nil
	}

	labelsAndRest := content[colonIdx:]

	i := 0
	for i < len(labelsAndRest) {
		if labelsAndRest[i] == ':' {
			i++
			start := i
			for i < len(labelsAndRest) && (unicode.IsLetter(rune(labelsAndRest[i])) || unicode.IsDigit(rune(labelsAndRest[i])) || labelsAndRest[i] == '_') {
				i++
			}
			if i > start {
				labels = append(labels, labelsAndRest[start:i])
			}
		} else if labelsAndRest[i] == '{' || labelsAndRest[i] == '$' {
			break
		} else {
			i++
		}
	}

	return labels
}

// findLabelsForVariable tries to find the labels for a node variable using the AST.
func (d *Dialect) findLabelsForVariable(parsed *cyphergrammar.Script, nodeContent string) []string {
	content := strings.TrimSpace(nodeContent)
	varEnd := strings.IndexAny(content, ":{ ")
	var varName string
	if varEnd > 0 {
		varName = content[:varEnd]
	} else if varEnd < 0 && len(content) > 0 && unicode.IsLetter(rune(content[0])) {
		varName = content
	}

	if varName == "" || parsed == nil || parsed.Query == nil || parsed.Query.RegularQuery == nil {
		return nil
	}

	rq := parsed.Query.RegularQuery
	if rq.SingleQuery == nil {
		return nil
	}

	for _, clause := range rq.SingleQuery.Clauses {
		if labels := d.findVariableLabelsInClause(clause, varName); len(labels) > 0 {
			return labels
		}
	}

	return nil
}

func (d *Dialect) findVariableLabelsInClause(clause *cyphergrammar.Clause, varName string) []string {
	if clause == nil {
		return nil
	}

	if clause.Reading != nil && clause.Reading.Match != nil && clause.Reading.Match.Pattern != nil {
		if labels := d.findVariableLabelsInPattern(clause.Reading.Match.Pattern, varName); len(labels) > 0 {
			return labels
		}
	}

	if clause.Updating != nil {
		if clause.Updating.Create != nil && clause.Updating.Create.Pattern != nil {
			if labels := d.findVariableLabelsInPattern(clause.Updating.Create.Pattern, varName); len(labels) > 0 {
				return labels
			}
		}
		if clause.Updating.Merge != nil && clause.Updating.Merge.Pattern != nil {
			if labels := d.findVariableLabelsInPatternPart(clause.Updating.Merge.Pattern, varName); len(labels) > 0 {
				return labels
			}
		}
	}

	return nil
}

func (d *Dialect) findVariableLabelsInPattern(pattern *cyphergrammar.Pattern, varName string) []string {
	if pattern == nil {
		return nil
	}
	for _, part := range pattern.Parts {
		if labels := d.findVariableLabelsInPatternPart(part, varName); len(labels) > 0 {
			return labels
		}
	}
	return nil
}

func (d *Dialect) findVariableLabelsInPatternPart(part *cyphergrammar.PatternPart, varName string) []string {
	if part == nil || part.Element == nil {
		return nil
	}
	return d.findVariableLabelsInPatternElement(part.Element, varName)
}

func (d *Dialect) findVariableLabelsInPatternElement(elem *cyphergrammar.PatternElement, varName string) []string {
	if elem == nil {
		return nil
	}

	if elem.Paren != nil {
		return d.findVariableLabelsInPatternElement(elem.Paren, varName)
	}

	if elem.Node != nil {
		if elem.Node.Variable == varName && elem.Node.Labels != nil {
			return elem.Node.Labels.Labels
		}
	}

	for _, chain := range elem.Chain {
		if chain.Node != nil {
			if chain.Node.Variable == varName && chain.Node.Labels != nil {
				return chain.Node.Labels.Labels
			}
		}
	}

	return nil
}

func (d *Dialect) looksLikeFunctionName(text string) bool {
	text = strings.TrimRightFunc(text, unicode.IsSpace)
	if len(text) == 0 {
		return false
	}

	end := len(text)
	start := end
	for i := end - 1; i >= 0; i-- {
		c := rune(text[i])
		if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' || c == '.' {
			start = i
		} else {
			break
		}
	}

	if start == end {
		return false
	}

	name := strings.ToLower(text[start:end])
	if _, ok := cypherFunctionTypes[name]; ok {
		return true
	}
	if functionsWithArgInference[name] {
		return true
	}

	return false
}

func extractVariableBeforeDot(text string) string {
	if len(text) == 0 || text[len(text)-1] != '.' {
		return ""
	}

	end := len(text) - 1
	start := end

	for i := end - 1; i >= 0; i-- {
		c := rune(text[i])
		if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' {
			start = i
		} else {
			break
		}
	}

	if start == end {
		return ""
	}

	return text[start:end]
}

func extractCypherPrefix(text string) string {
	if len(text) == 0 {
		return ""
	}

	end := len(text)
	start := end

	for i := end - 1; i >= 0; i-- {
		c := rune(text[i])
		if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' {
			start = i
		} else {
			break
		}
	}

	return text[start:end]
}

func filterCompletions(items []scaf.QueryCompletion, prefix string) []scaf.QueryCompletion {
	if prefix == "" {
		return items
	}

	prefix = strings.ToLower(prefix)
	var filtered []scaf.QueryCompletion

	for _, item := range items {
		if strings.HasPrefix(strings.ToLower(item.Label), prefix) {
			filtered = append(filtered, item)
		}
	}

	return filtered
}

func (d *Dialect) completeKeywords(cc *completionContext) []scaf.QueryCompletion {
	var items []scaf.QueryCompletion

	for keyword, desc := range cypherKeywords {
		items = append(items, scaf.QueryCompletion{
			Label:         keyword,
			Kind:          scaf.QueryCompletionKeyword,
			Detail:        desc,
			InsertText:    keyword,
			Documentation: desc,
			SortText:      "1" + keyword,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Label < items[j].Label
	})

	return items
}

func (d *Dialect) completeFunctions(cc *completionContext) []scaf.QueryCompletion {
	var items []scaf.QueryCompletion

	for name, returnType := range cypherFunctionTypes {
		items = append(items, scaf.QueryCompletion{
			Label:      name,
			Kind:       scaf.QueryCompletionFunction,
			Detail:     fmt.Sprintf("→ %s", returnType),
			InsertText: name + "($1)",
			IsSnippet:  true,
			SortText:   "2" + name,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Label < items[j].Label
	})

	return items
}

func (d *Dialect) completeLabels(cc *completionContext, ctx *scaf.QueryLSPContext) []scaf.QueryCompletion {
	if ctx == nil || ctx.Schema == nil {
		return nil
	}

	schema, ok := ctx.Schema.(*analysis.TypeSchema)
	if !ok || schema == nil {
		return nil
	}

	var items []scaf.QueryCompletion

	for name := range schema.Models {
		items = append(items, scaf.QueryCompletion{
			Label:      name,
			Kind:       scaf.QueryCompletionLabel,
			Detail:     "node label",
			InsertText: name,
			SortText:   "3" + name,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Label < items[j].Label
	})

	return items
}

func (d *Dialect) completeRelationshipTypes(cc *completionContext, ctx *scaf.QueryLSPContext) []scaf.QueryCompletion {
	if ctx == nil || ctx.Schema == nil {
		return nil
	}

	schema, ok := ctx.Schema.(*analysis.TypeSchema)
	if !ok || schema == nil {
		return nil
	}

	hasLeftLabels := len(cc.leftNodeLabels) > 0

	var items []scaf.QueryCompletion
	seen := make(map[string]bool)

	if hasLeftLabels {
		if cc.relDirectionIn {
			// Pattern: (leftNode)<-[:]- means we want relationships that TARGET leftNode
			// Look for relationships from ANY model that have:
			// - Target matching one of leftNodeLabels
			// - Direction is Outgoing (the rel points TO our node)
			for modelName, model := range schema.Models {
				for _, rel := range model.Relationships {
					if rel.RelType == "" || seen[rel.RelType] {
						continue
					}

					// Only outgoing relationships can point TO our left node
					if rel.Direction != analysis.DirectionOutgoing {
						continue
					}

					// Check if this rel targets one of our left labels
					targetMatches := false
					for _, label := range cc.leftNodeLabels {
						if rel.Target == label {
							targetMatches = true
							break
						}
					}

					if !targetMatches {
						continue
					}

					seen[rel.RelType] = true

					items = append(items, scaf.QueryCompletion{
						Label:      rel.RelType,
						Kind:       scaf.QueryCompletionRelType,
						Detail:     fmt.Sprintf("%s → %s", modelName, rel.Target),
						InsertText: rel.RelType,
						SortText:   "1" + rel.RelType,
					})
				}
			}
		} else {
			// Pattern: (leftNode)-[:]- means we want relationships FROM leftNode
			// Look at the leftNode's model for outgoing relationships
			for _, label := range cc.leftNodeLabels {
				model, exists := schema.Models[label]
				if !exists {
					continue
				}

				for _, rel := range model.Relationships {
					if rel.RelType == "" || seen[rel.RelType] {
						continue
					}

					// Only show outgoing relationships from this model
					if rel.Direction != analysis.DirectionOutgoing {
						continue
					}

					seen[rel.RelType] = true

					items = append(items, scaf.QueryCompletion{
						Label:      rel.RelType,
						Kind:       scaf.QueryCompletionRelType,
						Detail:     fmt.Sprintf("%s → %s", label, rel.Target),
						InsertText: rel.RelType,
						SortText:   "1" + rel.RelType,
					})
				}
			}
		}
	}

	// If no context-specific results, show all relationship types
	if len(items) == 0 {
		for modelName, model := range schema.Models {
			for _, rel := range model.Relationships {
				if rel.RelType != "" && !seen[rel.RelType] {
					seen[rel.RelType] = true

					direction := ""
					switch rel.Direction {
					case analysis.DirectionOutgoing:
						direction = " →"
					case analysis.DirectionIncoming:
						direction = " ←"
					}

					items = append(items, scaf.QueryCompletion{
						Label:      rel.RelType,
						Kind:       scaf.QueryCompletionRelType,
						Detail:     fmt.Sprintf("%s%s %s", modelName, direction, rel.Target),
						InsertText: rel.RelType,
						SortText:   "3" + rel.RelType,
					})
				}
			}
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Label < items[j].Label
	})

	return items
}

func (d *Dialect) completeProperties(cc *completionContext, ctx *scaf.QueryLSPContext) []scaf.QueryCompletion {
	if ctx == nil || ctx.Schema == nil {
		return nil
	}

	schema, ok := ctx.Schema.(*analysis.TypeSchema)
	if !ok || schema == nil {
		return nil
	}

	var items []scaf.QueryCompletion
	seen := make(map[string]bool)

	for _, model := range schema.Models {
		for _, field := range model.Fields {
			if !seen[field.Name] {
				seen[field.Name] = true
				items = append(items, scaf.QueryCompletion{
					Label:      field.Name,
					Kind:       scaf.QueryCompletionProperty,
					Detail:     field.Type.String(),
					InsertText: field.Name,
					SortText:   "3" + field.Name,
				})
			}
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Label < items[j].Label
	})

	return items
}

func (d *Dialect) completeVariables(cc *completionContext, parsed *cyphergrammar.Script) []scaf.QueryCompletion {
	if parsed == nil || parsed.Query == nil || parsed.Query.RegularQuery == nil {
		return nil
	}

	vars := d.extractVariables(parsed)
	var items []scaf.QueryCompletion

	for varName := range vars {
		items = append(items, scaf.QueryCompletion{
			Label:      varName,
			Kind:       scaf.QueryCompletionVariable,
			Detail:     "variable",
			InsertText: varName,
			SortText:   "1" + varName,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Label < items[j].Label
	})

	return items
}

func (d *Dialect) extractVariables(parsed *cyphergrammar.Script) map[string]bool {
	vars := make(map[string]bool)

	if parsed == nil || parsed.Query == nil || parsed.Query.RegularQuery == nil {
		return vars
	}

	rq := parsed.Query.RegularQuery
	if rq.SingleQuery == nil {
		return vars
	}

	for _, clause := range rq.SingleQuery.Clauses {
		d.extractVariablesFromClause(clause, vars)
	}

	return vars
}

func (d *Dialect) extractVariablesFromClause(clause *cyphergrammar.Clause, vars map[string]bool) {
	if clause == nil {
		return
	}

	if clause.Reading != nil && clause.Reading.Match != nil && clause.Reading.Match.Pattern != nil {
		d.extractVariablesFromPattern(clause.Reading.Match.Pattern, vars)
	}

	if clause.Updating != nil {
		if clause.Updating.Create != nil && clause.Updating.Create.Pattern != nil {
			d.extractVariablesFromPattern(clause.Updating.Create.Pattern, vars)
		}
	}
}

func (d *Dialect) extractVariablesFromPattern(pattern *cyphergrammar.Pattern, vars map[string]bool) {
	if pattern == nil {
		return
	}

	for _, part := range pattern.Parts {
		if part == nil || part.Element == nil {
			continue
		}

		if part.Var != "" {
			vars[part.Var] = true
		}

		d.extractVariablesFromPatternElement(part.Element, vars)
	}
}

func (d *Dialect) extractVariablesFromPatternElement(elem *cyphergrammar.PatternElement, vars map[string]bool) {
	if elem == nil {
		return
	}

	if elem.Paren != nil {
		d.extractVariablesFromPatternElement(elem.Paren, vars)
		return
	}

	if elem.Node != nil && elem.Node.Variable != "" {
		vars[elem.Node.Variable] = true
	}

	for _, chain := range elem.Chain {
		if chain.Rel != nil && chain.Rel.Detail != nil && chain.Rel.Detail.Variable != "" {
			vars[chain.Rel.Detail.Variable] = true
		}
		if chain.Node != nil && chain.Node.Variable != "" {
			vars[chain.Node.Variable] = true
		}
	}
}

func (d *Dialect) completeParameters(cc *completionContext, ctx *scaf.QueryLSPContext) []scaf.QueryCompletion {
	if ctx == nil || ctx.DeclaredParams == nil {
		return nil
	}

	var items []scaf.QueryCompletion

	for name, typeExpr := range ctx.DeclaredParams {
		detail := "parameter"
		if typeExpr != nil {
			detail = typeExpr.ToGoType()
		}

		items = append(items, scaf.QueryCompletion{
			Label:      name,
			Kind:       scaf.QueryCompletionParameter,
			Detail:     detail,
			InsertText: name,
			SortText:   "1" + name,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Label < items[j].Label
	})

	return items
}

// Hover returns hover information for a position in a Cypher query.
func (d *Dialect) Hover(query string, offset int, ctx *scaf.QueryLSPContext) *scaf.QueryHover {
	// Get the word at the offset
	word := d.getWordAt(query, offset)
	if word == "" {
		return nil
	}

	// Check if it's a keyword
	if desc, ok := cypherKeywords[strings.ToUpper(word)]; ok {
		return &scaf.QueryHover{
			Contents: fmt.Sprintf("**%s** (keyword)\n\n%s", strings.ToUpper(word), desc),
		}
	}

	// Check if it's a function
	if retType, ok := cypherFunctionTypes[strings.ToLower(word)]; ok {
		return &scaf.QueryHover{
			Contents: fmt.Sprintf("**%s** (function)\n\nReturns: `%s`", strings.ToLower(word), retType),
		}
	}

	return nil
}

func (d *Dialect) getWordAt(query string, offset int) string {
	if offset < 0 || offset > len(query) {
		return ""
	}

	// Find word boundaries
	start := offset
	for start > 0 && (unicode.IsLetter(rune(query[start-1])) || unicode.IsDigit(rune(query[start-1])) || query[start-1] == '_') {
		start--
	}

	end := offset
	for end < len(query) && (unicode.IsLetter(rune(query[end])) || unicode.IsDigit(rune(query[end])) || query[end] == '_') {
		end++
	}

	return query[start:end]
}

// Diagnostics returns diagnostics for a Cypher query.
func (d *Dialect) Diagnostics(query string, ctx *scaf.QueryLSPContext) []scaf.QueryDiagnostic {
	var diags []scaf.QueryDiagnostic

	// Parse and check for syntax errors
	parsed, err := cyphergrammar.Parse(query)
	if err != nil {
		diags = append(diags, scaf.QueryDiagnostic{
			Range:    scaf.QueryRange{Start: 0, End: len(query)},
			Severity: scaf.QueryDiagnosticError,
			Message:  err.Error(),
			Code:     "syntax-error",
		})
	}

	// Add semantic diagnostics if we have a schema
	if parsed != nil && ctx != nil && ctx.Schema != nil {
		diags = append(diags, d.semanticDiagnostics(query, parsed, ctx)...)
	}

	return diags
}

func (d *Dialect) semanticDiagnostics(query string, parsed *cyphergrammar.Script, ctx *scaf.QueryLSPContext) []scaf.QueryDiagnostic {
	var diags []scaf.QueryDiagnostic

	schema, ok := ctx.Schema.(*analysis.TypeSchema)
	if !ok || schema == nil {
		return diags
	}

	// Collect all labels and relationship types used in the query
	labelsUsed := d.extractLabelsFromAST(parsed)
	relTypesUsed := d.extractRelTypesFromAST(parsed)

	// Build set of known labels and relationship types
	knownLabels := make(map[string]bool)
	knownRelTypes := make(map[string]bool)
	for name, model := range schema.Models {
		knownLabels[name] = true
		for _, rel := range model.Relationships {
			if rel.RelType != "" {
				knownRelTypes[rel.RelType] = true
			}
		}
	}

	// Check for unknown labels
	for label, pos := range labelsUsed {
		if !knownLabels[label] {
			diags = append(diags, scaf.QueryDiagnostic{
				Range:    scaf.QueryRange{Start: pos.start, End: pos.end},
				Severity: scaf.QueryDiagnosticWarning,
				Message:  fmt.Sprintf("Unknown node label: %s", label),
				Code:     "unknown-label",
			})
		}
	}

	// Check for unknown relationship types
	for relType, pos := range relTypesUsed {
		if !knownRelTypes[relType] {
			diags = append(diags, scaf.QueryDiagnostic{
				Range:    scaf.QueryRange{Start: pos.start, End: pos.end},
				Severity: scaf.QueryDiagnosticWarning,
				Message:  fmt.Sprintf("Unknown relationship type: %s", relType),
				Code:     "unknown-reltype",
			})
		}
	}

	// Check for type mismatches in property comparisons
	diags = append(diags, d.typeMismatchDiagnostics(parsed, schema)...)

	return diags
}

// typeMismatchDiagnostics checks for type mismatches in property comparisons.
// For example: (u:User {name: false}) where name is a string field.
func (d *Dialect) typeMismatchDiagnostics(parsed *cyphergrammar.Script, schema *analysis.TypeSchema) []scaf.QueryDiagnostic {
	var diags []scaf.QueryDiagnostic

	if parsed == nil || parsed.Query == nil {
		return diags
	}

	if rq := parsed.Query.RegularQuery; rq != nil && rq.SingleQuery != nil {
		for _, clause := range rq.SingleQuery.Clauses {
			diags = append(diags, d.checkClauseTypeMismatches(clause, schema)...)
		}
	}

	return diags
}

func (d *Dialect) checkClauseTypeMismatches(clause *cyphergrammar.Clause, schema *analysis.TypeSchema) []scaf.QueryDiagnostic {
	var diags []scaf.QueryDiagnostic

	if clause == nil {
		return diags
	}

	// Check MATCH patterns
	if clause.Reading != nil && clause.Reading.Match != nil && clause.Reading.Match.Pattern != nil {
		diags = append(diags, d.checkPatternTypeMismatches(clause.Reading.Match.Pattern, schema)...)
	}

	// Check CREATE patterns
	if clause.Updating != nil && clause.Updating.Create != nil && clause.Updating.Create.Pattern != nil {
		diags = append(diags, d.checkPatternTypeMismatches(clause.Updating.Create.Pattern, schema)...)
	}

	// Check MERGE patterns
	if clause.Updating != nil && clause.Updating.Merge != nil && clause.Updating.Merge.Pattern != nil {
		diags = append(diags, d.checkPatternElementTypeMismatches(clause.Updating.Merge.Pattern.Element, schema)...)
	}

	return diags
}

func (d *Dialect) checkPatternTypeMismatches(pattern *cyphergrammar.Pattern, schema *analysis.TypeSchema) []scaf.QueryDiagnostic {
	var diags []scaf.QueryDiagnostic

	if pattern == nil {
		return diags
	}

	for _, part := range pattern.Parts {
		if part != nil && part.Element != nil {
			diags = append(diags, d.checkPatternElementTypeMismatches(part.Element, schema)...)
		}
	}

	return diags
}

func (d *Dialect) checkPatternElementTypeMismatches(elem *cyphergrammar.PatternElement, schema *analysis.TypeSchema) []scaf.QueryDiagnostic {
	var diags []scaf.QueryDiagnostic

	if elem == nil {
		return diags
	}

	// Handle parenthesized pattern
	if elem.Paren != nil {
		return d.checkPatternElementTypeMismatches(elem.Paren, schema)
	}

	// Check node pattern
	if elem.Node != nil {
		diags = append(diags, d.checkNodePatternTypeMismatches(elem.Node, schema)...)
	}

	// Check chain elements
	for _, chain := range elem.Chain {
		if chain.Node != nil {
			diags = append(diags, d.checkNodePatternTypeMismatches(chain.Node, schema)...)
		}
	}

	return diags
}

func (d *Dialect) checkNodePatternTypeMismatches(node *cyphergrammar.NodePattern, schema *analysis.TypeSchema) []scaf.QueryDiagnostic {
	var diags []scaf.QueryDiagnostic

	if node == nil || node.Properties == nil || node.Properties.Map == nil {
		return diags
	}

	// Get the labels for this node
	var labels []string
	if node.Labels != nil {
		labels = node.Labels.Labels
	}

	if len(labels) == 0 {
		return diags // No labels to check against
	}

	// Check each property in the map
	for _, pair := range node.Properties.Map.Pairs {
		if pair == nil || pair.Value == nil {
			continue
		}

		propName := pair.Key
		expectedType := d.lookupPropertyType(propName, labels, schema)
		if expectedType == nil {
			continue // Unknown property, skip
		}

		actualType := d.inferLiteralType(pair.Value)
		if actualType == "" {
			continue // Can't infer type (e.g., parameter), skip
		}

		if !d.typesCompatible(expectedType, actualType) {
			diags = append(diags, scaf.QueryDiagnostic{
				Range:    scaf.QueryRange{Start: pair.Value.Pos.Offset, End: d.expressionEndOffset(pair.Value)},
				Severity: scaf.QueryDiagnosticWarning,
				Message:  fmt.Sprintf("Type mismatch: property '%s' expects %s, got %s", propName, expectedType.Name, actualType),
				Code:     "type-mismatch",
			})
		}
	}

	return diags
}

// lookupPropertyType finds the type of a property from the schema.
func (d *Dialect) lookupPropertyType(propName string, labels []string, schema *analysis.TypeSchema) *analysis.Type {
	for _, label := range labels {
		if model, ok := schema.Models[label]; ok {
			for _, field := range model.Fields {
				if field.Name == propName && field.Type != nil {
					return field.Type
				}
			}
		}
	}
	return nil
}

// inferLiteralType infers the type of a literal expression.
// Returns empty string if type cannot be inferred (e.g., for parameters or complex expressions).
func (d *Dialect) inferLiteralType(expr *cyphergrammar.Expression) string {
	if expr == nil {
		return ""
	}

	// Navigate through the expression tree to find a literal
	atom := d.extractAtomFromExpression(expr)
	if atom == nil {
		return ""
	}

	// Check literal types
	if atom.Literal != nil {
		lit := atom.Literal
		if lit.True || lit.False {
			return "boolean"
		}
		if lit.Int != nil {
			return "integer"
		}
		if lit.Float != nil {
			return "float"
		}
		if lit.String != nil {
			return "string"
		}
		if lit.Null {
			return "" // null is compatible with any nullable type
		}
		if lit.List != nil {
			return "list"
		}
		if lit.Map != nil {
			return "map"
		}
	}

	// Parameter - we can't know the type
	if atom.Parameter != nil {
		return ""
	}

	return ""
}

// extractAtomFromExpression navigates the expression tree to find the atom.
func (d *Dialect) extractAtomFromExpression(expr *cyphergrammar.Expression) *cyphergrammar.Atom {
	if expr == nil || expr.Left == nil {
		return nil
	}

	xor := expr.Left
	if xor == nil || xor.Left == nil {
		return nil
	}

	and := xor.Left
	if and == nil || and.Left == nil {
		return nil
	}

	not := and.Left
	if not == nil || not.Expr == nil {
		return nil
	}

	comp := not.Expr
	if comp == nil || comp.Left == nil {
		return nil
	}

	add := comp.Left
	if add == nil || add.Left == nil {
		return nil
	}

	mult := add.Left
	if mult == nil || mult.Left == nil {
		return nil
	}

	pow := mult.Left
	if pow == nil || pow.Left == nil {
		return nil
	}

	unary := pow.Left
	if unary == nil || unary.Expr == nil {
		return nil
	}

	return unary.Expr.Atom
}

// typesCompatible checks if a value type is compatible with an expected schema type.
func (d *Dialect) typesCompatible(expected *analysis.Type, actual string) bool {
	if expected == nil {
		return true
	}

	expectedName := strings.ToLower(expected.Name)

	switch expectedName {
	case "string":
		return actual == "string"
	case "int", "integer", "int64", "int32":
		return actual == "integer"
	case "float", "float64", "float32", "double":
		return actual == "integer" || actual == "float" // integers can be used as floats
	case "bool", "boolean":
		return actual == "boolean"
	case "[]string", "[]int", "[]integer", "[]float", "[]bool", "[]boolean":
		return actual == "list"
	default:
		// For complex types (models, etc.), be permissive
		return true
	}
}

// expressionEndOffset calculates the end offset of an expression.
func (d *Dialect) expressionEndOffset(expr *cyphergrammar.Expression) int {
	if expr == nil {
		return 0
	}

	// For simple literals, estimate based on type
	atom := d.extractAtomFromExpression(expr)
	if atom != nil && atom.Literal != nil {
		lit := atom.Literal
		if lit.True {
			return expr.Pos.Offset + 4 // "true"
		}
		if lit.False {
			return expr.Pos.Offset + 5 // "false"
		}
		if lit.Null {
			return expr.Pos.Offset + 4 // "null"
		}
		if lit.Int != nil {
			return expr.Pos.Offset + len(fmt.Sprintf("%d", *lit.Int))
		}
		if lit.Float != nil {
			return expr.Pos.Offset + len(fmt.Sprintf("%g", *lit.Float))
		}
		if lit.String != nil {
			return expr.Pos.Offset + len(*lit.String) + 2 // include quotes
		}
	}

	// Fallback: use position + some reasonable length
	return expr.Pos.Offset + 5
}

type labelPos struct {
	start int
	end   int
}

func (d *Dialect) extractLabelsFromAST(script *cyphergrammar.Script) map[string]labelPos {
	labels := make(map[string]labelPos)

	if script == nil || script.Query == nil {
		return labels
	}

	if rq := script.Query.RegularQuery; rq != nil && rq.SingleQuery != nil {
		for _, clause := range rq.SingleQuery.Clauses {
			d.extractLabelsFromClause(clause, labels)
		}
	}

	return labels
}

func (d *Dialect) extractLabelsFromClause(clause *cyphergrammar.Clause, labels map[string]labelPos) {
	if clause == nil {
		return
	}

	if clause.Reading != nil {
		if clause.Reading.Match != nil && clause.Reading.Match.Pattern != nil {
			d.extractLabelsFromPattern(clause.Reading.Match.Pattern, labels)
		}
	}

	if clause.Updating != nil {
		if clause.Updating.Create != nil && clause.Updating.Create.Pattern != nil {
			d.extractLabelsFromPattern(clause.Updating.Create.Pattern, labels)
		}
		if clause.Updating.Merge != nil && clause.Updating.Merge.Pattern != nil {
			d.extractLabelsFromPatternPart(clause.Updating.Merge.Pattern, labels)
		}
	}
}

func (d *Dialect) extractLabelsFromPattern(pattern *cyphergrammar.Pattern, labels map[string]labelPos) {
	if pattern == nil {
		return
	}

	for _, part := range pattern.Parts {
		d.extractLabelsFromPatternPart(part, labels)
	}
}

func (d *Dialect) extractLabelsFromPatternPart(part *cyphergrammar.PatternPart, labels map[string]labelPos) {
	if part == nil || part.Element == nil {
		return
	}

	d.extractLabelsFromPatternElement(part.Element, labels)
}

func (d *Dialect) extractLabelsFromPatternElement(elem *cyphergrammar.PatternElement, labels map[string]labelPos) {
	if elem == nil {
		return
	}

	if elem.Paren != nil {
		d.extractLabelsFromPatternElement(elem.Paren, labels)
		return
	}

	if elem.Node != nil && elem.Node.Labels != nil {
		currentOffset := elem.Node.Labels.Pos.Offset + 1
		for _, label := range elem.Node.Labels.Labels {
			if label != "" {
				labels[label] = labelPos{
					start: currentOffset,
					end:   currentOffset + len(label),
				}
			}
			currentOffset += len(label) + 1
		}
	}

	for _, chain := range elem.Chain {
		if chain.Node != nil && chain.Node.Labels != nil {
			currentOffset := chain.Node.Labels.Pos.Offset + 1
			for _, label := range chain.Node.Labels.Labels {
				if label != "" {
					labels[label] = labelPos{
						start: currentOffset,
						end:   currentOffset + len(label),
					}
				}
				currentOffset += len(label) + 1
			}
		}
	}
}

func (d *Dialect) extractRelTypesFromAST(script *cyphergrammar.Script) map[string]labelPos {
	relTypes := make(map[string]labelPos)

	if script == nil || script.Query == nil {
		return relTypes
	}

	if rq := script.Query.RegularQuery; rq != nil && rq.SingleQuery != nil {
		for _, clause := range rq.SingleQuery.Clauses {
			d.extractRelTypesFromClause(clause, relTypes)
		}
	}

	return relTypes
}

func (d *Dialect) extractRelTypesFromClause(clause *cyphergrammar.Clause, relTypes map[string]labelPos) {
	if clause == nil {
		return
	}

	if clause.Reading != nil {
		if clause.Reading.Match != nil && clause.Reading.Match.Pattern != nil {
			d.extractRelTypesFromPattern(clause.Reading.Match.Pattern, relTypes)
		}
	}

	if clause.Updating != nil {
		if clause.Updating.Create != nil && clause.Updating.Create.Pattern != nil {
			d.extractRelTypesFromPattern(clause.Updating.Create.Pattern, relTypes)
		}
	}
}

func (d *Dialect) extractRelTypesFromPattern(pattern *cyphergrammar.Pattern, relTypes map[string]labelPos) {
	if pattern == nil {
		return
	}

	for _, part := range pattern.Parts {
		if part == nil || part.Element == nil {
			continue
		}
		d.extractRelTypesFromPatternElement(part.Element, relTypes)
	}
}

func (d *Dialect) extractRelTypesFromPatternElement(elem *cyphergrammar.PatternElement, relTypes map[string]labelPos) {
	if elem == nil {
		return
	}

	if elem.Paren != nil {
		d.extractRelTypesFromPatternElement(elem.Paren, relTypes)
		return
	}

	for _, chain := range elem.Chain {
		if chain.Rel != nil && chain.Rel.Detail != nil && chain.Rel.Detail.Types != nil {
			currentOffset := chain.Rel.Detail.Types.Pos.Offset + 1
			for i, relType := range chain.Rel.Detail.Types.Types {
				if relType != "" {
					relTypes[relType] = labelPos{
						start: currentOffset,
						end:   currentOffset + len(relType),
					}
				}
				if i < len(chain.Rel.Detail.Types.Types)-1 {
					currentOffset += len(relType) + 1
				}
			}
		}
	}
}

// SignatureHelp returns signature help for function calls.
func (d *Dialect) SignatureHelp(query string, offset int, ctx *scaf.QueryLSPContext) *scaf.QuerySignatureHelp {
	// Find if we're inside a function call
	funcName, paramIdx := d.findFunctionContext(query, offset)
	if funcName == "" {
		return nil
	}

	sig := d.getFunctionSignature(funcName)
	if sig == nil {
		return nil
	}

	return &scaf.QuerySignatureHelp{
		Signatures:      []scaf.QuerySignature{*sig},
		ActiveSignature: 0,
		ActiveParameter: paramIdx,
	}
}

func (d *Dialect) findFunctionContext(query string, offset int) (string, int) {
	if offset <= 0 || offset > len(query) {
		return "", 0
	}

	// Look backwards for function call
	parenDepth := 0
	commaCount := 0

	for i := offset - 1; i >= 0; i-- {
		switch query[i] {
		case ')':
			parenDepth++
		case '(':
			if parenDepth == 0 {
				// Found the opening paren, extract function name
				funcEnd := i
				funcStart := funcEnd
				for funcStart > 0 && (unicode.IsLetter(rune(query[funcStart-1])) || unicode.IsDigit(rune(query[funcStart-1])) || query[funcStart-1] == '_') {
					funcStart--
				}
				if funcStart < funcEnd {
					return strings.ToLower(query[funcStart:funcEnd]), commaCount
				}
				return "", 0
			}
			parenDepth--
		case ',':
			if parenDepth == 0 {
				commaCount++
			}
		}
	}

	return "", 0
}

func (d *Dialect) getFunctionSignature(name string) *scaf.QuerySignature {
	// Common function signatures
	signatures := map[string]*scaf.QuerySignature{
		"count": {
			Label:         "count(expression) → integer",
			Documentation: "Returns the number of values/rows.",
			Parameters: []scaf.QueryParameterInfo{
				{Label: "expression", Documentation: "Expression to count. Use * to count rows."},
			},
		},
		"sum": {
			Label:         "sum(expression) → number",
			Documentation: "Returns the sum of numeric values.",
			Parameters: []scaf.QueryParameterInfo{
				{Label: "expression", Documentation: "Numeric expression to sum."},
			},
		},
		"avg": {
			Label:         "avg(expression) → float",
			Documentation: "Returns the average of numeric values.",
			Parameters: []scaf.QueryParameterInfo{
				{Label: "expression", Documentation: "Numeric expression to average."},
			},
		},
		"collect": {
			Label:         "collect(expression) → list",
			Documentation: "Returns a list containing all values.",
			Parameters: []scaf.QueryParameterInfo{
				{Label: "expression", Documentation: "Values to collect into a list."},
			},
		},
		"coalesce": {
			Label:         "coalesce(expression...) → any",
			Documentation: "Returns the first non-null value.",
			Parameters: []scaf.QueryParameterInfo{
				{Label: "expression", Documentation: "Expressions to evaluate."},
			},
		},
	}

	return signatures[name]
}

// Definition returns go-to-definition targets.
func (d *Dialect) Definition(query string, offset int, ctx *scaf.QueryLSPContext) []scaf.QueryLocation {
	// TODO: Implement go-to-definition for labels/relationship types
	return nil
}

// InlayHints returns inlay hints for inferred parameter types.
// Shows type annotations for parameters that lack explicit types in the function signature.
func (d *Dialect) InlayHints(query string, ctx *scaf.QueryLSPContext) []scaf.QueryInlayHint {
	if ctx == nil {
		return nil
	}

	// Get schema for type inference
	schema, _ := ctx.Schema.(*analysis.TypeSchema)
	if schema == nil {
		return nil
	}

	// Use the analyzer to extract parameters with inferred types
	analyzer := NewAnalyzer()
	metadata, err := analyzer.AnalyzeQueryWithSchema(query, schema)
	if err != nil || metadata == nil {
		return nil
	}

	var hints []scaf.QueryInlayHint

	for _, param := range metadata.Parameters {
		// Skip if the parameter already has an explicit type annotation
		if declType, exists := ctx.DeclaredParams[param.Name]; exists && declType != nil {
			continue
		}

		// Skip if we couldn't infer a type
		if param.Type == nil {
			continue
		}

		// Create an inlay hint for this parameter
		hints = append(hints, scaf.QueryInlayHint{
			ParameterName: param.Name,
			Label:         ": " + param.Type.String(),
			Kind:          scaf.QueryInlayHintType,
			Tooltip:       fmt.Sprintf("Type inferred from query usage in schema"),
		})
	}

	return hints
}
