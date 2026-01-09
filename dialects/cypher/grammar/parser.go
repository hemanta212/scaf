package cyphergrammar

import (
	"strings"

	"github.com/alecthomas/participle/v2"
)

// Parser is the Cypher parser instance.
var Parser = participle.MustBuild[Script](
	participle.Lexer(CypherLexer),
	participle.Elide("Whitespace", "BlockComment", "LineComment"),
	participle.UseLookahead(10),         // Higher lookahead for nested property access + function calls
	participle.CaseInsensitive("Ident"), // Cypher keywords are case-insensitive
)

// Parse parses a Cypher query string into an AST.
func Parse(query string) (*Script, error) {
	return Parser.ParseString("", query)
}

// ParseBytes parses a Cypher query from bytes into an AST.
func ParseBytes(query []byte) (*Script, error) {
	return Parser.ParseBytes("", query)
}

// String returns the full name of an InvocationName (e.g., "apoc.text.join").
func (n *InvocationName) String() string {
	if n == nil {
		return ""
	}
	return strings.Join(n.Parts, ".")
}

// GetText returns the text representation of a PropertyExpr.
func (p *PropertyExpr) GetText() string {
	if p == nil {
		return ""
	}
	parts := append([]string{p.Base}, p.Props...)
	return strings.Join(parts, ".")
}

// IsFloat returns true if this literal is a floating-point number.
func (l *Literal) IsFloat() bool {
	return l != nil && l.Float != nil
}

// IsInt returns true if this literal is an integer.
func (l *Literal) IsInt() bool {
	return l != nil && (l.Int != nil || l.HexInt != nil || l.OctInt != nil)
}

// IsString returns true if this literal is a string.
func (l *Literal) IsString() bool {
	return l != nil && l.String != nil
}

// IsBool returns true if this literal is a boolean.
func (l *Literal) IsBool() bool {
	return l != nil && (l.True || l.False)
}

// IsNull returns true if this literal is NULL.
func (l *Literal) IsNull() bool {
	return l != nil && l.Null
}

// HasOR returns true if this expression uses OR.
func (e *Expression) HasOR() bool {
	return e != nil && len(e.Right) > 0
}

// HasXOR returns true if the XorExpr uses XOR.
func (x *XorExpr) HasXOR() bool {
	return x != nil && len(x.Right) > 0
}

// HasAND returns true if the AndExpr uses AND.
func (a *AndExpr) HasAND() bool {
	return a != nil && len(a.Right) > 0
}

// HasComparison returns true if this is a comparison expression.
func (c *ComparisonExpr) HasComparison() bool {
	return c != nil && len(c.Right) > 0
}
