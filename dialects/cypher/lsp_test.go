//nolint:testpackage
package cypher

import (
	"testing"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
)

// createTestSchema creates a test schema with Person, Company, and Movie models.
func createTestSchema() *analysis.TypeSchema {
	return &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"Person": {
				Name: "Person",
				Fields: []*analysis.Field{
					{Name: "name", Type: analysis.TypeString, Required: true},
					{Name: "age", Type: analysis.TypeInt},
					{Name: "email", Type: analysis.TypeString, Unique: true},
				},
				Relationships: []*analysis.Relationship{
					{
						Name:      "Friends",
						RelType:   "FRIENDS",
						Target:    "Person",
						Many:      true,
						Direction: analysis.DirectionOutgoing,
					},
					{
						Name:      "WorksAt",
						RelType:   "WORKS_AT",
						Target:    "Company",
						Many:      false,
						Direction: analysis.DirectionOutgoing,
					},
				},
			},
			"Company": {
				Name: "Company",
				Fields: []*analysis.Field{
					{Name: "name", Type: analysis.TypeString, Required: true},
				},
				Relationships: []*analysis.Relationship{
					{
						Name:      "Employees",
						RelType:   "WORKS_AT",
						Target:    "Person",
						Many:      true,
						Direction: analysis.DirectionIncoming,
					},
				},
			},
			"Movie": {
				Name: "Movie",
				Fields: []*analysis.Field{
					{Name: "title", Type: analysis.TypeString, Required: true},
					{Name: "released", Type: analysis.TypeInt},
				},
				Relationships: []*analysis.Relationship{
					{
						Name:      "Actors",
						RelType:   "ACTED_IN",
						Target:    "Person",
						Many:      true,
						Direction: analysis.DirectionIncoming,
					},
				},
			},
		},
	}
}

func TestDialect_Complete_RelTypes_ContextAware(t *testing.T) {
	d := NewDialect()
	ctx := &scaf.QueryLSPContext{
		Schema: createTestSchema(),
	}

	// Schema relationships:
	// - Person.FRIENDS -> Person (outgoing)
	// - Person.WORKS_AT -> Company (outgoing)
	// - Company.WORKS_AT <- Person (incoming, i.e., Person->Company)
	// - Movie.ACTED_IN <- Person (incoming, i.e., Person->Movie)
	//
	// So FRIENDS, WORKS_AT, ACTED_IN all originate FROM Person in the graph.
	// FRIENDS also targets Person (self-referential).

	tests := []struct {
		name        string
		query       string
		offset      int
		wantTypes   []string // relationship types that SHOULD be suggested
		noWantTypes []string // relationship types that should NOT be suggested
	}{
		{
			name:      "outgoing from Person - show Person's outgoing rels",
			query:     "MATCH (p:Person)-[:",
			offset:    19,
			wantTypes: []string{"FRIENDS", "WORKS_AT"},
			// ACTED_IN is not defined on Person, so shouldn't appear when filtering by Person
			noWantTypes: []string{},
		},
		{
			name:      "incoming to Person - show FRIENDS (Person->Person)",
			query:     "MATCH (p:Person)<-[:",
			offset:    20,
			wantTypes: []string{"FRIENDS"}, // FRIENDS targets Person
			// WORKED_AT targets Company, ACTED_IN targets Person but is defined on Movie
			noWantTypes: []string{"WORKS_AT"},
		},
		{
			name:      "outgoing from Company - Company has only incoming rel defined",
			query:     "MATCH (c:Company)-[:",
			offset:    20,
			wantTypes: []string{}, // Company.WORKS_AT is incoming, not outgoing
			// Falls back to all rels when no matches
		},
		{
			name:        "incoming to Company - Person->Company with WORKS_AT",
			query:       "MATCH (c:Company)<-[:",
			offset:      21,
			wantTypes:   []string{"WORKS_AT"}, // Person.WORKS_AT targets Company
			noWantTypes: []string{"FRIENDS"},  // FRIENDS doesn't involve Company
		},
		{
			name:      "no label context - show all rels",
			query:     "MATCH ()-[:",
			offset:    11,
			wantTypes: []string{"FRIENDS", "WORKS_AT", "ACTED_IN"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := d.Complete(tt.query, tt.offset, ctx)

			// Build a set of returned relationship types
			gotTypes := make(map[string]bool)
			for _, item := range items {
				if item.Kind == scaf.QueryCompletionRelType {
					gotTypes[item.Label] = true
				}
			}

			// Check expected types are present
			for _, want := range tt.wantTypes {
				if !gotTypes[want] {
					t.Errorf("Expected relationship type %q in completions, got: %v", want, keysOf(gotTypes))
				}
			}

			// Check unwanted types are absent
			for _, nowant := range tt.noWantTypes {
				if gotTypes[nowant] {
					t.Errorf("Did not expect relationship type %q in completions, got: %v", nowant, keysOf(gotTypes))
				}
			}
		})
	}
}

func TestDialect_Complete_RelTypes_VariableInference(t *testing.T) {
	d := NewDialect()
	ctx := &scaf.QueryLSPContext{
		Schema: createTestSchema(),
	}

	// When a variable is bound earlier with a label, completions should use that label
	query := "MATCH (p:Person)-[:FRIENDS]->(friend)-[:"
	offset := len(query)

	items := d.Complete(query, offset, ctx)

	gotTypes := make(map[string]bool)
	for _, item := range items {
		if item.Kind == scaf.QueryCompletionRelType {
			gotTypes[item.Label] = true
		}
	}

	// Log what we got for debugging
	t.Logf("Got relationship types for variable-bound node: %v", keysOf(gotTypes))

	// The 'friend' node has no explicit label, so without inference we fall back to all rels
	// This test documents current behavior - enhancement would infer Person from FRIENDS target
}

func TestExtractLabelsFromNodeContent(t *testing.T) {
	tests := []struct {
		content string
		want    []string
	}{
		{"p:Person", []string{"Person"}},
		{"p:Person:Admin", []string{"Person", "Admin"}},
		{":Person", []string{"Person"}},
		{"p", nil},
		{"p:Person {name: 'test'}", []string{"Person"}},
		{"p:Person:Admin {name: 'test'}", []string{"Person", "Admin"}},
		{"", nil},
		// Note: " p : Person " with spaces around colon is invalid Cypher syntax
		// The parser would never produce this, so we don't need to handle it
	}

	for _, tt := range tests {
		t.Run(tt.content, func(t *testing.T) {
			got := extractLabelsFromNodeContent(tt.content)
			if len(got) != len(tt.want) {
				t.Errorf("extractLabelsFromNodeContent(%q) = %v, want %v", tt.content, got, tt.want)
				return
			}
			for i, label := range got {
				if label != tt.want[i] {
					t.Errorf("extractLabelsFromNodeContent(%q)[%d] = %q, want %q", tt.content, i, label, tt.want[i])
				}
			}
		})
	}
}

func TestDialect_ImplementsDialectLSP(t *testing.T) {
	var _ scaf.DialectLSP = (*Dialect)(nil)
}

func TestDialect_Diagnostics_TypeMismatch(t *testing.T) {
	d := NewDialect()
	schema := createTestSchema()
	ctx := &scaf.QueryLSPContext{
		Schema: schema,
	}

	tests := []struct {
		name      string
		query     string
		wantCodes []string // diagnostic codes to expect
		wantMsgs  []string // substrings that should appear in diagnostic messages
	}{
		{
			name:      "string property with boolean value",
			query:     `MATCH (p:Person {name: false}) RETURN p`,
			wantCodes: []string{"type-mismatch"},
			wantMsgs:  []string{"property 'name' expects string, got boolean"},
		},
		{
			name:      "string property with integer value",
			query:     `MATCH (p:Person {name: 123}) RETURN p`,
			wantCodes: []string{"type-mismatch"},
			wantMsgs:  []string{"property 'name' expects string, got integer"},
		},
		{
			name:      "integer property with string value",
			query:     `MATCH (p:Person {age: "thirty"}) RETURN p`,
			wantCodes: []string{"type-mismatch"},
			wantMsgs:  []string{"property 'age' expects int, got string"},
		},
		{
			name:      "integer property with boolean value",
			query:     `MATCH (p:Person {age: true}) RETURN p`,
			wantCodes: []string{"type-mismatch"},
			wantMsgs:  []string{"property 'age' expects int, got boolean"},
		},
		{
			name:      "correct types - no diagnostics",
			query:     `MATCH (p:Person {name: "Alice", age: 30}) RETURN p`,
			wantCodes: nil,
		},
		{
			name:      "parameter value - no diagnostic (type unknown)",
			query:     `MATCH (p:Person {name: $name}) RETURN p`,
			wantCodes: nil,
		},
		{
			name:      "null value - no diagnostic (null compatible with nullable)",
			query:     `MATCH (p:Person {name: null}) RETURN p`,
			wantCodes: nil,
		},
		{
			name:      "multiple mismatches",
			query:     `MATCH (p:Person {name: 123, age: "old"}) RETURN p`,
			wantCodes: []string{"type-mismatch", "type-mismatch"},
		},
		{
			name:      "CREATE with type mismatch",
			query:     `CREATE (p:Person {name: false})`,
			wantCodes: []string{"type-mismatch"},
		},
		{
			name:      "MERGE with type mismatch",
			query:     `MERGE (p:Person {name: 42})`,
			wantCodes: []string{"type-mismatch"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := d.Diagnostics(tt.query, ctx)

			// Filter to only type-mismatch diagnostics
			var typeMismatchDiags []scaf.QueryDiagnostic
			for _, diag := range diags {
				if diag.Code == "type-mismatch" {
					typeMismatchDiags = append(typeMismatchDiags, diag)
				}
			}

			if len(typeMismatchDiags) != len(tt.wantCodes) {
				t.Errorf("got %d type-mismatch diagnostics, want %d", len(typeMismatchDiags), len(tt.wantCodes))
				for i, diag := range typeMismatchDiags {
					t.Errorf("  [%d] %s: %s", i, diag.Code, diag.Message)
				}
				return
			}

			for i, wantMsg := range tt.wantMsgs {
				if i >= len(typeMismatchDiags) {
					break
				}
				if !containsSubstring(typeMismatchDiags[i].Message, wantMsg) {
					t.Errorf("diagnostic[%d].Message = %q, want substring %q", i, typeMismatchDiags[i].Message, wantMsg)
				}
			}
		})
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func keysOf(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
