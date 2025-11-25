package scaf

import "errors"

// Sentinel errors.
var (
	// ErrConfigNotFound is returned when no .scaf.yaml is found.
	ErrConfigNotFound = errors.New("scaf: no .scaf.yaml found")

	// ErrUnknownDialect is returned when an unknown dialect is requested.
	ErrUnknownDialect = errors.New("scaf: unknown dialect")
)
