// Package cypher provides a scaf Dialect for Cypher query analysis.
package cypher

import "github.com/rlch/scaf"

//nolint:gochecknoinits // Dialect self-registration pattern
func init() {
	scaf.RegisterDialect(NewDialect())
}

// Dialect implements scaf.Dialect for Cypher query analysis.
type Dialect struct{}

// NewDialect creates a new Cypher dialect for query analysis.
func NewDialect() *Dialect {
	return &Dialect{}
}

// Name returns the dialect identifier.
func (d *Dialect) Name() string {
	return scaf.DialectCypher
}

// Analyze extracts metadata from a Cypher query.
func (d *Dialect) Analyze(query string) (*scaf.QueryMetadata, error) {
	return NewAnalyzer().AnalyzeQuery(query)
}

var _ scaf.Dialect = (*Dialect)(nil)
