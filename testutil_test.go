package scaf_test

import (
	"github.com/alecthomas/participle/v2/lexer"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/rlch/scaf"
)

// cmpIgnoreAST is a cmp option that ignores AST metadata fields in comparisons.
// This allows tests to compare AST structure without specifying exact source positions,
// tokens, comments, close fields, or recovery metadata.
//
// Fields ignored:
//   - lexer.Position, lexer.Token, []lexer.Token (via embedded NodeMeta)
//   - LeadingComments, TrailingComment (via embedded CommentMeta)
//   - RecoveredSpan (via embedded RecoveryMeta)
//   - Close (captured closing braces on completable nodes)
var cmpIgnoreAST = cmp.Options{
	// Ignore position and token types completely
	cmpopts.IgnoreTypes(lexer.Position{}, lexer.Token{}, []lexer.Token{}),
	// Ignore comment fields (from embedded CommentMeta)
	cmpopts.IgnoreFields(scaf.Suite{}, "LeadingComments", "TrailingComment"),
	cmpopts.IgnoreFields(scaf.Import{}, "LeadingComments", "TrailingComment"),
	cmpopts.IgnoreFields(scaf.Query{}, "LeadingComments", "TrailingComment"),
	cmpopts.IgnoreFields(scaf.QueryScope{}, "LeadingComments", "TrailingComment", "Close"),
	cmpopts.IgnoreFields(scaf.Group{}, "LeadingComments", "TrailingComment", "Close"),
	cmpopts.IgnoreFields(scaf.Test{}, "LeadingComments", "TrailingComment", "Close"),
	cmpopts.IgnoreFields(scaf.Assert{}, "Close"),
	// Ignore recovery metadata (from embedded RecoveryMeta)
	cmpopts.IgnoreFields(scaf.Statement{}, "RecoveredSpan"),
}

// ptr returns a pointer to the given value.
func ptr[T any](v T) *T {
	return &v
}

// boolPtr returns a pointer to a Boolean value.
func boolPtr(v bool) *scaf.Boolean {
	b := scaf.Boolean(v)
	return &b
}
