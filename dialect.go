package scaf

import (
	"context"
	"errors"
	"fmt"
)

// ErrNoTransactionSupport is returned when a dialect does not support transactions.
var ErrNoTransactionSupport = errors.New("dialect does not support transactions")

// Dialect represents a query language (cypher, sql).
// It provides static analysis of queries without requiring a database connection.
type Dialect interface {
	// Name returns the dialect identifier (e.g., "cypher", "sql").
	Name() string

	// Analyze extracts metadata from a query string.
	Analyze(query string) (*QueryMetadata, error)
}

var newDialects = make(map[string]Dialect)

// RegisterDialectInstance registers a dialect instance by name.
// This is the new registration method for pure dialect implementations.
func RegisterDialectInstance(d Dialect) {
	newDialects[d.Name()] = d
}

// GetDialect returns a dialect by name.
// Returns nil if no dialect is registered with that name.
func GetDialect(name string) Dialect { //nolint:ireturn
	return newDialects[name]
}

// RegisteredDialectInstances returns the names of all registered dialect instances.
func RegisteredDialectInstances() []string {
	names := make([]string, 0, len(newDialects))
	for name := range newDialects {
		names = append(names, name)
	}

	return names
}

// =============================================================================
// DEPRECATED: Legacy Dialect interface for backwards compatibility
// These types will be removed after migration to Database interface.
// =============================================================================

// LegacyDialect defines the old interface for database backends.
// Deprecated: Use Database interface instead.
type LegacyDialect interface {
	// Name returns the dialect identifier (e.g., "neo4j", "postgres").
	Name() string

	// Execute runs a query with parameters and returns the results.
	Execute(ctx context.Context, query string, params map[string]any) ([]map[string]any, error)

	// Close releases any resources held by the dialect.
	Close() error
}

// Transaction represents an active database transaction.
// Queries executed through a transaction are isolated until Commit or Rollback.
type Transaction interface {
	// Execute runs a query within this transaction.
	Execute(ctx context.Context, query string, params map[string]any) ([]map[string]any, error)

	// Commit commits the transaction.
	Commit(ctx context.Context) error

	// Rollback aborts the transaction.
	Rollback(ctx context.Context) error
}

// Transactional is an optional interface for dialects that support transactions.
// Deprecated: Use TransactionalDatabase instead.
type Transactional interface {
	LegacyDialect

	// Begin starts a new transaction.
	Begin(ctx context.Context) (Transaction, error)
}

// DialectFactory creates a LegacyDialect from connection configuration.
// Deprecated: Use DatabaseFactory instead.
type DialectFactory func(cfg DialectConfig) (LegacyDialect, error)

// DialectConfig holds connection settings for a dialect.
// Deprecated: Use database-specific config types instead.
type DialectConfig struct {
	// Connection URI (e.g., "bolt://localhost:7687", "postgres://localhost/db")
	URI string `yaml:"uri"`

	// Optional credentials (if not in URI)
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`

	// Dialect-specific options
	Options map[string]any `yaml:"options,omitempty"`
}

var legacyDialects = make(map[string]DialectFactory)

// RegisterDialect registers a legacy dialect factory by name.
// Deprecated: Use RegisterDatabase instead.
func RegisterDialect(name string, factory DialectFactory) {
	legacyDialects[name] = factory
}

// NewDialect creates a legacy dialect instance by name.
// Deprecated: Use NewDatabase instead.
func NewDialect(name string, cfg DialectConfig) (LegacyDialect, error) { //nolint:ireturn
	factory, ok := legacyDialects[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownDialect, name)
	}

	d, err := factory(cfg)
	if err != nil {
		return nil, err
	}

	return &dialectWrapper{d}, nil
}

// dialectWrapper wraps a LegacyDialect to return concrete type.
type dialectWrapper struct {
	LegacyDialect
}

// Begin delegates to the underlying dialect if it supports transactions.
func (w *dialectWrapper) Begin(ctx context.Context) (Transaction, error) { //nolint:ireturn
	if tx, ok := w.LegacyDialect.(Transactional); ok {
		return tx.Begin(ctx)
	}

	return nil, ErrNoTransactionSupport
}

// Transactional returns true if the underlying dialect supports transactions.
func (w *dialectWrapper) Transactional() bool {
	_, ok := w.LegacyDialect.(Transactional)

	return ok
}

// RegisteredDialects returns the names of all registered legacy dialects.
// Deprecated: Use RegisteredDatabases instead.
func RegisteredDialects() []string {
	names := make([]string, 0, len(legacyDialects))
	for name := range legacyDialects {
		names = append(names, name)
	}

	return names
}

// =============================================================================
// Query Analyzer (being merged into Dialect)
// =============================================================================

// QueryAnalyzer provides static analysis of queries for IDE features.
// Deprecated: Use Dialect.Analyze() instead.
type QueryAnalyzer interface {
	// AnalyzeQuery extracts metadata from a query string.
	AnalyzeQuery(query string) (*QueryMetadata, error)
}

// QueryAnalyzerFactory creates a QueryAnalyzer for a dialect.
type QueryAnalyzerFactory func() QueryAnalyzer

var analyzers = make(map[string]QueryAnalyzerFactory)

// RegisterAnalyzer registers a query analyzer factory by dialect name.
// Dialects should call this in their init() function.
func RegisterAnalyzer(dialectName string, factory QueryAnalyzerFactory) {
	analyzers[dialectName] = factory
}

// GetAnalyzer returns a QueryAnalyzer for the given dialect name.
// Returns nil if no analyzer is registered for that dialect.
// Deprecated: Use GetDialect(name).Analyze() instead.
func GetAnalyzer(dialectName string) QueryAnalyzer { //nolint:ireturn
	// First check new dialect instances
	if d := GetDialect(dialectName); d != nil {
		return &dialectAnalyzerAdapter{d}
	}

	// Fall back to legacy analyzers
	factory, ok := analyzers[dialectName]
	if !ok {
		return nil
	}

	return factory()
}

// dialectAnalyzerAdapter adapts a Dialect to the QueryAnalyzer interface.
type dialectAnalyzerAdapter struct {
	dialect Dialect
}

func (a *dialectAnalyzerAdapter) AnalyzeQuery(query string) (*QueryMetadata, error) {
	return a.dialect.Analyze(query)
}

// RegisteredAnalyzers returns the names of all registered analyzers.
func RegisteredAnalyzers() []string {
	names := make([]string, 0, len(analyzers))
	for name := range analyzers {
		names = append(names, name)
	}

	return names
}

// MarkdownLanguage returns the markdown language identifier for a dialect.
// Used for syntax highlighting in IDE hover/completion documentation.
func MarkdownLanguage(dialectName string) string {
	// Common dialect name to markdown language mapping
	switch dialectName {
	case DialectCypher, DatabaseNeo4j:
		return DialectCypher
	case DatabasePostgres, "postgresql", DatabaseMySQL, DatabaseSQLite, DialectSQL:
		return DialectSQL
	default:
		return dialectName
	}
}

// QueryMetadata holds extracted information about a query.
type QueryMetadata struct {
	// Parameters are the $-prefixed parameters used in the query.
	Parameters []ParameterInfo

	// Returns are the fields returned by the query.
	Returns []ReturnInfo
}

// ParameterInfo describes a query parameter.
type ParameterInfo struct {
	// Name is the parameter name (without $ prefix).
	Name string

	// Type is the inferred type, if known (e.g., "string", "int").
	Type string

	// Position is the character offset in the query.
	Position int

	// Line is the 1-indexed line number in the query.
	Line int

	// Column is the 1-indexed column in the query.
	Column int

	// Length is the length of the parameter reference in characters.
	Length int

	// Count is how many times this parameter appears.
	Count int
}

// ReturnInfo describes a returned field.
type ReturnInfo struct {
	// Name is the field name or alias.
	Name string

	// Type is the inferred type, if known.
	Type string

	// Expression is the original expression text.
	Expression string

	// Alias is the explicit alias if AS keyword was used, empty otherwise.
	// When Alias is set, the database column name is Alias.
	// When Alias is empty, the database column name is Expression.
	Alias string

	// IsAggregate indicates this is an aggregate function result.
	IsAggregate bool

	// IsWildcard indicates this is a wildcard (*) return.
	IsWildcard bool
}
