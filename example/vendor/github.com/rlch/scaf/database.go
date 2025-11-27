package scaf

import (
	"context"
	"fmt"
)

// Database represents an execution target (neo4j, postgres, mysql).
// It handles connection establishment and query execution.
type Database interface {
	// Name returns the database identifier (e.g., "neo4j", "postgres").
	Name() string

	// Dialect returns the query language this database uses.
	Dialect() Dialect

	// Execute runs a query with parameters and returns results.
	Execute(ctx context.Context, query string, params map[string]any) ([]map[string]any, error)

	// Close releases database resources.
	Close() error
}

// DatabaseTransaction represents an active database transaction.
// Queries executed through a transaction are isolated until Commit or Rollback.
type DatabaseTransaction interface {
	// Execute runs a query within this transaction.
	Execute(ctx context.Context, query string, params map[string]any) ([]map[string]any, error)

	// Commit commits the transaction.
	Commit(ctx context.Context) error

	// Rollback aborts the transaction.
	Rollback(ctx context.Context) error
}

// TransactionalDatabase is implemented by databases that support transactions.
// The runner uses this for test isolation (rollback after each test).
type TransactionalDatabase interface {
	Database

	// Begin starts a new transaction.
	Begin(ctx context.Context) (DatabaseTransaction, error)
}

// DatabaseFactory creates a Database from configuration.
type DatabaseFactory func(cfg any) (Database, error)

var databases = make(map[string]DatabaseFactory)

// RegisterDatabase registers a database factory by name.
func RegisterDatabase(name string, factory DatabaseFactory) {
	databases[name] = factory
}

// NewDatabase creates a database instance by name.
func NewDatabase(name string, cfg any) (Database, error) { //nolint:ireturn
	factory, ok := databases[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownDatabase, name)
	}

	return factory(cfg)
}

// RegisteredDatabases returns the names of all registered databases.
func RegisteredDatabases() []string {
	names := make([]string, 0, len(databases))
	for name := range databases {
		names = append(names, name)
	}

	return names
}
