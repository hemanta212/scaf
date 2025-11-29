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
	cmpopts.IgnoreFields(scaf.StatementValue{}, "RecoveredSpan"),
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

// makeParenExpr creates a ParenExpr from ExprTokens.
// This is a test helper to convert the old Expr-based syntax to the new ParenExpr syntax.
func makeParenExpr(tokens []*scaf.ExprToken) *scaf.ParenExpr {
	balancedTokens := make([]*scaf.BalancedExprToken, 0, len(tokens))
	for _, tok := range tokens {
		balancedTokens = append(balancedTokens, exprTokenToBalanced(tok))
	}
	return &scaf.ParenExpr{Tokens: balancedTokens}
}

// exprTokenToBalanced converts an ExprToken to a BalancedExprToken.
func exprTokenToBalanced(tok *scaf.ExprToken) *scaf.BalancedExprToken {
	return &scaf.BalancedExprToken{
		Str:    tok.Str,
		Number: tok.Number,
		Ident:  tok.Ident,
		Op:     tok.Op,
		Dot:    tok.Dot,
		Colon:  tok.Colon,
		Comma:  tok.Comma,
		LBrack: tok.LBrack,
		RBrack: tok.RBrack,
		// Note: LParen/RParen from ExprToken become NestedParen in BalancedExprToken
		// For simplicity in tests, we skip nested paren handling here
	}
}

// makeConditions creates ParenExpr conditions from Expr slices for test compatibility.
func makeConditions(exprs ...*scaf.Expr) []*scaf.ParenExpr {
	result := make([]*scaf.ParenExpr, len(exprs))
	for i, expr := range exprs {
		result[i] = makeParenExpr(expr.ExprTokens)
	}
	return result
}
