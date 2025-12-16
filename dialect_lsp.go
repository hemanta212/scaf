package scaf

// DialectLSP provides LSP features for positions inside query bodies.
// Dialects optionally implement this interface to provide rich editing support
// for their query language (completions, hover, diagnostics, etc.).
//
// The LSP server detects when the cursor is inside a query body (backtick string)
// and delegates to the dialect's LSP implementation for that content.
//
// All positions are byte offsets within the query string (not the document).
// The LSP server handles mapping between document positions and query offsets.
type DialectLSP interface {
	// Complete returns completions for a position within a query body.
	// offset is the byte offset within the query string.
	// ctx provides schema info, surrounding scaf context, etc.
	Complete(query string, offset int, ctx *QueryLSPContext) []QueryCompletion

	// Hover returns hover info for a position within a query body.
	// Returns nil if no hover info is available at this position.
	Hover(query string, offset int, ctx *QueryLSPContext) *QueryHover

	// Diagnostics returns syntax/semantic errors for a query body.
	// Called when the document changes to provide inline error squiggles.
	Diagnostics(query string, ctx *QueryLSPContext) []QueryDiagnostic

	// SignatureHelp returns signature info for function calls at a position.
	// Returns nil if not inside a function call.
	SignatureHelp(query string, offset int, ctx *QueryLSPContext) *QuerySignatureHelp

	// Definition returns go-to-definition targets for a position.
	// Used for navigating to label/type definitions in schema.
	// Returns nil if no definition is available.
	Definition(query string, offset int, ctx *QueryLSPContext) []QueryLocation

	// InlayHints returns inlay hints for a query body.
	// Used to show inferred types for parameters that lack explicit type annotations.
	// The hints identify parameters by name; the LSP server positions them in the function signature.
	InlayHints(query string, ctx *QueryLSPContext) []QueryInlayHint
}

// QueryLSPContext provides context for dialect LSP features.
// Contains schema information and surrounding scaf context.
type QueryLSPContext struct {
	// Schema provides type information for models/labels.
	// This is an interface{} to avoid import cycles - cast to *analysis.TypeSchema when needed.
	// May be nil if no schema is available.
	Schema any

	// FunctionScope is the enclosing function name (e.g., "GetUser").
	FunctionScope string

	// FilePath is the document path for resolving relative references.
	FilePath string

	// DeclaredParams contains parameter names declared in the function signature.
	// Maps parameter name to its type expression (nil if untyped).
	DeclaredParams map[string]*TypeExpr

	// TriggerCharacter is the character that triggered completion (e.g., ".", ":").
	// Empty for manual/typing triggers.
	TriggerCharacter string
}

// QueryCompletion represents a completion item from a dialect.
type QueryCompletion struct {
	// Label is the text shown in the completion list.
	Label string

	// Kind indicates the type of completion (keyword, function, label, etc.).
	Kind QueryCompletionKind

	// Detail is additional info shown next to the label (e.g., function signature).
	Detail string

	// Documentation is longer documentation shown in a hover panel.
	// Supports markdown formatting.
	Documentation string

	// InsertText is the text to insert when the completion is selected.
	// If empty, Label is used.
	InsertText string

	// IsSnippet indicates InsertText uses snippet syntax with placeholders.
	// E.g., "count(${1:expression})" where ${1:...} is a placeholder.
	IsSnippet bool

	// SortText is used to control sort order. If empty, Label is used.
	SortText string

	// FilterText is used for filtering. If empty, Label is used.
	FilterText string

	// Deprecated indicates this item is deprecated.
	Deprecated bool
}

// QueryCompletionKind indicates the type of a completion item.
type QueryCompletionKind int

const (
	// QueryCompletionKeyword is a language keyword (MATCH, RETURN, WHERE, etc.).
	QueryCompletionKeyword QueryCompletionKind = iota

	// QueryCompletionFunction is a built-in or user-defined function.
	QueryCompletionFunction

	// QueryCompletionLabel is a node label or entity type from the schema.
	QueryCompletionLabel

	// QueryCompletionProperty is a property name from the schema.
	QueryCompletionProperty

	// QueryCompletionRelType is a relationship type from the schema.
	QueryCompletionRelType

	// QueryCompletionVariable is a variable defined in the query.
	QueryCompletionVariable

	// QueryCompletionParameter is a $parameter from the function signature.
	QueryCompletionParameter

	// QueryCompletionSnippet is a code snippet/template.
	QueryCompletionSnippet

	// QueryCompletionOperator is an operator (+, -, =, etc.).
	QueryCompletionOperator

	// QueryCompletionProcedure is a stored procedure.
	QueryCompletionProcedure
)

// QueryHover contains hover information for a position in a query.
type QueryHover struct {
	// Contents is the hover text in markdown format.
	Contents string

	// Range is the range in the query that this hover applies to.
	// Used to highlight the relevant token. If nil, the word at cursor is used.
	Range *QueryRange
}

// QueryRange represents a range within a query string.
// Positions are byte offsets from the start of the query.
type QueryRange struct {
	// Start is the byte offset of the range start (inclusive).
	Start int
	// End is the byte offset of the range end (exclusive).
	End int
}

// QueryDiagnostic represents an error or warning in a query.
type QueryDiagnostic struct {
	// Range is the location of the diagnostic in the query.
	Range QueryRange

	// Severity indicates error vs warning vs info vs hint.
	Severity QueryDiagnosticSeverity

	// Message is the diagnostic message.
	Message string

	// Code is an optional diagnostic code (e.g., "unknown-label").
	Code string
}

// QueryDiagnosticSeverity indicates the severity of a diagnostic.
type QueryDiagnosticSeverity int

const (
	// QueryDiagnosticError indicates a problem that will cause query failure.
	QueryDiagnosticError QueryDiagnosticSeverity = iota + 1

	// QueryDiagnosticWarning indicates a potential problem.
	QueryDiagnosticWarning

	// QueryDiagnosticInfo indicates informational message.
	QueryDiagnosticInfo

	// QueryDiagnosticHint indicates a hint for improvement.
	QueryDiagnosticHint
)

// QuerySignatureHelp provides signature information for function calls.
type QuerySignatureHelp struct {
	// Signatures is the list of possible signatures.
	Signatures []QuerySignature

	// ActiveSignature is the index of the currently active signature.
	ActiveSignature int

	// ActiveParameter is the index of the currently active parameter.
	ActiveParameter int
}

// QuerySignature describes a function signature.
type QuerySignature struct {
	// Label is the full signature label (e.g., "count(expression) → integer").
	Label string

	// Documentation is longer documentation for this signature.
	Documentation string

	// Parameters describes each parameter.
	Parameters []QueryParameterInfo
}

// QueryParameterInfo describes a function parameter for signature help.
type QueryParameterInfo struct {
	// Label is the parameter label shown in the signature.
	// Can be the parameter name or a range [start, end] into the signature label.
	Label string

	// Documentation is longer documentation for this parameter.
	Documentation string
}

// QueryLocation represents a location for go-to-definition results.
type QueryLocation struct {
	// URI is the file URI (for cross-file definitions).
	// Empty string means the current file.
	URI string

	// Range is the location within the query or file.
	Range QueryRange
}

// QueryInlayHint represents an inlay hint from a dialect.
// Used to display inferred type information for parameters.
type QueryInlayHint struct {
	// ParameterName identifies which function parameter this hint is for.
	// The LSP server uses this to position the hint after the parameter name
	// in the function signature.
	ParameterName string

	// Label is the hint text to display (e.g., ": string").
	Label string

	// Kind indicates the type of hint.
	Kind QueryInlayHintKind

	// Tooltip provides additional information shown on hover.
	// Supports markdown formatting.
	Tooltip string
}

// QueryInlayHintKind indicates the type of an inlay hint.
type QueryInlayHintKind int

const (
	// QueryInlayHintType is a type annotation hint.
	QueryInlayHintType QueryInlayHintKind = iota

	// QueryInlayHintParameter is a parameter name hint.
	QueryInlayHintParameter
)

// GetDialectLSP returns the DialectLSP implementation for a dialect.
// Returns nil if the dialect doesn't implement DialectLSP.
func GetDialectLSP(dialectName string) DialectLSP { //nolint:ireturn
	d := GetDialect(dialectName)
	if d == nil {
		return nil
	}

	// Check if dialect implements DialectLSP
	if lsp, ok := d.(DialectLSP); ok {
		return lsp
	}

	return nil
}
