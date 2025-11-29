package cyphergrammar

import (
	"github.com/alecthomas/participle/v2/lexer"
)

// CypherLexer defines the lexer for Cypher queries.
// Note: Cypher is case-insensitive for keywords, but identifiers preserve case.
var CypherLexer = lexer.MustStateful(lexer.Rules{
	"Root": {
		// Whitespace and comments (elided from output)
		{Name: "Whitespace", Pattern: `[ \t\r\n]+`, Action: nil},
		{Name: "BlockComment", Pattern: `/\*[^*]*\*+(?:[^/*][^*]*\*+)*/`, Action: nil},
		{Name: "LineComment", Pattern: `//[^\r\n]*`, Action: nil},

		// Multi-character operators (must come before single-char)
		{Name: "NotEqual", Pattern: `<>`},
		{Name: "LessEqual", Pattern: `<=`},
		{Name: "GreaterEqual", Pattern: `>=`},
		{Name: "AddAssign", Pattern: `\+=`},
		{Name: "Range", Pattern: `\.\.`},

		// Single-character operators
		{Name: "Eq", Pattern: `=`},
		{Name: "Less", Pattern: `<`},
		{Name: "Greater", Pattern: `>`},
		{Name: "Plus", Pattern: `\+`},
		{Name: "Minus", Pattern: `-`},
		{Name: "Star", Pattern: `\*`},
		{Name: "Slash", Pattern: `/`},
		{Name: "Percent", Pattern: `%`},
		{Name: "Caret", Pattern: `\^`},
		{Name: "Dot", Pattern: `\.`},
		{Name: "Comma", Pattern: `,`},
		{Name: "Semicolon", Pattern: `;`},
		{Name: "Colon", Pattern: `:`},
		{Name: "Pipe", Pattern: `\|`},
		{Name: "Dollar", Pattern: `\$`},
		{Name: "LParen", Pattern: `\(`},
		{Name: "RParen", Pattern: `\)`},
		{Name: "LBrace", Pattern: `\{`},
		{Name: "RBrace", Pattern: `\}`},
		{Name: "LBracket", Pattern: `\[`},
		{Name: "RBracket", Pattern: `\]`},

		// String literals (both single and double quotes)
		{Name: "String", Pattern: `"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'`},

		// Escaped identifier (backtick-quoted)
		{Name: "EscapedIdent", Pattern: "`[^`]+`"},

		// Numbers - float must come before int to match longest
		{Name: "Float", Pattern: `-?(?:\d+\.\d*|\.\d+)(?:[eE][+-]?\d+)?`},
		{Name: "HexInt", Pattern: `-?0[xX][0-9a-fA-F]+`},
		{Name: "OctalInt", Pattern: `-?0[0-7]+`},
		{Name: "Int", Pattern: `-?\d+`},

		// Identifiers (including keywords - we'll match keywords by literal)
		// Must come after numbers to avoid matching leading digits
		{Name: "Ident", Pattern: `[a-zA-Z_][a-zA-Z0-9_]*`},
	},
})
