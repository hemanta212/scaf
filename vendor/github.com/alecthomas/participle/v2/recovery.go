package participle

import (
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
)

// RecoveryStrategy defines a strategy for recovering from parse errors.
//
// Error recovery allows the parser to continue parsing after encountering an error,
// collecting multiple errors and producing a partial AST. This is inspired by
// Chumsky's recovery system in Rust and classic compiler panic-mode recovery.
//
// There is no silver bullet strategy for error recovery. By definition, if the input
// to a parser is invalid then the parser can only make educated guesses as to the
// meaning of the input. Different recovery strategies will work better for different
// languages, and for different patterns within those languages.
type RecoveryStrategy interface {
	// Recover attempts to recover from a parse error.
	//
	// Parameters:
	//   - ctx: The parse context (positioned after the failed parse attempt)
	//   - err: The error that triggered recovery
	//   - parent: The parent value being parsed into
	//
	// Returns:
	//   - recovered: true if recovery was successful
	//   - values: any values recovered (may be nil/fallback for skip strategies)
	//   - newErr: the error to report (may be modified/wrapped)
	Recover(ctx *parseContext, err error, parent reflect.Value) (recovered bool, values []reflect.Value, newErr error)
}

// =============================================================================
// ViaParser Strategy - Chumsky's via_parser equivalent
// =============================================================================

// ViaParserStrategy recovers by running an alternative parser.
//
// This is the equivalent of Chumsky's via_parser strategy. When the main parser
// fails, this strategy runs a custom recovery parser to produce a fallback value.
//
// This is the most flexible recovery strategy - you have full control over what
// gets parsed and returned during recovery.
type ViaParserStrategy struct {
	// parseFn is the recovery parser function (stored as reflect.Value)
	parseFn reflect.Value
	// targetType is the expected return type of the recovery parser
	targetType reflect.Type
}

// viaParserStrategy creates just the strategy without registering it.
// Used internally and by RecoverTypeWith.
func viaParserStrategy[T any](recoveryParser func(*lexer.PeekingLexer) (T, error)) *ViaParserStrategy {
	parseFnVal := reflect.ValueOf(recoveryParser)
	parseFnType := parseFnVal.Type()
	return &ViaParserStrategy{
		parseFn:    parseFnVal,
		targetType: parseFnType.Out(0),
	}
}

func (v *ViaParserStrategy) Recover(ctx *parseContext, err error, parent reflect.Value) (bool, []reflect.Value, error) {
	// Call the recovery parser
	results := v.parseFn.Call([]reflect.Value{reflect.ValueOf(&ctx.PeekingLexer)})

	// Check for error from recovery parser
	if errVal := results[1].Interface(); errVal != nil {
		if _, ok := errVal.(error); ok {
			// Recovery parser failed - don't recover
			return false, nil, err
		}
	}

	// Return the recovered value
	recoveredValue := results[0]
	return true, []reflect.Value{recoveredValue}, err
}

// TargetType returns the type this strategy applies to.
// This implements TypeAwareStrategy, allowing ViaParserStrategy to be used with Recover().
func (v *ViaParserStrategy) TargetType() reflect.Type {
	return v.targetType
}

// ViaParser creates a type-specific recovery strategy using a custom parser function.
// This is the equivalent of Chumsky's via_parser strategy.
//
// When the main parser fails to parse type T, this strategy runs a custom recovery
// parser to produce a fallback value. This is the most flexible recovery strategy -
// you have full control over what gets parsed and returned during recovery.
//
// Use with Recover() at parse time:
//
//	ast, err := parser.ParseString("", input,
//	    participle.Recover(
//	        participle.ViaParser(func(lex *lexer.PeekingLexer) (*Expr, error) {
//	            // Skip tokens until semicolon
//	            for !lex.Peek().EOF() && lex.Peek().Value != ";" {
//	                lex.Next()
//	            }
//	            return &Expr{IsError: true}, nil
//	        }),
//	        participle.SkipUntil(";"), // Global fallback for other types
//	    ),
//	)
//
// The strategy will only be applied when parsing the target type (*Expr in this example).
// Other types will fall through to subsequent strategies (like SkipUntil).
func ViaParser[T any](recoveryParser func(*lexer.PeekingLexer) (T, error)) *ViaParserStrategy {
	parseFnVal := reflect.ValueOf(recoveryParser)
	parseFnType := parseFnVal.Type()
	return &ViaParserStrategy{
		parseFn:    parseFnVal,
		targetType: parseFnType.Out(0),
	}
}

// recoveryConfig holds recovery configuration for a parse context.
type recoveryConfig struct {
	strategies     []RecoveryStrategy                    // Global fallback strategies
	typeStrategies map[reflect.Type][]RecoveryStrategy   // Type-specific strategies (e.g., ViaParser)
	errors         []error
	maxErrors      int
	traceWriter    io.Writer                             // If non-nil, write recovery trace to this writer
}

// TypeAwareStrategy is an optional interface for strategies that are type-specific.
// When passed to Recover(), these strategies are only applied when parsing their target type.
type TypeAwareStrategy interface {
	RecoveryStrategy
	// TargetType returns the type this strategy applies to.
	TargetType() reflect.Type
}

// recoveryResult holds the result of a recovery attempt with additional context.
type recoveryResult struct {
	recovered     bool
	values        []reflect.Value
	err           error
	skippedTokens []lexer.Token
	strategyName  string
	progressed    bool // Whether the lexer position advanced during recovery
}

// EnhancedRecoveryStrategy is an optional interface for strategies that provide
// enhanced recovery information including skipped tokens.
type EnhancedRecoveryStrategy interface {
	RecoveryStrategy
	// RecoverWithContext performs recovery and returns detailed information about
	// what was skipped. This is used for generating better error messages.
	RecoverWithContext(ctx *parseContext, err error, parent reflect.Value) recoveryResult
	// Name returns a human-readable name for this strategy (used in error messages).
	Name() string
}

// RecoveryError wraps multiple errors that occurred during parsing with recovery.
type RecoveryError struct {
	Errors []error
}

func (r *RecoveryError) Error() string {
	if len(r.Errors) == 0 {
		return "no errors"
	}
	if len(r.Errors) == 1 {
		return r.Errors[0].Error()
	}
	msg := r.Errors[0].Error()
	for i := 1; i < len(r.Errors); i++ {
		msg += "\n" + r.Errors[i].Error()
	}
	return msg
}

// Unwrap returns the first error for compatibility with errors.Is/As.
func (r *RecoveryError) Unwrap() error {
	if len(r.Errors) == 0 {
		return nil
	}
	return r.Errors[0]
}

// RecoveredParseError represents a parse error that was recovered from,
// with additional context about what was skipped and where.
type RecoveredParseError struct {
	// Pos is the position where the error occurred.
	Pos lexer.Position
	// EndPos is the position where recovery ended (after skipped tokens).
	EndPos lexer.Position
	// Expected is the error message describing what was expected.
	Expected string
	// Label is a descriptive name for what was being parsed (e.g., "expression", "statement").
	Label string
	// SkippedTokens are the tokens that were skipped during recovery.
	SkippedTokens []lexer.Token
	// Strategy is the name of the recovery strategy that succeeded.
	Strategy string
	// Underlying is the original error that triggered recovery.
	Underlying error
}

func (e *RecoveredParseError) Error() string {
	return FormatError(e)
}

func (e *RecoveredParseError) Position() lexer.Position {
	return e.Pos
}

// Message implements the Error interface, providing a richly formatted error message.
func (e *RecoveredParseError) Message() string {
	return e.buildMessage()
}

// formatSkippedTokens returns a human-readable representation of skipped tokens.
func (e *RecoveredParseError) formatSkippedTokens() string {
	if len(e.SkippedTokens) == 0 {
		return ""
	}
	if len(e.SkippedTokens) == 1 {
		return fmt.Sprintf("%q", e.SkippedTokens[0].Value)
	}
	if len(e.SkippedTokens) <= 3 {
		var parts []string
		for _, t := range e.SkippedTokens {
			parts = append(parts, fmt.Sprintf("%q", t.Value))
		}
		return strings.Join(parts, ", ")
	}
	// For longer sequences, show first 2 and count
	return fmt.Sprintf("%q, %q, ... (%d tokens total)",
		e.SkippedTokens[0].Value,
		e.SkippedTokens[1].Value,
		len(e.SkippedTokens))
}

func (e *RecoveredParseError) buildMessage() string {
	var parts []string

	// Start with label context if available
	if e.Label != "" {
		parts = append(parts, fmt.Sprintf("in %s", e.Label))
	}

	// Add the core error message
	parts = append(parts, e.Expected)

	// Add information about what was skipped
	if len(e.SkippedTokens) > 0 {
		parts = append(parts, fmt.Sprintf("(skipped %s)", e.formatSkippedTokens()))
	}

	return strings.Join(parts, ": ")
}

// Unwrap returns the underlying error.
func (e *RecoveredParseError) Unwrap() error {
	return e.Underlying
}

// makeRecoveredError creates a RecoveredParseError with proper context.
func makeRecoveredError(
	pos lexer.Position,
	endPos lexer.Position,
	expected string,
	label string,
	skippedTokens []lexer.Token,
	strategy string,
	underlying error,
) *RecoveredParseError {
	return &RecoveredParseError{
		Pos:           pos,
		EndPos:        endPos,
		Expected:      expected,
		Label:         label,
		SkippedTokens: skippedTokens,
		Strategy:      strategy,
		Underlying:    underlying,
	}
}

// SkipUntilStrategy skips tokens until one of the synchronization tokens is found.
//
// This is the classic "panic mode" recovery strategy from compiler theory.
// It's simple but effective for languages with clear statement terminators
// (like semicolons) or block delimiters.
//
// Example usage:
//
//	parser.ParseString("", input, participle.Recover(SkipUntil(";", "}", ")")))
type SkipUntilStrategy struct {
	// Tokens to synchronize on (the parser will stop before these tokens)
	SyncTokens []string
	// If true, consume the sync token; if false, leave it for the next parse
	ConsumeSyncToken bool
	// Fallback returns a fallback value when recovery succeeds.
	// If nil, an empty/zero value is used.
	Fallback func() interface{}
}

// SkipUntil creates a recovery strategy that skips tokens until a sync token is found.
//
// The sync tokens are typically statement terminators (";"), block delimiters ("}", ")"),
// or keywords that start new constructs ("if", "while", "func", etc.).
func SkipUntil(tokens ...string) *SkipUntilStrategy {
	return &SkipUntilStrategy{
		SyncTokens:       tokens,
		ConsumeSyncToken: false,
	}
}

// SkipPast creates a recovery strategy that skips tokens until a sync token is found,
// then consumes the sync token.
func SkipPast(tokens ...string) *SkipUntilStrategy {
	return &SkipUntilStrategy{
		SyncTokens:       tokens,
		ConsumeSyncToken: true,
	}
}

// WithFallback sets a fallback value generator for the skip strategy.
func (s *SkipUntilStrategy) WithFallback(f func() interface{}) *SkipUntilStrategy {
	s.Fallback = f
	return s
}

func (s *SkipUntilStrategy) Recover(ctx *parseContext, err error, parent reflect.Value) (bool, []reflect.Value, error) {
	result := s.RecoverWithContext(ctx, err, parent)
	return result.recovered, result.values, result.err
}

// RecoverWithContext implements EnhancedRecoveryStrategy.
func (s *SkipUntilStrategy) RecoverWithContext(ctx *parseContext, err error, parent reflect.Value) recoveryResult {
	syncSet := make(map[string]bool)
	for _, t := range s.SyncTokens {
		syncSet[t] = true
	}

	var skippedTokens []lexer.Token

	// Skip tokens until we find a sync token or EOF
	for {
		token := ctx.Peek()
		if token.EOF() {
			return recoveryResult{recovered: false, err: err, skippedTokens: skippedTokens, strategyName: s.Name()}
		}
		if syncSet[token.Value] {
			if s.ConsumeSyncToken {
				skippedTokens = append(skippedTokens, *ctx.Next())
			}
			// Recovery successful
			var values []reflect.Value
			if s.Fallback != nil {
				values = []reflect.Value{reflect.ValueOf(s.Fallback())}
			}
			return recoveryResult{
				recovered:     true,
				values:        values,
				err:           err,
				skippedTokens: skippedTokens,
				strategyName:  s.Name(),
			}
		}
		skippedTokens = append(skippedTokens, *ctx.Next())
	}
}

// Name implements EnhancedRecoveryStrategy.
func (s *SkipUntilStrategy) Name() string {
	if s.ConsumeSyncToken {
		return "skip_past"
	}
	return "skip_until"
}

// SkipThenRetryUntilStrategy skips tokens and retries parsing until successful
// or a termination condition is met.
//
// This is more sophisticated than SkipUntil - it repeatedly:
// 1. Skips one token
// 2. Tries to parse again
// 3. If parsing succeeds without new errors, returns success
// 4. If parsing fails, repeats from step 1
//
// This continues until a termination token is found or EOF is reached.
//
// Note: This strategy requires special handling in recoveryNode to actually
// retry the inner parser. When used via Recover(), it signals retry mode.
type SkipThenRetryUntilStrategy struct {
	// Tokens that terminate the recovery attempt (stop trying)
	UntilTokens []string
	// Maximum tokens to skip before giving up (0 = unlimited)
	MaxSkip int
}

// SkipThenRetryUntil creates a strategy that skips tokens and retries parsing.
func SkipThenRetryUntil(untilTokens ...string) *SkipThenRetryUntilStrategy {
	return &SkipThenRetryUntilStrategy{
		UntilTokens: untilTokens,
		MaxSkip:     100, // Reasonable default to prevent infinite loops
	}
}

// WithMaxSkip sets the maximum number of tokens to skip.
func (s *SkipThenRetryUntilStrategy) WithMaxSkip(max int) *SkipThenRetryUntilStrategy {
	s.MaxSkip = max
	return s
}

// Recover implements simple skip behavior.
// The actual retry logic is in recoveryNode.Parse for retry-capable strategies.
func (s *SkipThenRetryUntilStrategy) Recover(ctx *parseContext, err error, parent reflect.Value) (bool, []reflect.Value, error) {
	untilSet := make(map[string]bool)
	for _, t := range s.UntilTokens {
		untilSet[t] = true
	}

	// Check if we're at a terminating token or EOF
	token := ctx.Peek()
	if token.EOF() || untilSet[token.Value] {
		return false, nil, err
	}

	// Skip one token - caller may retry
	ctx.Next()
	return true, nil, err
}

// IsRetryStrategy returns true, indicating this strategy supports retry semantics.
func (s *SkipThenRetryUntilStrategy) IsRetryStrategy() bool {
	return true
}

// ShouldStop returns true if we've hit a termination condition.
func (s *SkipThenRetryUntilStrategy) ShouldStop(ctx *parseContext) bool {
	token := ctx.Peek()
	if token.EOF() {
		return true
	}
	for _, t := range s.UntilTokens {
		if token.Value == t {
			return true
		}
	}
	return false
}

// RetryStrategy is an optional interface for strategies that support
// retry-at-each-position semantics (like Chumsky's skip_then_retry_until).
type RetryStrategy interface {
	RecoveryStrategy
	// IsRetryStrategy returns true for strategies that want retry behavior.
	IsRetryStrategy() bool
	// ShouldStop returns true when the strategy should give up.
	ShouldStop(ctx *parseContext) bool
}

// NestedDelimitersStrategy recovers by finding balanced delimiters.
//
// This is particularly useful for recovering from errors inside parenthesized
// expressions, function arguments, array indices, etc. It respects nesting,
// so it will correctly handle nested brackets.
//
// Example: If parsing `foo(bar(1, 2, err!@#), baz)` fails on `err!@#`,
// this strategy can skip to the closing `)` of `bar(...)` while respecting
// the nested parentheses.
type NestedDelimitersStrategy struct {
	// Start delimiter (e.g., "(", "[", "{")
	Start string
	// End delimiter (e.g., ")", "]", "}")
	End string
	// Additional delimiter pairs to respect for nesting
	Others [][2]string
	// Fallback returns a fallback value when recovery succeeds.
	Fallback func() interface{}
}

// NestedDelimiters creates a strategy that skips to balanced delimiters.
func NestedDelimiters(start, end string, others ...[2]string) *NestedDelimitersStrategy {
	return &NestedDelimitersStrategy{
		Start:  start,
		End:    end,
		Others: others,
	}
}

// WithFallback sets a fallback value generator for the nested delimiters strategy.
func (n *NestedDelimitersStrategy) WithFallback(f func() interface{}) *NestedDelimitersStrategy {
	n.Fallback = f
	return n
}

func (n *NestedDelimitersStrategy) Recover(ctx *parseContext, err error, parent reflect.Value) (bool, []reflect.Value, error) {
	result := n.RecoverWithContext(ctx, err, parent)
	return result.recovered, result.values, result.err
}

// RecoverWithContext implements EnhancedRecoveryStrategy.
func (n *NestedDelimitersStrategy) RecoverWithContext(ctx *parseContext, err error, parent reflect.Value) recoveryResult {
	// Build delimiter maps
	openers := map[string]string{n.Start: n.End}
	closers := map[string]bool{n.End: true}
	for _, pair := range n.Others {
		openers[pair[0]] = pair[1]
		closers[pair[1]] = true
	}

	// Track nesting depth for each delimiter type
	depths := make(map[string]int)
	var skippedTokens []lexer.Token

	// We start inside the delimited region, so we're looking for the closing delimiter
	// at depth 0 (or the matching closer for our opener)
	targetClose := n.End
	depth := 1 // We're inside one level of our target delimiters

	for {
		token := ctx.Peek()
		if token.EOF() {
			return recoveryResult{recovered: false, err: err, skippedTokens: skippedTokens, strategyName: n.Name()}
		}

		// Check if this opens a nested delimiter
		if closer, isOpener := openers[token.Value]; isOpener {
			if token.Value == n.Start {
				depth++
			} else {
				depths[closer]++
			}
		}

		// Check if this closes a delimiter
		if closers[token.Value] {
			if token.Value == targetClose {
				depth--
				if depth == 0 {
					// Found our balanced closer - don't consume it
					var values []reflect.Value
					if n.Fallback != nil {
						values = []reflect.Value{reflect.ValueOf(n.Fallback())}
					}
					return recoveryResult{
						recovered:     true,
						values:        values,
						err:           err,
						skippedTokens: skippedTokens,
						strategyName:  n.Name(),
					}
				}
			} else if depths[token.Value] > 0 {
				depths[token.Value]--
			} else {
				// Mismatched closer - this is an error, but we can try to continue
				// by treating it as the end of our recovery region
				return recoveryResult{recovered: false, err: err, skippedTokens: skippedTokens, strategyName: n.Name()}
			}
		}

		skippedTokens = append(skippedTokens, *ctx.Next())
	}
}

// Name implements EnhancedRecoveryStrategy.
func (n *NestedDelimitersStrategy) Name() string {
	return "nested_delimiters"
}

// TokenSyncStrategy synchronizes on specific token types rather than values.
//
// This is useful when you want to recover to any identifier, any string literal,
// or other token categories defined by your lexer.
type TokenSyncStrategy struct {
	// Token types to synchronize on (use lexer symbol names)
	SyncTypes []lexer.TokenType
	// If true, consume the sync token
	ConsumeSyncToken bool
	// Fallback value generator
	Fallback func() interface{}
}

// SyncToTokenType creates a strategy that syncs on token types.
func SyncToTokenType(types ...lexer.TokenType) *TokenSyncStrategy {
	return &TokenSyncStrategy{
		SyncTypes:        types,
		ConsumeSyncToken: false,
	}
}

func (t *TokenSyncStrategy) Recover(ctx *parseContext, err error, parent reflect.Value) (bool, []reflect.Value, error) {
	result := t.RecoverWithContext(ctx, err, parent)
	return result.recovered, result.values, result.err
}

// RecoverWithContext implements EnhancedRecoveryStrategy.
func (t *TokenSyncStrategy) RecoverWithContext(ctx *parseContext, err error, parent reflect.Value) recoveryResult {
	syncSet := make(map[lexer.TokenType]bool)
	for _, tt := range t.SyncTypes {
		syncSet[tt] = true
	}

	var skippedTokens []lexer.Token

	for {
		token := ctx.Peek()
		if token.EOF() {
			return recoveryResult{recovered: false, err: err, skippedTokens: skippedTokens, strategyName: t.Name()}
		}
		if syncSet[token.Type] {
			if t.ConsumeSyncToken {
				skippedTokens = append(skippedTokens, *ctx.Next())
			}
			var values []reflect.Value
			if t.Fallback != nil {
				values = []reflect.Value{reflect.ValueOf(t.Fallback())}
			}
			return recoveryResult{
				recovered:     true,
				values:        values,
				err:           err,
				skippedTokens: skippedTokens,
				strategyName:  t.Name(),
			}
		}
		skippedTokens = append(skippedTokens, *ctx.Next())
	}
}

// Name implements EnhancedRecoveryStrategy.
func (t *TokenSyncStrategy) Name() string {
	return "sync_to_token_type"
}

// CompositeStrategy tries multiple strategies in order until one succeeds.
type CompositeStrategy struct {
	Strategies []RecoveryStrategy
}

// TryStrategies creates a composite strategy that tries each strategy in order.
func TryStrategies(strategies ...RecoveryStrategy) *CompositeStrategy {
	return &CompositeStrategy{Strategies: strategies}
}

func (c *CompositeStrategy) Recover(ctx *parseContext, err error, parent reflect.Value) (bool, []reflect.Value, error) {
	result := c.RecoverWithContext(ctx, err, parent)
	return result.recovered, result.values, result.err
}

// RecoverWithContext implements EnhancedRecoveryStrategy.
func (c *CompositeStrategy) RecoverWithContext(ctx *parseContext, err error, parent reflect.Value) recoveryResult {
	checkpoint := ctx.saveCheckpoint()

	for _, strategy := range c.Strategies {
		// Try enhanced recovery first if available
		if enhanced, ok := strategy.(EnhancedRecoveryStrategy); ok {
			result := enhanced.RecoverWithContext(ctx, err, parent)
			if result.recovered {
				return result
			}
		} else {
			recovered, values, newErr := strategy.Recover(ctx, err, parent)
			if recovered {
				return recoveryResult{
					recovered:    true,
					values:       values,
					err:          newErr,
					strategyName: c.Name(),
				}
			}
		}
		// Reset cursor for next strategy attempt
		ctx.restoreCheckpoint(checkpoint)
	}
	return recoveryResult{recovered: false, err: err, strategyName: c.Name()}
}

// Name implements EnhancedRecoveryStrategy.
func (c *CompositeStrategy) Name() string {
	return "composite"
}

// Helper functions for checkpoint-based recovery

// saveCheckpoint saves the current lexer position for potential restoration.
func (p *parseContext) saveCheckpoint() lexer.Checkpoint {
	return p.PeekingLexer.MakeCheckpoint()
}

// restoreCheckpoint restores the lexer to a previously saved position.
func (p *parseContext) restoreCheckpoint(cp lexer.Checkpoint) {
	p.PeekingLexer.LoadCheckpoint(cp)
}

// =============================================================================
// Per-Node Recovery Configuration
// =============================================================================

// nodeRecoveryConfig holds recovery configuration attached to a grammar node.
// This enables Chumsky-style per-parser recovery strategies.
type nodeRecoveryConfig struct {
	// Strategies to try for this node (in order)
	strategies []RecoveryStrategy
	// Label for error messages (e.g., "expression", "statement")
	label string
}

// recoverableNode is an optional interface that nodes can implement
// to support per-node recovery configuration.
type recoverableNode interface {
	node
	// SetRecovery sets the recovery configuration for this node.
	SetRecovery(config *nodeRecoveryConfig)
	// GetRecovery returns the recovery configuration, or nil if none.
	GetRecovery() *nodeRecoveryConfig
}

// =============================================================================
// Recovery Tag Parsing
// =============================================================================

// recoveryTagPattern matches recovery tag expressions like:
// - skip_until(;)
// - skip_past(;, })
// - nested((, ))
// - nested((, ), [(, )])  - with additional delimiters
// - retry_until(;)
var recoveryTagPattern = regexp.MustCompile(`^(\w+)\((.*)\)$`)

// parseRecoveryTag parses a recovery struct tag into a RecoveryStrategy.
//
// Supported strategy formats:
//   - skip_until(tok1, tok2, ...)     - skip until one of the tokens, don't consume
//   - skip_past(tok1, tok2, ...)      - skip until and consume the sync token
//   - nested(start, end)              - skip to balanced delimiter
//   - nested(start, end, [s1, e1])    - with additional delimiter pairs
//   - retry(tok1, tok2, ...)          - skip and retry until tokens (alias: retry_until)
//
// Multiple strategies are separated by semicolon (;):
//   - "nested((, )); skip_until(;)"
//
// Legacy syntax with pipe (|) is also supported for backwards compatibility:
//   - "nested((, ))|skip_until(;)"
//
// Labels should use a separate `label` struct tag for clarity:
//   - `parser:"@@" recover:"skip_until(;)" label:"expression"`
//
// Legacy inline label syntax is still supported:
//   - "label:name; skip_until(;)"
func parseRecoveryTag(tag string) (*nodeRecoveryConfig, error) {
	if tag == "" {
		return nil, nil
	}

	config := &nodeRecoveryConfig{}

	// Normalize separators: support both ; and | for backwards compatibility
	// Replace | with ; for uniform processing
	tag = strings.ReplaceAll(tag, "|", ";")

	// Split by ; for multiple strategies
	strategyStrs := splitStrategies(tag)
	for _, stratStr := range strategyStrs {
		stratStr = strings.TrimSpace(stratStr)
		if stratStr == "" {
			continue
		}

		// Check for label prefix (legacy inline syntax)
		if strings.HasPrefix(stratStr, "label:") {
			config.label = strings.TrimSpace(stratStr[6:])
			continue
		}

		strategy, err := parseSingleRecoveryStrategy(stratStr)
		if err != nil {
			return nil, err
		}
		if strategy != nil {
			config.strategies = append(config.strategies, strategy)
		}
	}

	if len(config.strategies) == 0 && config.label == "" {
		return nil, nil
	}
	return config, nil
}

// splitStrategies splits a recover tag by semicolons, but respects parentheses.
// This allows tokens like ";" inside strategy arguments.
func splitStrategies(tag string) []string {
	var result []string
	var current strings.Builder
	depth := 0

	for _, r := range tag {
		switch r {
		case '(':
			depth++
			current.WriteRune(r)
		case ')':
			depth--
			current.WriteRune(r)
		case ';':
			if depth == 0 {
				result = append(result, current.String())
				current.Reset()
			} else {
				current.WriteRune(r)
			}
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// parseSingleRecoveryStrategy parses a single recovery strategy expression.
func parseSingleRecoveryStrategy(expr string) (RecoveryStrategy, error) {
	matches := recoveryTagPattern.FindStringSubmatch(expr)
	if matches == nil {
		return nil, fmt.Errorf("invalid recovery strategy syntax: %q", expr)
	}

	name := matches[1]
	argsStr := matches[2]

	switch name {
	case "skip_until":
		tokens := parseTokenList(argsStr)
		if len(tokens) == 0 {
			return nil, fmt.Errorf("skip_until requires at least one token")
		}
		return SkipUntil(tokens...), nil

	case "skip_past":
		tokens := parseTokenList(argsStr)
		if len(tokens) == 0 {
			return nil, fmt.Errorf("skip_past requires at least one token")
		}
		return SkipPast(tokens...), nil

	case "retry_until", "retry":
		tokens := parseTokenList(argsStr)
		if len(tokens) == 0 {
			return nil, fmt.Errorf("retry requires at least one token")
		}
		return SkipThenRetryUntil(tokens...), nil

	case "nested":
		return parseNestedStrategy(argsStr)

	default:
		return nil, fmt.Errorf("unknown recovery strategy: %q", name)
	}
}

// parseTokenList parses a comma-separated list of tokens.
// Handles both quoted and unquoted tokens.
func parseTokenList(s string) []string {
	var tokens []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, r := range s {
		switch {
		case !inQuote && (r == '"' || r == '\''):
			inQuote = true
			quoteChar = r
		case inQuote && r == quoteChar:
			inQuote = false
			quoteChar = 0
		case !inQuote && r == ',':
			if tok := strings.TrimSpace(current.String()); tok != "" {
				tokens = append(tokens, tok)
			}
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}

	if tok := strings.TrimSpace(current.String()); tok != "" {
		tokens = append(tokens, tok)
	}

	return tokens
}

// parseNestedStrategy parses a nested() strategy expression.
// Formats:
//   - nested(start, end)
//   - nested(start, end, [s1, e1], [s2, e2], ...)
func parseNestedStrategy(argsStr string) (RecoveryStrategy, error) {
	// Simple parsing: split by commas, handling brackets
	args := parseNestedArgs(argsStr)

	if len(args) < 2 {
		return nil, fmt.Errorf("nested requires at least start and end delimiters")
	}

	start := strings.TrimSpace(args[0])
	end := strings.TrimSpace(args[1])

	var others [][2]string
	for i := 2; i < len(args); i++ {
		// Parse [s, e] format
		arg := strings.TrimSpace(args[i])
		if strings.HasPrefix(arg, "[") && strings.HasSuffix(arg, "]") {
			inner := arg[1 : len(arg)-1]
			parts := strings.SplitN(inner, ",", 2)
			if len(parts) == 2 {
				others = append(others, [2]string{
					strings.TrimSpace(parts[0]),
					strings.TrimSpace(parts[1]),
				})
			}
		}
	}

	return NestedDelimiters(start, end, others...), nil
}

// parseNestedArgs splits arguments while respecting bracket nesting.
func parseNestedArgs(s string) []string {
	var args []string
	var current strings.Builder
	depth := 0

	for _, r := range s {
		switch r {
		case '[':
			depth++
			current.WriteRune(r)
		case ']':
			depth--
			current.WriteRune(r)
		case ',':
			if depth == 0 {
				args = append(args, current.String())
				current.Reset()
			} else {
				current.WriteRune(r)
			}
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}

// =============================================================================
// Field Recovery Tag Extraction
// =============================================================================

// fieldRecoveryTag extracts the recovery tag from a struct field.
func fieldRecoveryTag(field reflect.StructField) string {
	return field.Tag.Get("recover")
}

// =============================================================================
// Recovery-Aware Wrapper Node
// =============================================================================

// recoveryNode wraps another node with recovery configuration.
// This allows any node to have per-node recovery without modifying all node types.
type recoveryNode struct {
	inner    node
	recovery *nodeRecoveryConfig
}

func (r *recoveryNode) String() string   { return r.inner.String() }
func (r *recoveryNode) GoString() string { return fmt.Sprintf("recovery{%s}", r.inner.GoString()) }

func (r *recoveryNode) Parse(ctx *parseContext, parent reflect.Value) ([]reflect.Value, error) {
	// Save checkpoint for potential recovery
	checkpoint := ctx.saveCheckpoint()
	startPos := ctx.Peek().Pos

	// Try parsing normally
	values, err := r.inner.Parse(ctx, parent)

	// If parsing succeeded (values != nil, err == nil), return normally
	if err == nil && values != nil {
		return values, nil
	}

	// If no recovery strategies configured, just return the result
	if r.recovery == nil || len(r.recovery.strategies) == 0 {
		return values, err
	}

	// Check if we've exceeded max errors
	if ctx.recovery != nil && ctx.recovery.maxErrors > 0 && len(ctx.recoveryErrors) >= ctx.recovery.maxErrors {
		ctx.traceRecovery("max errors (%d) reached, not attempting recovery", ctx.recovery.maxErrors)
		return values, err
	}

	// Determine the error to report
	// If there was no explicit error but also no match (nil, nil), create an error
	reportErr := err
	if reportErr == nil {
		// Get current token to create a meaningful error
		tok := ctx.Peek()
		reportErr = &UnexpectedTokenError{Unexpected: *tok, expectNode: r.inner}
	}

	// Extract expected message from the underlying error
	var expectedMsg string
	if perr, ok := reportErr.(Error); ok {
		expectedMsg = perr.Message()
	} else {
		expectedMsg = reportErr.Error()
	}

	// Trace recovery attempt
	ctx.traceRecoveryAttempt(startPos, reportErr, r.inner.String())

	// Try each recovery strategy
	for _, strategy := range r.recovery.strategies {
		ctx.restoreCheckpoint(checkpoint)
		currentPos := ctx.Peek().Pos

		// Check if this is a retry strategy (like skip_then_retry_until)
		if retryStrat, ok := strategy.(RetryStrategy); ok && retryStrat.IsRetryStrategy() {
			ctx.traceRecoveryStrategy("skip_then_retry", currentPos)
			// Retry strategy: skip tokens one at a time and retry parsing at each position
			recoveredValues, skippedTokens, recovered := r.retryRecovery(ctx, retryStrat, reportErr, parent)
			if recovered {
				endPos := ctx.Peek().Pos
				ctx.traceRecoverySuccess("skip_then_retry", len(skippedTokens), endPos)
				r.recordEnhancedRecoveryError(ctx, startPos, endPos, expectedMsg, skippedTokens, "skip_then_retry", reportErr)
				if len(recoveredValues) > 0 {
					return recoveredValues, nil
				}
				return []reflect.Value{}, nil
			}
			ctx.traceRecoveryFailed("skip_then_retry", "could not find valid parse position")
			continue
		}

		// Try enhanced recovery first if available
		if enhanced, ok := strategy.(EnhancedRecoveryStrategy); ok {
			strategyName := enhanced.Name()
			ctx.traceRecoveryStrategy(strategyName, currentPos)
			result := enhanced.RecoverWithContext(ctx, reportErr, parent)
			if result.recovered {
				endPos := ctx.Peek().Pos
				ctx.traceRecoverySuccess(strategyName, len(result.skippedTokens), endPos)
				r.recordEnhancedRecoveryError(ctx, startPos, endPos, expectedMsg, result.skippedTokens, result.strategyName, reportErr)
				if len(result.values) > 0 {
					return result.values, nil
				}
				return []reflect.Value{}, nil
			}
			ctx.traceRecoveryFailed(strategyName, "strategy returned false")
			continue
		}

		// Regular strategy (fallback)
		ctx.traceRecoveryStrategy("unknown", currentPos)
		recovered, recoveredValues, _ := strategy.Recover(ctx, reportErr, parent)
		if recovered {
			endPos := ctx.Peek().Pos
			ctx.traceRecoverySuccess("unknown", 0, endPos)
			r.recordEnhancedRecoveryError(ctx, startPos, endPos, expectedMsg, nil, "unknown", reportErr)
			if len(recoveredValues) > 0 {
				return recoveredValues, nil
			}
			return []reflect.Value{}, nil
		}
		ctx.traceRecoveryFailed("unknown", "strategy returned false")
	}

	// No strategy succeeded, restore and return original result
	ctx.traceRecoveryAllFailed()
	ctx.restoreCheckpoint(checkpoint)
	return values, err
}

// retryRecovery implements the skip-then-retry-at-each-position behavior.
// Returns recovered values, skipped tokens, and whether recovery succeeded.
func (r *recoveryNode) retryRecovery(ctx *parseContext, strategy RetryStrategy, originalErr error, parent reflect.Value) ([]reflect.Value, []lexer.Token, bool) {
	// Get max skip from the strategy if it's our SkipThenRetryUntilStrategy
	maxSkip := 100
	if stru, ok := strategy.(*SkipThenRetryUntilStrategy); ok {
		if stru.MaxSkip > 0 {
			maxSkip = stru.MaxSkip
		}
	}

	var skippedTokens []lexer.Token
	for len(skippedTokens) < maxSkip {
		// Check if we should stop (hit termination token or EOF)
		if strategy.ShouldStop(ctx) {
			return nil, skippedTokens, false
		}

		// Skip one token
		skippedTokens = append(skippedTokens, *ctx.Next())

		// Try to parse again
		values, err := r.inner.Parse(ctx, parent)

		// If parsing succeeded (values != nil, no error), we recovered!
		if err == nil && values != nil {
			return values, skippedTokens, true
		}

		// If we got an error but made progress, that's also recovery
		// (we found a valid starting point even if parsing failed later)
	}

	return nil, skippedTokens, false
}

// recordEnhancedRecoveryError records a recovery error with rich context.
func (r *recoveryNode) recordEnhancedRecoveryError(ctx *parseContext, startPos, endPos lexer.Position, message string, skippedTokens []lexer.Token, strategyName string, underlying error) {
	err := makeRecoveredError(
		startPos,
		endPos,
		message,
		r.recovery.label,
		skippedTokens,
		strategyName,
		underlying,
	)
	ctx.addRecoveryError(err)
}

// SetRecovery implements recoverableNode.
func (r *recoveryNode) SetRecovery(config *nodeRecoveryConfig) {
	r.recovery = config
}

// GetRecovery implements recoverableNode.
func (r *recoveryNode) GetRecovery() *nodeRecoveryConfig {
	return r.recovery
}

// wrapWithRecovery wraps a node with recovery configuration if provided.
func wrapWithRecovery(n node, config *nodeRecoveryConfig) node {
	if config == nil || (len(config.strategies) == 0 && config.label == "") {
		return n
	}
	return &recoveryNode{inner: n, recovery: config}
}
