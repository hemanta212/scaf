//nolint:testpackage
package cypher

import (
	"slices"
	"testing"

	"github.com/rlch/scaf"
)

func TestDialect_Name(t *testing.T) {
	d := NewDialect()

	if got := d.Name(); got != scaf.DialectCypher {
		t.Errorf("Name() = %q, want %q", got, scaf.DialectCypher)
	}
}

func TestDialect_ImplementsInterface(_ *testing.T) {
	var _ scaf.Dialect = (*Dialect)(nil)
}

func TestDialect_Registration(t *testing.T) {
	dialects := scaf.RegisteredDialects()

	if !slices.Contains(dialects, scaf.DialectCypher) {
		t.Error("cypher dialect not registered")
	}
}

func TestDialect_Analyze(t *testing.T) {
	d := NewDialect()

	query := "MATCH (u:User {id: $id}) RETURN u.name AS name, u.email AS email"
	metadata, err := d.Analyze(query)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	// Check parameters
	if len(metadata.Parameters) != 1 {
		t.Errorf("Analyze() parameters = %d, want 1", len(metadata.Parameters))
	} else if metadata.Parameters[0].Name != "id" {
		t.Errorf("Analyze() parameter name = %q, want %q", metadata.Parameters[0].Name, "id")
	}

	// Check returns
	if len(metadata.Returns) != 2 {
		t.Errorf("Analyze() returns = %d, want 2", len(metadata.Returns))
	}
}
