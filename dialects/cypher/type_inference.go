package cypher

import (
	"slices"
	"strings"

	"github.com/rlch/scaf/analysis"
	cyphergrammar "github.com/rlch/scaf/dialects/cypher/grammar"
)

// ----------------------------------------------------------------------------
// Expression Type Inference
//
// This file implements type inference for Cypher expressions by walking the
// participle parse tree. Each grammar rule has a corresponding inference function.
// ----------------------------------------------------------------------------

// inferExpressionType is the entry point for expression type inference.
func inferExpressionType(expr *cyphergrammar.Expression, qctx *queryContext) *analysis.Type {
	if expr == nil {
		return nil
	}
	return inferExpression(expr, qctx)
}

// inferFieldInfo extracts FieldInfo (type + required) for property access expressions.
// Returns nil for non-field expressions (literals, function calls, etc).
func inferFieldInfo(expr *cyphergrammar.Expression, qctx *queryContext) *FieldInfo {
	if expr == nil || qctx.schema == nil {
		return nil
	}

	// Drill down to the postfix expression (where property access lives)
	post := getPostfixExpr(expr)
	if post == nil || post.Atom == nil {
		return nil
	}

	// Need a variable reference as base
	varName := post.Atom.Variable
	if varName == "" {
		return nil
	}

	// Check if it's a bound variable (from MATCH)
	binding, ok := qctx.bindings[varName]
	if !ok || len(binding.labels) == 0 {
		return nil
	}

	// Find property access suffix
	for _, suffix := range post.Suffixes {
		if suffix != nil && suffix.Property != "" {
			return LookupFieldInfo(binding.labels[0], suffix.Property, qctx.schema)
		}
	}

	return nil
}

// getPostfixExpr drills down through the expression tree to get the PostfixExpr.
func getPostfixExpr(expr *cyphergrammar.Expression) *cyphergrammar.PostfixExpr {
	if expr == nil || expr.Left == nil {
		return nil
	}
	xor := expr.Left
	if xor.Left == nil {
		return nil
	}
	and := xor.Left
	if and.Left == nil {
		return nil
	}
	not := and.Left
	if not.Expr == nil {
		return nil
	}
	comp := not.Expr
	if comp.Left == nil {
		return nil
	}
	add := comp.Left
	if add.Left == nil {
		return nil
	}
	mult := add.Left
	if mult.Left == nil {
		return nil
	}
	pow := mult.Left
	if pow.Left == nil {
		return nil
	}
	unary := pow.Left
	return unary.Expr
}

// inferExpression handles the expression rule.
// expression: xorExpression (OR xorExpression)*
func inferExpression(expr *cyphergrammar.Expression, qctx *queryContext) *analysis.Type {
	if expr == nil {
		return nil
	}

	// OR produces bool
	if len(expr.Right) > 0 {
		return analysis.TypeBool
	}

	return inferXorExpression(expr.Left, qctx)
}

// inferXorExpression handles the xorExpression rule.
func inferXorExpression(xor *cyphergrammar.XorExpr, qctx *queryContext) *analysis.Type {
	if xor == nil {
		return nil
	}

	// XOR produces bool
	if len(xor.Right) > 0 {
		return analysis.TypeBool
	}

	return inferAndExpression(xor.Left, qctx)
}

// inferAndExpression handles the andExpression rule.
func inferAndExpression(and *cyphergrammar.AndExpr, qctx *queryContext) *analysis.Type {
	if and == nil {
		return nil
	}

	// AND produces bool
	if len(and.Right) > 0 {
		return analysis.TypeBool
	}

	return inferNotExpression(and.Left, qctx)
}

// inferNotExpression handles the notExpression rule.
func inferNotExpression(not *cyphergrammar.NotExpr, qctx *queryContext) *analysis.Type {
	if not == nil {
		return nil
	}

	// NOT produces bool
	if not.Not {
		return analysis.TypeBool
	}

	return inferComparisonExpression(not.Expr, qctx)
}

// inferComparisonExpression handles the comparisonExpression rule.
func inferComparisonExpression(comp *cyphergrammar.ComparisonExpr, qctx *queryContext) *analysis.Type {
	if comp == nil {
		return nil
	}

	// Comparisons produce bool
	if len(comp.Right) > 0 {
		return analysis.TypeBool
	}

	return inferAddSubExpression(comp.Left, qctx)
}

// inferAddSubExpression handles the addSubExpression rule.
func inferAddSubExpression(add *cyphergrammar.AddSubExpr, qctx *queryContext) *analysis.Type {
	if add == nil {
		return nil
	}

	if len(add.Right) == 0 {
		return inferMultDivExpression(add.Left, qctx)
	}

	// Multiple operands with + or -
	var types []*analysis.Type
	types = append(types, inferMultDivExpression(add.Left, qctx))
	for _, term := range add.Right {
		types = append(types, inferMultDivExpression(term.Expr, qctx))
	}

	// String concatenation
	if slices.ContainsFunc(types, isStringType) {
		return analysis.TypeString
	}

	return unifyNumericTypes(types...)
}

// inferMultDivExpression handles the multDivExpression rule.
func inferMultDivExpression(mult *cyphergrammar.MultDivExpr, qctx *queryContext) *analysis.Type {
	if mult == nil {
		return nil
	}

	if len(mult.Right) == 0 {
		return inferPowerExpression(mult.Left, qctx)
	}

	// Check for division - always produces float64
	for _, term := range mult.Right {
		if term.Op == "/" {
			return analysis.TypeFloat64
		}
	}

	// Multiplication/modulo - unify numeric types
	var types []*analysis.Type
	types = append(types, inferPowerExpression(mult.Left, qctx))
	for _, term := range mult.Right {
		types = append(types, inferPowerExpression(term.Expr, qctx))
	}
	return unifyNumericTypes(types...)
}

// inferPowerExpression handles the powerExpression rule.
func inferPowerExpression(pow *cyphergrammar.PowerExpr, qctx *queryContext) *analysis.Type {
	if pow == nil {
		return nil
	}

	// Power always produces float64
	if len(pow.Right) > 0 {
		return analysis.TypeFloat64
	}

	return inferUnaryExpression(pow.Left, qctx)
}

// inferUnaryExpression handles the unaryExpression rule.
func inferUnaryExpression(unary *cyphergrammar.UnaryExpr, qctx *queryContext) *analysis.Type {
	if unary == nil {
		return nil
	}

	return inferPostfixExpression(unary.Expr, qctx)
}

// inferPostfixExpression handles the postfixExpression rule.
func inferPostfixExpression(post *cyphergrammar.PostfixExpr, qctx *queryContext) *analysis.Type {
	if post == nil {
		return nil
	}

	baseType := inferAtom(post.Atom, qctx)

	// Apply suffixes
	for _, suffix := range post.Suffixes {
		baseType = applySuffix(baseType, suffix, qctx)
	}

	return baseType
}

// applySuffix applies a postfix suffix to a base type.
func applySuffix(baseType *analysis.Type, suffix *cyphergrammar.PostfixSuffix, qctx *queryContext) *analysis.Type {
	if suffix == nil {
		return baseType
	}

	// Property access: .property
	if suffix.Property != "" {
		return resolvePropertyType(baseType, suffix.Property, qctx)
	}

	// Index access: [expr] or [start..end]
	if suffix.Index != nil {
		if suffix.Index.Range {
			// Range - same type
			return baseType
		}
		// Single index
		if baseType != nil && baseType.Kind == analysis.TypeKindSlice {
			return baseType.Elem
		}
		if isStringType(baseType) {
			return analysis.TypeString
		}
		if baseType != nil && baseType.Kind == analysis.TypeKindMap {
			return baseType.Elem
		}
		return nil
	}

	// IS NULL / IS NOT NULL
	if suffix.IsNull != nil {
		return analysis.TypeBool
	}

	// IN expression
	if suffix.In != nil {
		return analysis.TypeBool
	}

	// String predicates
	if suffix.StringPred != nil {
		return analysis.TypeBool
	}

	// Label predicate
	if suffix.Labels != nil {
		return analysis.TypeBool
	}

	return baseType
}

// resolvePropertyType looks up the type of a property on a given type.
func resolvePropertyType(baseType *analysis.Type, propName string, qctx *queryContext) *analysis.Type {
	if baseType == nil {
		return nil
	}

	// Pointer to model (*User) - look up field in schema
	if baseType.Kind == analysis.TypeKindPointer && baseType.Elem != nil {
		return lookupModelField(baseType.Elem.Name, propName, qctx)
	}

	// Named type (User) - look up field in schema
	if baseType.Kind == analysis.TypeKindNamed {
		return lookupModelField(baseType.Name, propName, qctx)
	}

	// Map - property access returns value type
	if baseType.Kind == analysis.TypeKindMap {
		return baseType.Elem
	}

	return nil
}

// lookupModelField finds a field's type from the schema.
func lookupModelField(modelName, fieldName string, qctx *queryContext) *analysis.Type {
	if qctx.schema == nil {
		return nil
	}

	model, ok := qctx.schema.Models[modelName]
	if !ok {
		return nil
	}

	for _, field := range model.Fields {
		if field.Name == fieldName {
			return field.Type
		}
	}

	return nil
}

// FieldInfo holds type and required info for a field lookup.
type FieldInfo struct {
	Type     *analysis.Type
	Required bool
}

// LookupFieldInfo looks up a field from the schema and returns both type and required info.
// This is used by extractProjectionItem to pass Required through to ReturnInfo.
func LookupFieldInfo(modelName, fieldName string, schema *analysis.TypeSchema) *FieldInfo {
	if schema == nil {
		return nil
	}

	model, ok := schema.Models[modelName]
	if !ok {
		return nil
	}

	for _, field := range model.Fields {
		if field.Name == fieldName {
			return &FieldInfo{
				Type:     field.Type,
				Required: field.Required,
			}
		}
	}

	return nil
}

// inferAtom handles the atom rule.
func inferAtom(atom *cyphergrammar.Atom, qctx *queryContext) *analysis.Type {
	if atom == nil {
		return nil
	}

	// Literal
	if atom.Literal != nil {
		return inferLiteral(atom.Literal, qctx)
	}

	// Parameter
	if atom.Parameter != nil {
		return nil // Parameters are untyped
	}

	// count(*)
	if atom.CountAll {
		return analysis.TypeInt
	}

	// Function call
	if atom.FunctionCall != nil {
		return inferFunctionInvocation(atom.FunctionCall, qctx)
	}

	// Variable
	if atom.Variable != "" {
		return inferSymbol(atom.Variable, qctx)
	}

	// Parenthesized expression
	if atom.Parenthesized != nil {
		return inferExpression(atom.Parenthesized, qctx)
	}

	// CASE expression
	if atom.CaseExpr != nil {
		return inferCaseExpression(atom.CaseExpr, qctx)
	}

	// List comprehension
	if atom.ListComprehension != nil {
		return inferListComprehension(atom.ListComprehension, qctx)
	}

	// Pattern comprehension
	if atom.PatternComprehension != nil {
		return inferPatternComprehension(atom.PatternComprehension, qctx)
	}

	// Filter predicate (ALL/ANY/NONE/SINGLE)
	if atom.FilterPredicate != nil {
		return analysis.TypeBool
	}

	// EXISTS subquery
	if atom.ExistsSubquery != nil {
		return analysis.TypeBool
	}

	return nil
}

// inferSymbol looks up a variable in the query context.
func inferSymbol(name string, qctx *queryContext) *analysis.Type {
	// Check if this is actually a numeric literal parsed as a symbol
	if isNumericString(name) {
		if strings.Contains(name, ".") || strings.ContainsAny(name, "eE") {
			return analysis.TypeFloat64
		}
		return analysis.TypeInt
	}

	// Check local variable bindings first
	if typ, ok := qctx.locals[name]; ok {
		return typ
	}

	// Look up as variable binding from MATCH clauses
	if binding, ok := qctx.bindings[name]; ok {
		if len(binding.labels) > 0 {
			return analysis.PointerTo(&analysis.Type{
				Kind: analysis.TypeKindNamed,
				Name: binding.labels[0],
			})
		}
	}

	return nil
}

// isNumericString checks if a string represents a number.
func isNumericString(s string) bool {
	if s == "" {
		return false
	}

	start := 0
	if s[0] == '-' || s[0] == '+' {
		start = 1
		if len(s) == 1 {
			return false
		}
	}

	hasDigit := false
	hasDot := false
	hasExp := false

	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			hasDigit = true
		case c == '.':
			if hasDot || hasExp {
				return false
			}
			hasDot = true
		case c == 'e' || c == 'E':
			if hasExp || !hasDigit {
				return false
			}
			hasExp = true
			hasDigit = false
			if i+1 < len(s) && (s[i+1] == '+' || s[i+1] == '-') {
				i++
			}
		case c == '_':
			continue
		default:
			return false
		}
	}

	return hasDigit
}

// inferLiteral determines the type of a literal value.
func inferLiteral(lit *cyphergrammar.Literal, qctx *queryContext) *analysis.Type {
	if lit == nil {
		return nil
	}

	if lit.True || lit.False {
		return analysis.TypeBool
	}

	if lit.Int != nil || lit.HexInt != nil || lit.OctInt != nil {
		return analysis.TypeInt
	}

	if lit.Float != nil {
		return analysis.TypeFloat64
	}

	if lit.String != nil {
		return analysis.TypeString
	}

	if lit.Null {
		return nil
	}

	if lit.List != nil {
		return inferListLiteral(lit.List, qctx)
	}

	if lit.Map != nil {
		return analysis.MapOf(analysis.TypeString, nil)
	}

	return nil
}

// inferListLiteral infers the type of a list literal.
func inferListLiteral(list *cyphergrammar.ListLiteral, qctx *queryContext) *analysis.Type {
	if list == nil || len(list.Items) == 0 {
		return analysis.SliceOf(nil)
	}

	// Infer from first element
	elemType := inferExpression(list.Items[0], qctx)
	return analysis.SliceOf(elemType)
}

// inferCaseExpression infers the type of a CASE expression.
func inferCaseExpression(caseExpr *cyphergrammar.CaseExpression, qctx *queryContext) *analysis.Type {
	if caseExpr == nil {
		return nil
	}

	// Infer from THEN expressions
	for _, when := range caseExpr.Whens {
		if t := inferExpression(when.Then, qctx); t != nil {
			return t
		}
	}

	// Fall back to ELSE
	if caseExpr.Else != nil {
		return inferExpression(caseExpr.Else, qctx)
	}

	return nil
}

// inferListComprehension infers the type of a list comprehension.
func inferListComprehension(lc *cyphergrammar.ListComprehension, qctx *queryContext) *analysis.Type {
	if lc == nil {
		return analysis.SliceOf(nil)
	}

	// Get element type from source list
	var elemType *analysis.Type
	if lc.Source != nil {
		listType := inferExpression(lc.Source, qctx)
		if listType != nil && listType.Kind == analysis.TypeKindSlice {
			elemType = listType.Elem
		}
	}

	// Create scoped context with loop variable
	scopedCtx := qctx
	if lc.Variable != "" && elemType != nil {
		scopedCtx = qctx.withLocal(lc.Variable, elemType)
	}

	// If there's a mapping expression, result is list of that type
	if lc.Mapping != nil {
		mappedType := inferExpression(lc.Mapping, scopedCtx)
		return analysis.SliceOf(mappedType)
	}

	// No mapping - preserves element type
	return analysis.SliceOf(elemType)
}

// inferPatternComprehension infers the type of a pattern comprehension.
func inferPatternComprehension(pc *cyphergrammar.PatternComprehension, qctx *queryContext) *analysis.Type {
	if pc == nil {
		return analysis.SliceOf(nil)
	}

	// Create scoped context with pattern bindings
	scopedCtx := qctx
	if pc.Pattern != nil {
		scopedCtx = extractPatternBindings(pc.Pattern, qctx)
	}

	// Result is list of the mapped expression type
	if pc.Mapping != nil {
		elemType := inferExpression(pc.Mapping, scopedCtx)
		return analysis.SliceOf(elemType)
	}

	return analysis.SliceOf(nil)
}

// extractPatternBindings extracts variable bindings from a pattern.
func extractPatternBindings(pattern *cyphergrammar.RelationshipChainPattern, qctx *queryContext) *queryContext {
	if pattern == nil {
		return qctx
	}

	scopedCtx := qctx

	// Extract from first node
	if pattern.Node != nil {
		scopedCtx = extractNodePatternBinding(pattern.Node, scopedCtx)
	}

	// Extract from chain
	for _, chain := range pattern.Chain {
		if chain.Node != nil {
			scopedCtx = extractNodePatternBinding(chain.Node, scopedCtx)
		}
	}

	return scopedCtx
}

// extractNodePatternBinding extracts a variable binding from a node pattern.
func extractNodePatternBinding(node *cyphergrammar.NodePattern, qctx *queryContext) *queryContext {
	if node == nil || node.Variable == "" {
		return qctx
	}

	var labels []string
	if node.Labels != nil {
		labels = node.Labels.Labels
	}

	if len(labels) > 0 {
		varType := analysis.PointerTo(&analysis.Type{
			Kind: analysis.TypeKindNamed,
			Name: labels[0],
		})
		return qctx.withLocal(node.Variable, varType)
	}

	return qctx
}

// ----------------------------------------------------------------------------
// Type Helpers
// ----------------------------------------------------------------------------

// isStringType checks if a type is string.
func isStringType(t *analysis.Type) bool {
	return t != nil && t.Kind == analysis.TypeKindPrimitive && t.Name == "string"
}

// isFloatType checks if a type is floating point.
func isFloatType(t *analysis.Type) bool {
	return t != nil && t.Kind == analysis.TypeKindPrimitive &&
		(t.Name == "float64" || t.Name == "float32")
}

// isIntType checks if a type is integer.
func isIntType(t *analysis.Type) bool {
	if t == nil || t.Kind != analysis.TypeKindPrimitive {
		return false
	}
	switch t.Name {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64":
		return true
	}
	return false
}

// unifyNumericTypes returns the "widest" numeric type from the inputs.
func unifyNumericTypes(types ...*analysis.Type) *analysis.Type {
	var hasFloat, hasInt bool

	for _, t := range types {
		if t == nil {
			continue
		}
		if isFloatType(t) {
			hasFloat = true
		} else if isIntType(t) {
			hasInt = true
		}
	}

	if hasFloat {
		return analysis.TypeFloat64
	}
	if hasInt {
		return analysis.TypeInt
	}
	return nil
}
