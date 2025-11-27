package scaf

import (
	"io"

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

// Parse parses a scaf DSL file and returns the AST with comments attached to nodes.
// This function is thread-safe.
//
// On parse errors, returns a partial AST containing everything successfully parsed
// up to the error location, along with the error. Callers should use the partial
// AST for features like completion and hover even when errors are present.
func Parse(data []byte) (*Suite, error) {
	return ParseWithRecovery(data, false)
}

// ParseWithRecovery parses a scaf DSL file with optional error recovery.
// When withRecovery is true, the parser will attempt to continue parsing after
// encountering errors, collecting multiple errors and producing a more complete
// partial AST. This is useful for LSP features where you want maximum information
// even from invalid files.
//
// Recovery uses statement-boundary synchronization:
//   - Skips to closing braces `}` (block terminators)
//   - Skips to keywords that start new constructs: test, group, query, import, setup, teardown, assert
//   - Handles nested braces and parentheses correctly
func ParseWithRecovery(data []byte, withRecovery bool) (*Suite, error) {
	return parseWithOptions(data, withRecovery, nil)
}

// ParseWithRecoveryTrace is like ParseWithRecovery but writes recovery trace to w.
// Useful for debugging recovery behavior.
func ParseWithRecoveryTrace(data []byte, withRecovery bool, w io.Writer) (*Suite, error) {
	return parseWithOptions(data, withRecovery, w)
}

func parseWithOptions(data []byte, withRecovery bool, traceWriter io.Writer) (*Suite, error) {
	// Lock to ensure trivia isn't overwritten by concurrent parses
	dslLexer.Lock()
	defer dslLexer.Unlock()

	var suite *Suite
	var err error

	if withRecovery {
		opts := []participle.ParseOption{
			participle.Recover(
				// Skip to statement boundaries - keywords and block terminators.
				// This is the idiomatic participle approach: when an error occurs,
				// skip tokens until we find a synchronization point where parsing
				// can resume.
				participle.SkipUntil(
					"}",        // Block closer - ends test, group, assert, scope, setup block
					"test",     // Test definition
					"group",    // Group definition
					"query",    // Query definition
					"import",   // Import statement
					"setup",    // Setup clause
					"teardown", // Teardown clause
					"assert",   // Assert block
				),
				// Handle nested braces correctly so we don't sync to a } inside a nested block
				participle.NestedDelimiters("{", "}"),
				// Handle parentheses in function calls like fixtures.CreateUser()
				participle.NestedDelimiters("(", ")"),
				// Handle brackets in list literals like [1, 2, 3]
				participle.NestedDelimiters("[", "]"),
			),
			participle.MaxRecoveryErrors(50),
		}
		if traceWriter != nil {
			opts = append(opts, participle.TraceRecovery(traceWriter))
		}
		suite, err = parser.ParseBytes("", data, opts...)
	} else {
		suite, err = parser.ParseBytes("", data)
	}

	// Attach comments even to partial ASTs - Participle populates as much
	// of the AST as possible before the error location
	if suite != nil {
		attachComments(suite, dslLexer.Trivia())
	}

	return suite, err
}

// ExportedLexer returns the lexer definition for testing purposes.
//
//nolint:revive // unexported-return: intentionally returns unexported type for internal test use
func ExportedLexer() *dslDefinition {
	return dslLexer
}
