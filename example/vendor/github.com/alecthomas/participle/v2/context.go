package participle

import (
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
)

type contextFieldSet struct {
	tokens     []lexer.Token
	strct      reflect.Value
	field      structLexerField
	fieldValue []reflect.Value
}

// Context for a single parse.
type parseContext struct {
	lexer.PeekingLexer
	depth             int
	trace             io.Writer
	deepestError      error
	deepestErrorDepth int
	lookahead         int
	caseInsensitive   map[lexer.TokenType]bool
	apply             []*contextFieldSet
	allowTrailing     bool

	// Error recovery support
	recovery       *recoveryConfig
	recoveryErrors []error
}

func newParseContext(lex *lexer.PeekingLexer, lookahead int, caseInsensitive map[lexer.TokenType]bool) parseContext {
	return parseContext{
		PeekingLexer:    *lex,
		caseInsensitive: caseInsensitive,
		lookahead:       lookahead,
	}
}

func (p *parseContext) DeepestError(err error) error {
	if p.PeekingLexer.Cursor() >= p.deepestErrorDepth {
		return err
	}
	if p.deepestError != nil {
		return p.deepestError
	}
	return err
}

// Defer adds a function to be applied once a branch has been picked.
func (p *parseContext) Defer(tokens []lexer.Token, strct reflect.Value, field structLexerField, fieldValue []reflect.Value) {
	p.apply = append(p.apply, &contextFieldSet{tokens, strct, field, fieldValue})
}

// Apply deferred functions.
func (p *parseContext) Apply() error {
	for _, apply := range p.apply {
		if err := setField(apply.tokens, apply.strct, apply.field, apply.fieldValue); err != nil {
			return err
		}
	}
	p.apply = nil
	return nil
}

// Branch accepts the branch as the correct branch.
func (p *parseContext) Accept(branch *parseContext) {
	p.apply = append(p.apply, branch.apply...)
	p.PeekingLexer = branch.PeekingLexer
	if branch.deepestErrorDepth >= p.deepestErrorDepth {
		p.deepestErrorDepth = branch.deepestErrorDepth
		p.deepestError = branch.deepestError
	}
	// Merge recovery errors from the branch
	p.recoveryErrors = append(p.recoveryErrors, branch.recoveryErrors...)
}

// Branch starts a new lookahead branch.
func (p *parseContext) Branch() *parseContext {
	branch := &parseContext{}
	*branch = *p
	branch.apply = nil
	branch.recoveryErrors = nil // Don't share slice with parent
	return branch
}

func (p *parseContext) MaybeUpdateError(err error) {
	if p.PeekingLexer.Cursor() >= p.deepestErrorDepth {
		p.deepestError = err
		p.deepestErrorDepth = p.PeekingLexer.Cursor()
	}
}

// Stop returns true if parsing should terminate after the given "branch" failed to match.
//
// Additionally, track the deepest error in the branch - the deeper the error, the more useful it usually is.
// It could already be the deepest error in the branch (only if deeper than current parent context deepest),
// or it could be "err", the latest error on the branch (even if same depth; the lexer holds the position).
func (p *parseContext) Stop(err error, branch *parseContext) bool {
	if branch.deepestErrorDepth > p.deepestErrorDepth {
		p.deepestError = branch.deepestError
		p.deepestErrorDepth = branch.deepestErrorDepth
	} else if branch.PeekingLexer.Cursor() >= p.deepestErrorDepth {
		p.deepestError = err
		p.deepestErrorDepth = maxInt(branch.PeekingLexer.Cursor(), branch.deepestErrorDepth)
	}
	if !p.hasInfiniteLookahead() && branch.PeekingLexer.Cursor() > p.PeekingLexer.Cursor()+p.lookahead {
		p.Accept(branch)
		return true
	}
	return false
}

func (p *parseContext) hasInfiniteLookahead() bool { return p.lookahead < 0 }

func (p *parseContext) printTrace(n node) func() {
	if p.trace != nil {
		tok := p.PeekingLexer.Peek()
		fmt.Fprintf(p.trace, "%s%q %s\n", strings.Repeat(" ", p.depth*2), tok, n.GoString())
		p.depth += 1
		return func() { p.depth -= 1 }
	}
	return func() {}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Recovery support methods

// recoveryEnabled returns true if error recovery is enabled.
func (p *parseContext) recoveryEnabled() bool {
	return p.recovery != nil && len(p.recovery.strategies) > 0
}

// addRecoveryError records an error that occurred during recovery.
func (p *parseContext) addRecoveryError(err error) {
	p.recoveryErrors = append(p.recoveryErrors, err)
}

// tryRecover attempts to recover from a parse error using the configured strategies.
func (p *parseContext) tryRecover(err error, parent reflect.Value) recoveryResult {
	if !p.recoveryEnabled() {
		return recoveryResult{}
	}

	// Check if we've exceeded max errors
	if p.recovery.maxErrors > 0 && len(p.recoveryErrors) >= p.recovery.maxErrors {
		p.traceRecovery("max errors (%d) reached, not attempting recovery", p.recovery.maxErrors)
		return recoveryResult{}
	}

	startPos := p.Peek().Pos
	startCursor := p.RawCursor()

	p.traceRecoveryAttempt(startPos, err, "recovery")

	// Try each strategy in order
	for _, strategy := range p.recovery.strategies {
		if result, ok := p.tryStrategy(strategy, err, parent, startCursor); ok {
			return result
		}
	}

	p.traceRecoveryAllFailed()
	return recoveryResult{}
}

// tryStrategy attempts a single recovery strategy, returning the result and whether it succeeded.
func (p *parseContext) tryStrategy(strategy RecoveryStrategy, err error, parent reflect.Value, startCursor lexer.RawCursor) (recoveryResult, bool) {
	checkpoint := p.PeekingLexer.MakeCheckpoint()
	currentPos := p.Peek().Pos

	strategyName := p.getStrategyName(strategy)
	p.traceRecoveryStrategy(strategyName, currentPos)

	var recovered bool
	var values []reflect.Value
	var newErr error
	var recoveredTokens []lexer.Token

	if enhanced, ok := strategy.(EnhancedRecoveryStrategy); ok {
		result := enhanced.RecoverWithContext(p, err, parent)
		recovered = result.recovered
		values = result.values
		newErr = result.err
		recoveredTokens = result.recoveredTokens
		strategyName = result.strategyName // Use actual name from result
	} else {
		recovered, values, newErr = strategy.Recover(p, err, parent)
	}

	if recovered {
		progressed := p.RawCursor() > startCursor
		p.traceRecoverySuccess(strategyName, len(recoveredTokens), p.Peek().Pos)
		p.addRecoveryError(newErr)
		return recoveryResult{
			recovered:       true,
			values:          values,
			progressed:      progressed,
			recoveredTokens: recoveredTokens,
			strategyName:    strategyName,
		}, true
	}

	p.traceRecoveryFailed(strategyName, "strategy returned false")
	p.PeekingLexer.LoadCheckpoint(checkpoint)
	return recoveryResult{}, false
}

// getStrategyName returns a human-readable name for tracing.
func (p *parseContext) getStrategyName(strategy RecoveryStrategy) string {
	if enhanced, ok := strategy.(EnhancedRecoveryStrategy); ok {
		return enhanced.Name()
	}
	return "unknown"
}

// Recovery tracing methods

// traceRecovery writes a trace message if recovery tracing is enabled.
func (p *parseContext) traceRecovery(format string, args ...interface{}) {
	if p.recovery != nil && p.recovery.traceWriter != nil {
		fmt.Fprintf(p.recovery.traceWriter, "[recovery] "+format+"\n", args...)
	}
}

// traceRecoveryAttempt logs when a recovery attempt begins.
func (p *parseContext) traceRecoveryAttempt(pos lexer.Position, err error, nodeName string) {
	if p.recovery != nil && p.recovery.traceWriter != nil {
		fmt.Fprintf(p.recovery.traceWriter, "[recovery] %s: attempting recovery for %q (error: %v)\n",
			pos, nodeName, err)
	}
}

// traceRecoveryStrategy logs when a specific strategy is being tried.
func (p *parseContext) traceRecoveryStrategy(strategyName string, pos lexer.Position) {
	if p.recovery != nil && p.recovery.traceWriter != nil {
		fmt.Fprintf(p.recovery.traceWriter, "[recovery]   trying strategy %q at %v\n", strategyName, pos)
	}
}

// traceRecoverySuccess logs when recovery succeeds.
func (p *parseContext) traceRecoverySuccess(strategyName string, skippedCount int, newPos lexer.Position) {
	if p.recovery != nil && p.recovery.traceWriter != nil {
		fmt.Fprintf(p.recovery.traceWriter, "[recovery]   SUCCESS: %q skipped %d token(s), now at %v\n",
			strategyName, skippedCount, newPos)
	}
}

// traceRecoveryFailed logs when a strategy fails.
func (p *parseContext) traceRecoveryFailed(strategyName string, reason string) {
	if p.recovery != nil && p.recovery.traceWriter != nil {
		fmt.Fprintf(p.recovery.traceWriter, "[recovery]   FAILED: %q - %s\n", strategyName, reason)
	}
}

// traceRecoveryAllFailed logs when all strategies fail.
func (p *parseContext) traceRecoveryAllFailed() {
	if p.recovery != nil && p.recovery.traceWriter != nil {
		fmt.Fprintf(p.recovery.traceWriter, "[recovery]   all strategies failed\n")
	}
}
