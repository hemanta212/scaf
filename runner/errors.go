package runner

import "errors"

// Sentinel errors for the runner package.
var (
	// ErrMaxFailures is returned when the max failure limit is reached.
	ErrMaxFailures = errors.New("runner: max failures reached")

	// ErrSetupFailed is returned when a setup block fails.
	ErrSetupFailed = errors.New("runner: setup failed")

	// ErrNoDatabase is returned when no database is configured.
	ErrNoDatabase = errors.New("runner: no database configured")

	// ErrNoDialect is an alias for ErrNoDatabase for backwards compatibility.
	// Deprecated: Use ErrNoDatabase instead.
	ErrNoDialect = ErrNoDatabase

	// ErrConnectionFailed is returned when database connection fails.
	ErrConnectionFailed = errors.New("runner: database connection failed")

	// ErrUnknownQuery is returned when a referenced query is not found.
	ErrUnknownQuery = errors.New("runner: unknown query")

	// ErrExprNotBool is returned when an expression does not return a boolean.
	ErrExprNotBool = errors.New("runner: expression did not return bool")

	// ErrNoModuleContext is returned when named setup requires module resolution.
	ErrNoModuleContext = errors.New("runner: named setup requires module resolution")

	// ErrAssertNoQuery is returned when an assert has no inline or named query.
	ErrAssertNoQuery = errors.New("runner: assert query has no inline or named query")

	// Test errors for use in unit tests.
	errTestSetupFailed = errors.New("test: setup failed")
	errTestStop        = errors.New("test: stop")
	errTestFail        = errors.New("test: fail")
)
