// Package scaf provides a DSL parser for database test scaffolding.
package scaf

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
)

// =============================================================================
// Common embedded types for AST nodes
// =============================================================================

// NodeMeta contains position and token information common to all AST nodes.
// Participle automatically populates these fields during parsing.
type NodeMeta struct {
	Pos    lexer.Position `parser:""`
	EndPos lexer.Position `parser:""`
	Tokens []lexer.Token  `parser:""`
}

// Span returns the source span of this node.
func (n *NodeMeta) Span() Span { return Span{Start: n.Pos, End: n.EndPos} }

// CommentMeta holds comments attached to a node (populated after parsing).
type CommentMeta struct {
	LeadingComments []string `parser:""`
	TrailingComment string   `parser:""`
}

// RecoveryMeta holds recovery metadata for nodes that support error recovery.
// If RecoveredSpan is non-zero, it indicates recovery happened during parsing.
// Participle automatically populates these fields when recovery occurs.
type RecoveryMeta struct {
	// RecoveredSpan is the position where the parse error occurred.
	RecoveredSpan lexer.Position `parser:""`
	// RecoveredEnd is the position where recovery ended (after skipped tokens).
	RecoveredEnd lexer.Position `parser:""`
	// RecoveredTokens are the tokens that were skipped during recovery.
	// These can be used to understand what the user was typing when the error occurred.
	RecoveredTokens []lexer.Token `parser:""`
}

// WasRecovered returns true if this node was recovered from a parse error.
func (r *RecoveryMeta) WasRecovered() bool {
	return r.RecoveredSpan.Line != 0 || r.RecoveredSpan.Column != 0
}

// RecoveredText returns the text that was skipped during recovery.
func (r *RecoveryMeta) RecoveredText() string {
	if len(r.RecoveredTokens) == 0 {
		return ""
	}
	var b strings.Builder
	for _, tok := range r.RecoveredTokens {
		b.WriteString(tok.Value)
	}
	return b.String()
}

// LastRecoveredToken returns the last token that was recovered, or nil if none.
func (r *RecoveryMeta) LastRecoveredToken() *lexer.Token {
	if len(r.RecoveredTokens) == 0 {
		return nil
	}
	return &r.RecoveredTokens[len(r.RecoveredTokens)-1]
}

// =============================================================================
// Interfaces
// =============================================================================

// Node is the interface implemented by all AST nodes.
// It provides access to position information for error reporting and formatting.
type Node interface {
	Span() Span
}

// CompletableNode is implemented by AST nodes that can detect incomplete syntax
// for providing intelligent completions during typing.
type CompletableNode interface {
	Node
	// IsComplete returns true if the node is syntactically complete.
	// Incomplete nodes (e.g., "setup fixtures." without function name) are used
	// to provide context-aware completions.
	IsComplete() bool
}

// =============================================================================
// Top-level AST nodes
// =============================================================================

// File represents a complete scaf file with functions, setup, teardown, and test scopes.
// (Renamed from Suite for clarity - a file contains function definitions and tests)
type File struct {
	NodeMeta
	CommentMeta
	RecoveryMeta

	Imports   []*Import        `parser:"@@*"`
	Functions []*Function      `parser:"@@*"`
	Setup     *SetupClause     `parser:"('setup' @@)?"`
	Teardown  *string          `parser:"('teardown' @RawString)?"`
	Scopes    []*FunctionScope `parser:"@@*"`
}

// Suite is an alias for File for backward compatibility.
//
// Deprecated: Use File instead.
type Suite = File

// Import represents a module import statement.
// Examples:
//
//	import "../../setup/lesson_plan_db"
//	import fixtures "../shared/fixtures"
type Import struct {
	NodeMeta
	CommentMeta
	RecoveryMeta

	Alias *string `parser:"'import' @Ident?"`
	Path  string  `parser:"@String"`
}

// Function defines a named database query function with typed parameters.
// Examples:
//
//	fn GetUser(id: string) `MATCH (u:User {id: $id}) RETURN u`
//	fn CreateUser(name: string, age: int?) `CREATE (u:User {name: $name, age: $age}) RETURN u`
type Function struct {
	NodeMeta
	CommentMeta
	RecoveryMeta

	Name          string     `parser:"'fn' @Ident '('"`
	Params        []*FnParam `parser:"(@@ (Comma @@)*)?"`
	TrailingComma bool       `parser:"@Comma? ')'"`
	Body          string     `parser:"@RawString"`
}

// Query is an alias for Function for backward compatibility.
//
// Deprecated: Use Function instead.
type Query = Function

// FnParam represents a parameter in a function definition.
// Type annotations are optional - if omitted, the type is inferred as "any".
// Parameter names don't use $ prefix (the $ is used in the query body to reference them).
// Examples:
//
//	id                // untyped (inferred as any)
//	id: string        // simple type
//	name: string?     // nullable type
//	ids: [int]        // array type
//	data: {string: int}  // map type
type FnParam struct {
	NodeMeta
	RecoveryMeta

	Name string    `parser:"@Ident"`
	Type *TypeExpr `parser:"(Colon @@)?"`
}

// TypeExpr represents a type expression in the scaf DSL.
// Supports simple primitive types, arrays, maps, and nullable types.
type TypeExpr struct {
	NodeMeta
	RecoveryMeta

	// Simple is a simple type name like "string", "int", "bool", "float64", "any".
	Simple *string `parser:"( @Ident"`
	// Array is an array/slice type like "[string]" meaning []string.
	Array *TypeExpr `parser:"| '[' @@ ']'"`
	// Map is a map type like "{string: int}" meaning map[string]int.
	Map *MapTypeExpr `parser:"| @@ )"`
	// Nullable indicates the type is nullable (postfix ?).
	// This is set after parsing the base type.
	Nullable bool `parser:"@'?'?"`
}

// MapTypeExpr represents a map type expression like {string: int}.
type MapTypeExpr struct {
	NodeMeta
	RecoveryMeta
	Key   *TypeExpr `parser:"'{' @@"`
	Value *TypeExpr `parser:"Colon @@ '}'"`
}

// ToGoType converts a TypeExpr to a Go type string.
func (t *TypeExpr) ToGoType() string {
	if t == nil {
		return "any"
	}

	var base string
	switch {
	case t.Simple != nil:
		base = *t.Simple
	case t.Array != nil:
		base = "[]" + t.Array.ToGoType()
	case t.Map != nil:
		base = "map[" + t.Map.Key.ToGoType() + "]" + t.Map.Value.ToGoType()
	default:
		base = "any"
	}

	if t.Nullable {
		// For nullable types, we use pointers in Go
		return "*" + base
	}
	return base
}

// =============================================================================
// Setup-related nodes
// =============================================================================

// SetupClause represents a setup: inline query, module setup, query call, or block.
// Examples:
//
//	setup `CREATE (:User)`                              // inline query
//	setup fixtures                                      // module setup (runs module's setup clause)
//	setup fixtures.CreateUser($id: 1, $name: "Alice")   // query call with params
//	setup { fixtures; fixtures.CreateUser($id: 1) }     // block with multiple items
type SetupClause struct {
	NodeMeta
	RecoveryMeta
	Inline *string      `parser:"@RawString"`
	Call   *SetupCall   `parser:"| @@"`
	Module *string      `parser:"| @Ident"`
	Block  []*SetupItem `parser:"| '{' @@* '}'"`
}

// IsComplete returns true if the setup clause has content.
func (s *SetupClause) IsComplete() bool {
	return s.Inline != nil || s.Module != nil || s.Call != nil || len(s.Block) > 0
}

// SetupItem represents a single item in a setup block.
// Can be an inline query, module setup, or query call.
type SetupItem struct {
	NodeMeta
	RecoveryMeta
	Inline *string    `parser:"@RawString"`
	Call   *SetupCall `parser:"| @@"`
	Module *string    `parser:"| @Ident"`
}

// SetupCall invokes a query from a module with parameters.
// Examples:
//
//	fixtures.CreateUser($id: 1, $name: "Alice")
//	db.SeedData()
type SetupCall struct {
	NodeMeta
	RecoveryMeta
	Module        string        `parser:"@Ident Dot"`
	Query         string        `parser:"@Ident '('"`
	Params        []*SetupParam `parser:"(@@ (Comma @@)*)?"`
	TrailingComma bool          `parser:"@Comma? ')'"`
}

// IsComplete returns true if the setup call has all required parts.
func (c *SetupCall) IsComplete() bool {
	return c.Module != "" && c.Query != ""
}

// SetupParam is a parameter passed to a named setup.
type SetupParam struct {
	NodeMeta
	RecoveryMeta
	Name  string      `parser:"@Ident Colon"`
	Value *ParamValue `parser:"@@"`
}

// ParamValue represents a value in a parameter - either a literal or a field reference.
// Field references allow passing result values to assert queries.
// Examples:
//
//	$userId: 1           // literal
//	$authorId: u.id      // field reference from parent scope
//
// Note: Literal must come first to match keywords (true, false, null) before they're
// captured as identifiers by FieldRef.
type ParamValue struct {
	NodeMeta
	RecoveryMeta
	Literal  *Value       `parser:"@@"`
	FieldRef *DottedIdent `parser:"| @@"`
}

// ToGo converts a ParamValue to a native Go type.
// For field refs, returns nil - caller must resolve from scope.
func (p *ParamValue) ToGo() any {
	if p.Literal != nil {
		return p.Literal.ToGo()
	}

	return nil // Field ref - must be resolved by runner
}

// IsFieldRef returns true if this is a field reference.
func (p *ParamValue) IsFieldRef() bool {
	return p.FieldRef != nil && p.Literal == nil
}

// FieldRefString returns the field reference as a string, or empty if not a field ref.
func (p *ParamValue) FieldRefString() string {
	if p.FieldRef != nil {
		return p.FieldRef.String()
	}

	return ""
}

// String returns a string representation of the ParamValue.
func (p *ParamValue) String() string {
	if p.FieldRef != nil {
		return p.FieldRef.String()
	}

	if p.Literal != nil {
		return p.Literal.String()
	}

	return ""
}

// =============================================================================
// Scope and Test nodes
// =============================================================================

// FunctionScope groups tests that target a specific function.
type FunctionScope struct {
	NodeMeta
	CommentMeta
	RecoveryMeta
	FunctionName string         `parser:"@Ident '{'"`
	Setup        *SetupClause   `parser:"('setup' @@)?"`
	Teardown     *string        `parser:"('teardown' @RawString)?"`
	Items        []*TestOrGroup `parser:"@@*"`
	Close        string         `parser:"@'}'"`
}

// QueryScope is an alias for FunctionScope for backward compatibility.
//
// Deprecated: Use FunctionScope instead.
type QueryScope = FunctionScope

// IsComplete returns true if the function scope has a closing brace.
func (q *FunctionScope) IsComplete() bool {
	return q.Close != ""
}

// TestOrGroup is a union type - either a Test or a Group.
type TestOrGroup struct {
	NodeMeta
	RecoveryMeta
	Test  *Test  `parser:"@@"`
	Group *Group `parser:"| @@"`
}

// Group organizes related tests with optional shared setup and teardown.
type Group struct {
	NodeMeta
	CommentMeta
	RecoveryMeta
	Name     string         `parser:"'group' @String '{'"`
	Setup    *SetupClause   `parser:"('setup' @@)?"`
	Teardown *string        `parser:"('teardown' @RawString)?"`
	Items    []*TestOrGroup `parser:"@@*"`
	Close    string         `parser:"@'}'"`
}

// IsComplete returns true if the group has a closing brace.
func (g *Group) IsComplete() bool {
	return g.Close != ""
}

// Test defines a single test case with inputs, expected outputs, and optional assertions.
// Tests run in a transaction that rolls back after execution, so no teardown is needed.
type Test struct {
	NodeMeta
	CommentMeta
	RecoveryMeta
	Name       string       `parser:"'test' @String '{'"`
	Setup      *SetupClause `parser:"('setup' @@)?"`
	Statements []*Statement `parser:"@@*"`
	Asserts    []*Assert    `parser:"@@*"`
	Close      string       `parser:"@'}'"`
}

// IsComplete returns true if the test has a closing brace.
func (t *Test) IsComplete() bool {
	return t.Close != ""
}

// =============================================================================
// Assert nodes
// =============================================================================

// Assert represents an assertion block with optional query.
// Expressions are captured as tokens and reconstructed as strings for expr.Compile().
// Each condition is wrapped in parentheses - no semicolons needed.
// Supports shorthand form for single conditions without braces.
// Examples:
//
//	assert (u.age >= 18)                                     // shorthand single condition (no braces)
//	assert { (u.age > 18) }                                  // single condition
//	assert { (x > 0) (y < 10) (z == 5) }                     // multiple conditions
//	assert CreatePost($title: "x") { (p.title == "x") }      // named query with conditions
//	assert `MATCH (n) RETURN count(n) as cnt` { (cnt > 0) }  // inline query with conditions
type Assert struct {
	NodeMeta
	RecoveryMeta
	// Shorthand is a single parenthesized condition without braces: assert (expr)
	// When present, Query and Conditions should be nil/empty.
	Shorthand *ParenExpr `parser:"'assert' ( @@"`
	// Query is the optional query before the conditions block.
	Query *AssertQuery `parser:"| @@? '{'"`
	// Conditions are the parenthesized expressions inside the braces.
	Conditions []*ParenExpr `parser:"@@*"`
	// Close is the closing brace (empty for shorthand form).
	Close string `parser:"@'}' )"`
}

// IsShorthand returns true if this assertion uses the shorthand form (no braces).
func (a *Assert) IsShorthand() bool {
	return a.Shorthand != nil
}

// AllConditions returns all conditions, whether from shorthand or block form.
func (a *Assert) AllConditions() []*ParenExpr {
	if a.Shorthand != nil {
		return []*ParenExpr{a.Shorthand}
	}
	return a.Conditions
}

// ParenExpr is an expression wrapped in parentheses.
// Used for assert conditions and statement expressions where the parens delimit the expression.
// Uses BalancedExprToken to properly handle nested parentheses.
type ParenExpr struct {
	NodeMeta
	RecoveryMeta
	Tokens []*BalancedExprToken `parser:"'(' @@* ')'"`
}

// String returns the expression string without the surrounding parentheses.
func (p *ParenExpr) String() string {
	if p == nil || len(p.Tokens) == 0 {
		return ""
	}

	var b strings.Builder

	for i, tok := range p.Tokens {
		if i > 0 {
			prev := p.Tokens[i-1]
			// Add space between tokens except:
			// - around dots (u.name)
			// - after open brackets and nested parens in function call position
			// - before close brackets
			// - after commas (handled separately)
			// - between identifier and nested paren (function calls: len(x))
			needsSpace := !prev.IsDot() && !prev.IsOpenBracket() && !prev.Comma &&
				!tok.IsDot() && !tok.IsCloseBracket() &&
				(!prev.IsIdent() || (!tok.IsOpenBracket() && !tok.IsNestedParen()))
			if needsSpace {
				b.WriteByte(' ')
			}
		}
		b.WriteString(tok.String())
		if tok.Comma {
			b.WriteByte(' ')
		}
	}

	return b.String()
}

// BalancedExprToken captures individual tokens in a balanced parenthesis context.
// Unlike ExprToken, this does NOT capture top-level ) - only nested ones through NestedParen.
type BalancedExprToken struct {
	NodeMeta
	RecoveryMeta
	Str         *string            `parser:"@String"`
	Number      *string            `parser:"| @Number"`
	Ident       *string            `parser:"| @Ident"`
	Op          *string            `parser:"| @Op"`
	Dot         bool               `parser:"| @Dot"`
	Colon       bool               `parser:"| @Colon"`
	Comma       bool               `parser:"| @Comma"`
	NestedParen *BalancedParenExpr `parser:"| @@"`
	LBrack      bool               `parser:"| @'['"`
	RBrack      bool               `parser:"| @']'"`
}

// BalancedParenExpr is a nested parenthesized expression within a BalancedExprToken.
type BalancedParenExpr struct {
	NodeMeta
	RecoveryMeta
	Tokens []*BalancedExprToken `parser:"'(' @@* ')'"`
}

// String returns the string representation of a BalancedExprToken.
func (t *BalancedExprToken) String() string {
	switch {
	case t.Str != nil:
		return fmt.Sprintf("%q", *t.Str)
	case t.Number != nil:
		return *t.Number
	case t.Ident != nil:
		return *t.Ident
	case t.Op != nil:
		return *t.Op
	case t.Dot:
		return "."
	case t.Colon:
		return ":"
	case t.Comma:
		return ","
	case t.NestedParen != nil:
		return t.NestedParen.String()
	case t.LBrack:
		return "["
	case t.RBrack:
		return "]"
	default:
		return ""
	}
}

// String returns the string representation of a BalancedParenExpr.
func (p *BalancedParenExpr) String() string {
	var b strings.Builder
	b.WriteString("(")
	for i, tok := range p.Tokens {
		if i > 0 {
			prev := p.Tokens[i-1]
			needsSpace := !prev.IsDot() && !prev.IsOpenBracket() && !prev.Comma &&
				!tok.IsDot() && !tok.IsCloseBracket() &&
				(!prev.IsIdent() || (!tok.IsOpenBracket() && !tok.IsNestedParen()))
			if needsSpace {
				b.WriteByte(' ')
			}
		}
		b.WriteString(tok.String())
		if tok.Comma {
			b.WriteByte(' ')
		}
	}
	b.WriteString(")")
	return b.String()
}

// IsDot returns true if this token is a dot.
func (t *BalancedExprToken) IsDot() bool {
	return t.Dot
}

// IsOpenBracket returns true if this token is an opening bracket.
// Note: NestedParen is NOT considered an open bracket because it includes both ( and ).
func (t *BalancedExprToken) IsOpenBracket() bool {
	return t.LBrack
}

// IsNestedParen returns true if this token is a nested parenthesized expression.
func (t *BalancedExprToken) IsNestedParen() bool {
	return t.NestedParen != nil
}

// IsCloseBracket returns true if this token is a closing bracket.
func (t *BalancedExprToken) IsCloseBracket() bool {
	return t.RBrack // Note: nested parens don't need this - they include their own close
}

// IsIdent returns true if this token is an identifier.
func (t *BalancedExprToken) IsIdent() bool {
	return t.Ident != nil
}

// IsComplete returns true if the assert has a closing brace.
func (a *Assert) IsComplete() bool {
	return a.Close != ""
}

// AssertQuery specifies the query to run before evaluating conditions.
// Either an inline raw string query or a named query reference with params.
type AssertQuery struct {
	NodeMeta
	RecoveryMeta
	// Inline query (raw string)
	Inline *string `parser:"@RawString"`
	// Or named query reference with required parentheses
	QueryName     *string       `parser:"| @Ident '('"`
	Params        []*SetupParam `parser:"(@@ (Comma @@)*)?"`
	TrailingComma bool          `parser:"@Comma? ')'"`
}

// =============================================================================
// Expression nodes
// =============================================================================

// Expr captures tokens for expr-lang evaluation.
// Tokens are reconstructed into a string and parsed by expr.Compile() at runtime.
type Expr struct {
	NodeMeta
	RecoveryMeta
	ExprTokens []*ExprToken `parser:"@@+"`
}

// String reconstructs the expression as a string for expr-lang.
func (e *Expr) String() string {
	if e == nil || len(e.ExprTokens) == 0 {
		return ""
	}

	var b strings.Builder

	for i, tok := range e.ExprTokens {
		if i > 0 {
			prev := e.ExprTokens[i-1]
			// Add space between tokens except:
			// - around dots (u.name)
			// - after open brackets (foo(x), arr[0])
			// - before close brackets (foo(x), arr[0])
			// - between identifier and open bracket (function calls: len(x))
			// - after comma (we add space after comma below)
			needsSpace := !prev.IsDot() && !prev.IsOpenBracket() && !prev.Comma &&
				!tok.IsDot() && !tok.IsCloseBracket() &&
				(!prev.IsIdent() || !tok.IsOpenBracket())
			if needsSpace {
				b.WriteByte(' ')
			}
		}

		b.WriteString(tok.String())
		// Add space after comma
		if tok.Comma {
			b.WriteByte(' ')
		}
	}

	return b.String()
}

// ExprToken captures individual tokens that can appear in expressions.
// Matches expr-lang's token kinds: Identifier, Number, String, Operator, Bracket.
// Note: { } ; are NOT captured as they're expression delimiters.
type ExprToken struct {
	NodeMeta
	RecoveryMeta
	Str    *string `parser:"@String"`
	Number *string `parser:"| @Number"`
	Ident  *string `parser:"| @Ident"`
	Op     *string `parser:"| @Op"`
	Dot    bool    `parser:"| @Dot"`
	Colon  bool    `parser:"| @Colon"`
	Comma  bool    `parser:"| @Comma"`
	LParen bool    `parser:"| @'('"`
	RParen bool    `parser:"| @')'"`
	LBrack bool    `parser:"| @'['"`
	RBrack bool    `parser:"| @']'"`
}

// String returns the string representation of a token.
func (t *ExprToken) String() string {
	switch {
	case t.Str != nil:
		return fmt.Sprintf("%q", *t.Str)
	case t.Number != nil:
		return *t.Number
	case t.Ident != nil:
		return *t.Ident
	case t.Op != nil:
		return *t.Op
	case t.Dot:
		return "."
	case t.Colon:
		return ":"
	case t.Comma:
		return ","
	case t.LParen:
		return "("
	case t.RParen:
		return ")"
	case t.LBrack:
		return "["
	case t.RBrack:
		return "]"
	default:
		return ""
	}
}

// IsDot returns true if this token is a dot.
func (t *ExprToken) IsDot() bool {
	return t.Dot
}

// IsOpenBracket returns true if this token is an opening bracket.
func (t *ExprToken) IsOpenBracket() bool {
	return t.LParen || t.LBrack
}

// IsCloseBracket returns true if this token is a closing bracket.
func (t *ExprToken) IsCloseBracket() bool {
	return t.RParen || t.RBrack
}

// IsIdent returns true if this token is an identifier.
func (t *ExprToken) IsIdent() bool {
	return t.Ident != nil
}

// =============================================================================
// Statement and Value nodes
// =============================================================================

// DottedIdent represents a dot-separated identifier like "u.name" or "$userId".
type DottedIdent struct {
	NodeMeta
	RecoveryMeta
	Parts []string `parser:"@Ident (Dot @Ident)*"`
}

// String returns the dot-joined identifier.
func (d *DottedIdent) String() string {
	return strings.Join(d.Parts, ".")
}

// Statement represents a key-value pair for inputs ($var) or expected outputs.
// Examples:
//
//	$userId: 1                                    // input parameter (literal)
//	$userId: (2 * 4)                              // input parameter (expression)
//	$userId: (rand()) where (userId > 0)          // expression with constraint
//	$limit: 10 where (limit <= maxLimit)          // literal with constraint
//	u.name: "Alice"                               // expected output (equality)
type Statement struct {
	NodeMeta
	RecoveryMeta
	KeyParts *DottedIdent    `parser:"@@"`
	Value    *StatementValue `parser:"Colon @@"`
}

// StatementValue represents the value in a statement - literal or expression with optional constraint.
// All expressions are wrapped in parentheses for unambiguous parsing.
// Examples:
//
//	1                                 // literal value
//	(2 * 4)                           // parenthesized expression
//	(rand()) where (x > 0)            // expression with where constraint
//	"Alice" where (len(name) > 3)     // literal with where constraint
type StatementValue struct {
	NodeMeta
	RecoveryMeta
	// Literal is a simple literal value: 1, "hello", [1, 2, 3], {key: "value"}
	Literal *Value `parser:"( @@"`
	// Expr is a parenthesized expression: (2 * 4), (len(items))
	Expr *ParenExpr `parser:"| @@ )"`
	// Where is an optional constraint expression: where (x > 0)
	Where *ParenExpr `parser:"('where' @@)?"`
}

// IsExpr returns true if this statement value is an expression (not a literal).
func (sv *StatementValue) IsExpr() bool {
	return sv.Expr != nil
}

// HasWhere returns true if this statement value has a where constraint.
func (sv *StatementValue) HasWhere() bool {
	return sv.Where != nil
}

// ToValue returns the literal value, or nil if this is an expression.
// For backward compatibility with code that expects *Value.
func (sv *StatementValue) ToValue() *Value {
	return sv.Literal
}

// ToGo converts the literal value to a native Go type.
// Returns nil if this is an expression (expressions are evaluated at runtime).
func (sv *StatementValue) ToGo() any {
	if sv == nil || sv.Literal == nil {
		return nil
	}
	return sv.Literal.ToGo()
}

// ExprString returns the expression as a string for evaluation.
// Returns empty string if this is a literal value.
func (sv *StatementValue) ExprString() string {
	if sv == nil || sv.Expr == nil {
		return ""
	}
	return sv.Expr.String()
}

// WhereString returns the where constraint as a string for evaluation.
// Returns empty string if there is no where constraint.
func (sv *StatementValue) WhereString() string {
	if sv == nil || sv.Where == nil {
		return ""
	}
	return sv.Where.String()
}

// ToExpr converts the ParenExpr to an Expr for backward compatibility.
// Returns nil if this is not an expression.
func (sv *StatementValue) ToExpr() *Expr {
	if sv == nil || sv.Expr == nil {
		return nil
	}
	// Convert balanced tokens to regular expr tokens
	return parenExprToExpr(sv.Expr)
}

// parenExprToExpr converts a ParenExpr to a regular Expr.
func parenExprToExpr(p *ParenExpr) *Expr {
	if p == nil || len(p.Tokens) == 0 {
		return nil
	}
	tokens := make([]*ExprToken, 0, len(p.Tokens))
	for _, bt := range p.Tokens {
		tokens = append(tokens, balancedTokenToExprTokens(bt)...)
	}
	return &Expr{ExprTokens: tokens}
}

// balancedTokenToExprTokens converts a BalancedExprToken to ExprTokens.
func balancedTokenToExprTokens(bt *BalancedExprToken) []*ExprToken {
	switch {
	case bt.Str != nil:
		return []*ExprToken{{Str: bt.Str}}
	case bt.Number != nil:
		return []*ExprToken{{Number: bt.Number}}
	case bt.Ident != nil:
		return []*ExprToken{{Ident: bt.Ident}}
	case bt.Op != nil:
		return []*ExprToken{{Op: bt.Op}}
	case bt.Dot:
		return []*ExprToken{{Dot: true}}
	case bt.Colon:
		return []*ExprToken{{Colon: true}}
	case bt.Comma:
		return []*ExprToken{{Comma: true}}
	case bt.LBrack:
		return []*ExprToken{{LBrack: true}}
	case bt.RBrack:
		return []*ExprToken{{RBrack: true}}
	case bt.NestedParen != nil:
		// Recursively convert nested paren contents
		result := []*ExprToken{{LParen: true}}
		for _, nested := range bt.NestedParen.Tokens {
			result = append(result, balancedTokenToExprTokens(nested)...)
		}
		result = append(result, &ExprToken{RParen: true})
		return result
	default:
		return nil
	}
}

// String returns a string representation of the StatementValue.
func (sv *StatementValue) String() string {
	if sv == nil {
		return "null"
	}

	var b strings.Builder

	// Value part
	switch {
	case sv.IsExpr():
		b.WriteString("(")
		b.WriteString(sv.Expr.String())
		b.WriteString(")")
	case sv.Literal != nil:
		b.WriteString(sv.Literal.String())
	default:
		b.WriteString("null")
	}

	// Where clause
	if sv.HasWhere() {
		b.WriteString(" where (")
		b.WriteString(sv.Where.String())
		b.WriteString(")")
	}

	return b.String()
}

// Key returns the statement key as a dot-joined string.
func (s *Statement) Key() string {
	if s.KeyParts == nil {
		return ""
	}

	return s.KeyParts.String()
}

// NewStatement creates a Statement from a dot-separated key string and value.
// This is a convenience constructor for testing and programmatic AST construction.
//
//nolint:funcorder
func NewStatement(key string, value *Value) *Statement {
	parts := strings.Split(key, ".")

	return &Statement{
		KeyParts: &DottedIdent{Parts: parts},
		Value:    &StatementValue{Literal: value},
	}
}

// NewStatementExpr creates a Statement with an expression value.
// This is a convenience constructor for testing and programmatic AST construction.
//
//nolint:funcorder
func NewStatementExpr(key string, expr *ParenExpr) *Statement {
	parts := strings.Split(key, ".")

	return &Statement{
		KeyParts: &DottedIdent{Parts: parts},
		Value:    &StatementValue{Expr: expr},
	}
}

// NewStatementWithWhere creates a Statement with a where constraint.
// This is a convenience constructor for testing and programmatic AST construction.
//
//nolint:funcorder
func NewStatementWithWhere(key string, value *Value, where *ParenExpr) *Statement {
	parts := strings.Split(key, ".")

	return &Statement{
		KeyParts: &DottedIdent{Parts: parts},
		Value:    &StatementValue{Literal: value, Where: where},
	}
}

// Boolean is a bool type that implements participle's Capture interface.
type Boolean bool

// Capture implements participle's Capture interface for Boolean.
func (b *Boolean) Capture(values []string) error {
	*b = values[0] == "true"

	return nil
}

// Value represents a literal value (string, number, bool, null, map, or list).
type Value struct {
	NodeMeta
	RecoveryMeta
	Null    bool     `parser:"@'null'"`
	Str     *string  `parser:"| @String"`
	Number  *float64 `parser:"| @Number"`
	Boolean *Boolean `parser:"| @('true' | 'false')"`
	Map     *Map     `parser:"| @@"`
	List    *List    `parser:"| @@"`
}

// Map represents a key-value map literal.
type Map struct {
	NodeMeta
	RecoveryMeta
	Entries       []*MapEntry `parser:"'{' (@@ (Comma @@)*)?"`
	TrailingComma bool        `parser:"@Comma? '}'"`
}

// MapEntry represents a single entry in a map literal.
type MapEntry struct {
	NodeMeta
	RecoveryMeta
	Key   string `parser:"@Ident Colon"`
	Value *Value `parser:"@@"`
}

// List represents an array/list literal.
type List struct {
	NodeMeta
	RecoveryMeta
	Values        []*Value `parser:"'[' (@@ (Comma @@)*)?"`
	TrailingComma bool     `parser:"@Comma? ']'"`
}

// ToGo converts a Value to a native Go type.
func (v *Value) ToGo() any {
	switch {
	case v.Null:
		return nil
	case v.Str != nil:
		return *v.Str
	case v.Number != nil:
		return *v.Number
	case v.Boolean != nil:
		return bool(*v.Boolean)
	case v.Map != nil:
		m := make(map[string]any)
		for _, e := range v.Map.Entries {
			m[e.Key] = e.Value.ToGo()
		}

		return m
	case v.List != nil:
		l := make([]any, len(v.List.Values))
		for i, val := range v.List.Values {
			l[i] = val.ToGo()
		}

		return l
	default:
		return nil
	}
}

// String returns a string representation of the Value.
func (v *Value) String() string {
	switch {
	case v.Null:
		return "null"
	case v.Str != nil:
		return fmt.Sprintf("%q", *v.Str)
	case v.Number != nil:
		return fmt.Sprintf("%v", *v.Number)
	case v.Boolean != nil:
		return strconv.FormatBool(bool(*v.Boolean))
	case v.Map != nil:
		return v.mapString()
	case v.List != nil:
		return v.listString()
	default:
		return "nil"
	}
}

func (v *Value) mapString() string {
	parts := make([]string, len(v.Map.Entries))
	for i, e := range v.Map.Entries {
		parts[i] = fmt.Sprintf("%s: %s", e.Key, e.Value)
	}

	return "{" + strings.Join(parts, ", ") + "}"
}

func (v *Value) listString() string {
	parts := make([]string, len(v.List.Values))
	for i, val := range v.List.Values {
		parts[i] = val.String()
	}

	return "[" + strings.Join(parts, ", ") + "]"
}
