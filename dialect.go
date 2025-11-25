package scaf

import (
	"context"
	"fmt"
)

// Dialect defines the interface for database backends.
type Dialect interface {
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
// The runner uses this for test isolation (rollback after each test).
type Transactional interface {
	Dialect

	// Begin starts a new transaction.
	Begin(ctx context.Context) (Transaction, error)
}

// DialectFactory creates a Dialect from connection configuration.
type DialectFactory func(cfg DialectConfig) (Dialect, error)

// DialectConfig holds connection settings for a dialect.
type DialectConfig struct {
	// Connection URI (e.g., "bolt://localhost:7687", "postgres://localhost/db")
	URI string `yaml:"uri"`

	// Optional credentials (if not in URI)
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`

	// Dialect-specific options
	Options map[string]any `yaml:"options,omitempty"`
}

var dialects = make(map[string]DialectFactory)

// RegisterDialect registers a dialect factory by name.
func RegisterDialect(name string, factory DialectFactory) {
	dialects[name] = factory
}

// NewDialect creates a dialect instance by name.
func NewDialect(name string, cfg DialectConfig) (*dialectWrapper, error) {
	factory, ok := dialects[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownDialect, name)
	}

	d, err := factory(cfg)
	if err != nil {
		return nil, err
	}

	return &dialectWrapper{d}, nil
}

// dialectWrapper wraps a Dialect to return concrete type.
type dialectWrapper struct {
	Dialect
}

// RegisteredDialects returns the names of all registered dialects.
func RegisteredDialects() []string {
	names := make([]string, 0, len(dialects))
	for name := range dialects {
		names = append(names, name)
	}

	return names
}
