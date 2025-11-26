// Package scaf provides a DSL parser for database test scaffolding.
package scaf

import (
	"fmt"
	"strconv"
	"strings"
)

// Suite represents a complete test file with queries, setup, teardown, and test scopes.
type Suite struct {
	Imports  []*Import     `parser:"@@*"`
	Queries  []*Query      `parser:"@@*"`
	Setup    *SetupClause  `parser:"('setup' @@)?"`
	Teardown *string       `parser:"('teardown' @RawString)?"`
	Scopes   []*QueryScope `parser:"@@*"`
}

// Import represents a module import statement.
// Examples:
//
//	import "../../setup/lesson_plan_db"
//	import fixtures "../shared/fixtures"
type Import struct {
	Alias *string `parser:"'import' @Ident?"`
	Path  string  `parser:"@String"`
}

// Query defines a named database query.
type Query struct {
	Name string `parser:"'query' @Ident"`
	Body string `parser:"@RawString"`
}

// SetupClause represents either an inline setup query or a named setup reference.
// Examples:
//
//	setup `CREATE (:User)`                              // inline
//	setup lesson_plan_db.SetupLessonPlanDB()            // named, no params
//	setup fixtures.CreatePosts($n: 10, $authorId: 1)    // named with params
type SetupClause struct {
	Inline *string     `parser:"@RawString"`
	Named  *NamedSetup `parser:"| @@"`
}

// NamedSetup references a setup defined elsewhere (local or imported).
// Examples:
//
//	SetupLessonPlanDB()
//	module.SetupName($param: value)
type NamedSetup struct {
	Module *string       `parser:"(@Ident Dot)?"`
	Name   string        `parser:"@Ident '('"`
	Params []*SetupParam `parser:"(@@ (Comma @@)*)? ')'"`
}

// SetupParam is a parameter passed to a named setup.
type SetupParam struct {
	Name  string `parser:"@Ident Colon"`
	Value *Value `parser:"@@"`
}

// QueryScope groups tests that target a specific query.
type QueryScope struct {
	QueryName string         `parser:"@Ident '{'"`
	Setup     *SetupClause   `parser:"('setup' @@)?"`
	Teardown  *string        `parser:"('teardown' @RawString)?"`
	Items     []*TestOrGroup `parser:"@@* '}'"`
}

// TestOrGroup is a union type - either a Test or a Group.
type TestOrGroup struct {
	Test  *Test  `parser:"@@"`
	Group *Group `parser:"| @@"`
}

// Group organizes related tests with optional shared setup and teardown.
type Group struct {
	Name     string         `parser:"'group' @String '{'"`
	Setup    *SetupClause   `parser:"('setup' @@)?"`
	Teardown *string        `parser:"('teardown' @RawString)?"`
	Items    []*TestOrGroup `parser:"@@* '}'"`
}

// Test defines a single test case with inputs, expected outputs, and optional assertions.
// Tests run in a transaction that rolls back after execution, so no teardown is needed.
type Test struct {
	Name       string       `parser:"'test' @String '{'"`
	Setup      *SetupClause `parser:"('setup' @@)?"`
	Statements []*Statement `parser:"@@*"`
	Asserts    []*Assert    `parser:"@@*"`
	Close      string       `parser:"'}'"`
}

// Assert represents an assertion block with optional query.
// Expressions are captured as tokens and reconstructed as strings for expr.Compile().
// Examples:
//
//	assert { u.age > 18 }                                    // standalone expr
//	assert { x > 0; y < 10; z == 5 }                         // multiple exprs
//	assert CreatePost($title: "x") { p.title == "x" }        // named query with conditions
//	assert `MATCH (n) RETURN count(n) as cnt` { cnt > 0 }    // inline query with conditions
type Assert struct {
	Query      *AssertQuery `parser:"'assert' @@? '{'"`
	Conditions []*Expr      `parser:"(@@ Semi?)* '}'"`
}

// AssertQuery specifies the query to run before evaluating conditions.
// Either an inline raw string query or a named query reference with params.
type AssertQuery struct {
	// Inline query (raw string)
	Inline *string `parser:"@RawString"`
	// Or named query reference with required parentheses
	QueryName *string       `parser:"| @Ident '('"`
	Params    []*SetupParam `parser:"(@@ (Comma @@)*)? ')'"`
}

// Expr captures tokens for expr-lang evaluation.
// Tokens are reconstructed into a string and parsed by expr.Compile() at runtime.
type Expr struct {
	Tokens []*ExprToken `parser:"@@+"`
}

// String reconstructs the expression as a string for expr-lang.
func (e *Expr) String() string {
	if e == nil || len(e.Tokens) == 0 {
		return ""
	}
	var b strings.Builder
	for i, tok := range e.Tokens {
		if i > 0 {
			prev := e.Tokens[i-1]
			// Add space between tokens except:
			// - around dots (u.name)
			// - after open brackets (foo(x), arr[0])
			// - before close brackets (foo(x), arr[0])
			// - between identifier and open bracket (function calls: len(x))
			// - after comma (we add space after comma below)
			needsSpace := !prev.IsDot() && !prev.IsOpenBracket() && !prev.Comma &&
				!tok.IsDot() && !tok.IsCloseBracket() &&
				!(prev.IsIdent() && tok.IsOpenBracket())
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
	Str     *string `parser:"@String"`
	Number  *string `parser:"| @Number"`
	Ident   *string `parser:"| @Ident"`
	Op      *string `parser:"| @Op"`
	Dot     bool    `parser:"| @Dot"`
	Colon   bool    `parser:"| @Colon"`
	Comma   bool    `parser:"| @Comma"`
	LParen  bool    `parser:"| @'('"`
	RParen  bool    `parser:"| @')'"`
	LBrack  bool    `parser:"| @'['"`
	RBrack  bool    `parser:"| @']'"`
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

// DottedIdent represents a dot-separated identifier like "u.name" or "$userId".
type DottedIdent struct {
	Parts []string `parser:"@Ident (Dot @Ident)*"`
}

// String returns the dot-joined identifier.
func (d *DottedIdent) String() string {
	return strings.Join(d.Parts, ".")
}

// Statement represents a key-value pair for inputs ($var) or expected outputs.
// Examples:
//
//	$userId: 1                                    // input parameter
//	u.name: "Alice"                               // expected output (equality)
type Statement struct {
	KeyParts *DottedIdent `parser:"@@"`
	Value    *Value       `parser:"Colon @@"`
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
func NewStatement(key string, value *Value) *Statement {
	parts := strings.Split(key, ".")
	return &Statement{
		KeyParts: &DottedIdent{Parts: parts},
		Value:    value,
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
	Null    bool     `parser:"@'null'"`
	Str     *string  `parser:"| @String"`
	Number  *float64 `parser:"| @Number"`
	Boolean *Boolean `parser:"| @('true' | 'false')"`
	Map     *Map     `parser:"| @@"`
	List    *List    `parser:"| @@"`
}

// Map represents a key-value map literal.
type Map struct {
	Entries []*MapEntry `parser:"'{' (@@ (Comma @@)*)? '}'"`
}

// MapEntry represents a single entry in a map literal.
type MapEntry struct {
	Key   string `parser:"@Ident Colon"`
	Value *Value `parser:"@@"`
}

// List represents an array/list literal.
type List struct {
	Values []*Value `parser:"'[' (@@ (Comma @@)*)? ']'"`
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