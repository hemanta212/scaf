package scaf

import "errors"

// Sentinel errors.
var (
	// ErrConfigNotFound is returned when no .scaf.yaml is found.
	ErrConfigNotFound = errors.New("scaf: no .scaf.yaml found")

	// ErrUnknownDatabase is returned when an unknown database is requested.
	ErrUnknownDatabase = errors.New("scaf: unknown database")
)
