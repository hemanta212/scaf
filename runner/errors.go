package runner

import "errors"

// Sentinel errors for the runner package.
var (
	// ErrMaxFailures is returned when the max failure limit is reached.
	ErrMaxFailures = errors.New("runner: max failures reached")

	// ErrSetupFailed is returned when a setup block fails.
	ErrSetupFailed = errors.New("runner: setup failed")

	// ErrNoDialect is returned when no dialect is configured.
	ErrNoDialect = errors.New("runner: no dialect configured")

	// ErrConnectionFailed is returned when database connection fails.
	ErrConnectionFailed = errors.New("runner: database connection failed")

	// Test errors for use in unit tests.
	errTestSetupFailed = errors.New("test: setup failed")
	errTestStop        = errors.New("test: stop")
	errTestFail        = errors.New("test: fail")
)
