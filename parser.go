package scaf

import (
	"github.com/alecthomas/participle/v2"
)

// dslLexer is the custom lexer for the scaf DSL.
// Implements lexer.Definition interface for full control over tokenization.
var dslLexer = newDSLLexer()

var parser = participle.MustBuild[Suite](
	participle.Lexer(dslLexer),
	participle.Unquote("RawString", "String"),
	participle.Elide("Whitespace", "Comment"),
)

// Parse parses a scaf DSL file and returns the resulting Suite AST.
func Parse(data []byte) (*Suite, error) {
	return parser.ParseBytes("", data)
}

// ExportedLexer returns the lexer definition for testing purposes.
func ExportedLexer() *dslDefinition {
	return dslLexer
}
