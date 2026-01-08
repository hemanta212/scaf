package cypher_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
	"github.com/rlch/scaf/dialects/cypher"
)

// typeString returns the string representation of a type, or "" if nil.
func typeString(t *scaf.Type) string {
	if t == nil {
		return ""
	}
	return t.String()
}

// testSchema provides a schema for type inference tests.
func testSchema() *analysis.TypeSchema {
	return &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"User": {
				Name: "User",
				Fields: []*analysis.Field{
					{Name: "id", Type: analysis.TypeString, Unique: true},
					{Name: "email", Type: analysis.TypeString, Unique: true},
					{Name: "name", Type: analysis.TypeString},
					{Name: "age", Type: analysis.TypeInt},
					{Name: "active", Type: analysis.TypeBool},
					{Name: "score", Type: analysis.TypeFloat64},
					{Name: "tags", Type: analysis.SliceOf(analysis.TypeString)},
				},
			},
			"Movie": {
				Name: "Movie",
				Fields: []*analysis.Field{
					{Name: "id", Type: analysis.TypeString, Unique: true},
					{Name: "title", Type: analysis.TypeString},
					{Name: "year", Type: analysis.TypeInt},
					{Name: "rating", Type: analysis.TypeFloat64},
					{Name: "genres", Type: analysis.SliceOf(analysis.TypeString)},
				},
			},
			"Order": {
				Name: "Order",
				Fields: []*analysis.Field{
					{Name: "id", Type: analysis.TypeString, Unique: true},
					{Name: "total", Type: analysis.TypeFloat64},
					{Name: "quantity", Type: analysis.TypeInt},
				},
			},
		},
	}
}

func TestTypeInference_Literals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		wantType string
	}{
		{
			name:     "integer literal",
			query:    "RETURN 42",
			wantType: "int",
		},
		{
			name:     "float literal",
			query:    "RETURN 3.14",
			wantType: "float64",
		},
		{
			name:     "string literal",
			query:    `RETURN "hello"`,
			wantType: "string",
		},
		{
			name:     "boolean true",
			query:    "RETURN true",
			wantType: "bool",
		},
		{
			name:     "boolean false",
			query:    "RETURN false",
			wantType: "bool",
		},
		{
			name:     "empty list",
			query:    "RETURN []",
			wantType: "[]",
		},
		{
			name:     "list of integers",
			query:    "RETURN [1, 2, 3]",
			wantType: "[]int",
		},
		{
			name:     "list of strings",
			query:    `RETURN ["a", "b", "c"]`,
			wantType: "[]string",
		},
		{
			name:     "map literal",
			query:    `RETURN {name: "test", age: 25}`,
			wantType: "map[string]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()
			metadata, err := analyzer.AnalyzeQuery(tt.query)
			if err != nil {
				t.Fatalf("AnalyzeQuery() error: %v", err)
			}

			if len(metadata.Returns) != 1 {
				t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
			}

			gotType := typeString(metadata.Returns[0].Type)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
		})
	}
}

func TestTypeInference_BooleanOperators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		wantType string
	}{
		{
			name:     "OR expression",
			query:    "RETURN true OR false",
			wantType: "bool",
		},
		{
			name:     "AND expression",
			query:    "RETURN true AND false",
			wantType: "bool",
		},
		{
			name:     "XOR expression",
			query:    "RETURN true XOR false",
			wantType: "bool",
		},
		{
			name:     "NOT expression",
			query:    "RETURN NOT true",
			wantType: "bool",
		},
		{
			name:     "complex boolean",
			query:    "RETURN (true AND false) OR (NOT true)",
			wantType: "bool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()
			metadata, err := analyzer.AnalyzeQuery(tt.query)
			if err != nil {
				t.Fatalf("AnalyzeQuery() error: %v", err)
			}

			if len(metadata.Returns) != 1 {
				t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
			}

			gotType := typeString(metadata.Returns[0].Type)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
		})
	}
}

func TestTypeInference_ComparisonOperators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		wantType string
	}{
		{
			name:     "equals",
			query:    "RETURN 1 = 1",
			wantType: "bool",
		},
		{
			name:     "not equals",
			query:    "RETURN 1 <> 2",
			wantType: "bool",
		},
		{
			name:     "less than",
			query:    "RETURN 1 < 2",
			wantType: "bool",
		},
		{
			name:     "greater than",
			query:    "RETURN 2 > 1",
			wantType: "bool",
		},
		{
			name:     "less than or equal",
			query:    "RETURN 1 <= 2",
			wantType: "bool",
		},
		{
			name:     "greater than or equal",
			query:    "RETURN 2 >= 1",
			wantType: "bool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()
			metadata, err := analyzer.AnalyzeQuery(tt.query)
			if err != nil {
				t.Fatalf("AnalyzeQuery() error: %v", err)
			}

			if len(metadata.Returns) != 1 {
				t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
			}

			gotType := typeString(metadata.Returns[0].Type)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
		})
	}
}

func TestTypeInference_ArithmeticOperators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		wantType string
	}{
		{
			name:     "integer addition",
			query:    "RETURN 1 + 2",
			wantType: "int",
		},
		{
			name:     "integer subtraction",
			query:    "RETURN 5 - 3",
			wantType: "int",
		},
		{
			name:     "integer multiplication",
			query:    "RETURN 2 * 3",
			wantType: "int",
		},
		{
			name:     "integer division produces float",
			query:    "RETURN 10 / 2",
			wantType: "float64",
		},
		{
			name:     "float addition",
			query:    "RETURN 1.5 + 2.5",
			wantType: "float64",
		},
		{
			name:     "mixed int and float",
			query:    "RETURN 1 + 2.5",
			wantType: "float64",
		},
		{
			name:     "power operator",
			query:    "RETURN 2 ^ 3",
			wantType: "float64",
		},
		{
			name:     "modulo",
			query:    "RETURN 10 % 3",
			wantType: "int",
		},
		{
			name:     "string concatenation",
			query:    `RETURN "hello" + " " + "world"`,
			wantType: "string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()
			metadata, err := analyzer.AnalyzeQuery(tt.query)
			if err != nil {
				t.Fatalf("AnalyzeQuery() error: %v", err)
			}

			if len(metadata.Returns) != 1 {
				t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
			}

			gotType := typeString(metadata.Returns[0].Type)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
		})
	}
}

func TestTypeInference_Functions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		wantType string
	}{
		// Aggregate functions
		{
			name:     "count(*)",
			query:    "MATCH (n) RETURN count(*)",
			wantType: "int",
		},
		{
			name:     "count(n)",
			query:    "MATCH (n) RETURN count(n)",
			wantType: "int",
		},
		{
			name:     "sum",
			query:    "MATCH (n) RETURN sum(n.value)",
			wantType: "float64",
		},
		{
			name:     "avg",
			query:    "MATCH (n) RETURN avg(n.value)",
			wantType: "float64",
		},

		// Type conversion
		{
			name:     "toString",
			query:    "RETURN toString(123)",
			wantType: "string",
		},
		{
			name:     "toInteger",
			query:    `RETURN toInteger("123")`,
			wantType: "int",
		},
		{
			name:     "toFloat",
			query:    `RETURN toFloat("3.14")`,
			wantType: "float64",
		},
		{
			name:     "toBoolean",
			query:    `RETURN toBoolean("true")`,
			wantType: "bool",
		},

		// Scalar functions
		{
			name:     "size",
			query:    `RETURN size("hello")`,
			wantType: "int",
		},
		{
			name:     "length",
			query:    "MATCH p = (a)-[*]->(b) RETURN length(p)",
			wantType: "int",
		},
		{
			name:     "type",
			query:    "MATCH ()-[r]->() RETURN type(r)",
			wantType: "string",
		},
		{
			name:     "id",
			query:    "MATCH (n) RETURN id(n)",
			wantType: "int",
		},
		{
			name:     "labels",
			query:    "MATCH (n) RETURN labels(n)",
			wantType: "[]string",
		},
		{
			name:     "keys",
			query:    "MATCH (n) RETURN keys(n)",
			wantType: "[]string",
		},
		// Note: exists(expr) is deprecated in Neo4j 4+, use IS NOT NULL instead
		// The EXISTS {} subquery form is tested in TestTypeInference_Predicates

		// Math functions
		{
			name:     "abs",
			query:    "RETURN abs(-5)",
			wantType: "float64",
		},
		{
			name:     "ceil",
			query:    "RETURN ceil(3.14)",
			wantType: "float64",
		},
		{
			name:     "floor",
			query:    "RETURN floor(3.99)",
			wantType: "float64",
		},
		{
			name:     "round",
			query:    "RETURN round(3.5)",
			wantType: "float64",
		},
		{
			name:     "sqrt",
			query:    "RETURN sqrt(16)",
			wantType: "float64",
		},
		{
			name:     "rand",
			query:    "RETURN rand()",
			wantType: "float64",
		},

		// String functions
		{
			name:     "toLower",
			query:    `RETURN toLower("HELLO")`,
			wantType: "string",
		},
		{
			name:     "toUpper",
			query:    `RETURN toUpper("hello")`,
			wantType: "string",
		},
		{
			name:     "trim",
			query:    `RETURN trim("  hello  ")`,
			wantType: "string",
		},
		{
			name:     "replace",
			query:    `RETURN replace("hello", "l", "x")`,
			wantType: "string",
		},
		{
			name:     "substring",
			query:    `RETURN substring("hello", 0, 3)`,
			wantType: "string",
		},
		{
			name:     "split",
			query:    `RETURN split("a,b,c", ",")`,
			wantType: "[]string",
		},

		// List functions
		{
			name:     "range",
			query:    "RETURN range(1, 10)",
			wantType: "[]int",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()
			metadata, err := analyzer.AnalyzeQuery(tt.query)
			if err != nil {
				t.Fatalf("AnalyzeQuery() error: %v", err)
			}

			if len(metadata.Returns) != 1 {
				t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
			}

			gotType := typeString(metadata.Returns[0].Type)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
		})
	}
}

func TestTypeInference_FunctionsWithTypeInference(t *testing.T) {
	t.Parallel()

	schema := testSchema()

	tests := []struct {
		name     string
		query    string
		wantType string
	}{
		{
			name:     "collect preserves element type",
			query:    "MATCH (u:User) RETURN collect(u.name)",
			wantType: "[]string",
		},
		{
			name:     "collect on integer",
			query:    "MATCH (u:User) RETURN collect(u.age)",
			wantType: "[]int",
		},
		{
			name:     "head of string list",
			query:    "MATCH (u:User) RETURN head(u.tags)",
			wantType: "string",
		},
		{
			name:     "last of string list",
			query:    "MATCH (m:Movie) RETURN last(m.genres)",
			wantType: "string",
		},
		{
			name:     "tail preserves list type",
			query:    "MATCH (u:User) RETURN tail(u.tags)",
			wantType: "[]string",
		},
		{
			name:     "min preserves type",
			query:    "MATCH (u:User) RETURN min(u.age)",
			wantType: "int",
		},
		{
			name:     "max preserves type",
			query:    "MATCH (m:Movie) RETURN max(m.rating)",
			wantType: "float64",
		},
		{
			name:     "coalesce with string",
			query:    "MATCH (u:User) RETURN coalesce(u.name, 'Unknown')",
			wantType: "string",
		},
		{
			name:     "properties returns map",
			query:    "MATCH (u:User) RETURN properties(u)",
			wantType: "map[string]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()
			metadata, err := analyzer.AnalyzeQueryWithSchema(tt.query, schema)
			if err != nil {
				t.Fatalf("AnalyzeQueryWithSchema() error: %v", err)
			}

			if len(metadata.Returns) != 1 {
				t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
			}

			gotType := typeString(metadata.Returns[0].Type)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
		})
	}
}

func TestTypeInference_PropertyAccess(t *testing.T) {
	t.Parallel()

	schema := testSchema()

	tests := []struct {
		name     string
		query    string
		wantType string
	}{
		{
			name:     "string property",
			query:    "MATCH (u:User) RETURN u.name",
			wantType: "string",
		},
		{
			name:     "int property",
			query:    "MATCH (u:User) RETURN u.age",
			wantType: "int",
		},
		{
			name:     "bool property",
			query:    "MATCH (u:User) RETURN u.active",
			wantType: "bool",
		},
		{
			name:     "float64 property",
			query:    "MATCH (u:User) RETURN u.score",
			wantType: "float64",
		},
		{
			name:     "slice property",
			query:    "MATCH (u:User) RETURN u.tags",
			wantType: "[]string",
		},
		{
			name:     "whole node",
			query:    "MATCH (u:User) RETURN u",
			wantType: "*User",
		},
		{
			name:     "multiple models",
			query:    "MATCH (u:User)-[:LIKES]->(m:Movie) RETURN u.name, m.title",
			wantType: "string", // checking first return
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()
			metadata, err := analyzer.AnalyzeQueryWithSchema(tt.query, schema)
			if err != nil {
				t.Fatalf("AnalyzeQueryWithSchema() error: %v", err)
			}

			if len(metadata.Returns) < 1 {
				t.Fatalf("expected at least 1 return, got %d", len(metadata.Returns))
			}

			gotType := typeString(metadata.Returns[0].Type)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
		})
	}
}

func TestTypeInference_StringOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		wantType string
	}{
		{
			name:     "STARTS WITH",
			query:    `RETURN "hello" STARTS WITH "he"`,
			wantType: "bool",
		},
		{
			name:     "ENDS WITH",
			query:    `RETURN "hello" ENDS WITH "lo"`,
			wantType: "bool",
		},
		{
			name:     "CONTAINS",
			query:    `RETURN "hello" CONTAINS "ell"`,
			wantType: "bool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()
			metadata, err := analyzer.AnalyzeQuery(tt.query)
			if err != nil {
				t.Fatalf("AnalyzeQuery() error: %v", err)
			}

			if len(metadata.Returns) != 1 {
				t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
			}

			gotType := typeString(metadata.Returns[0].Type)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
		})
	}
}

func TestTypeInference_NullOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		wantType string
	}{
		{
			name:     "IS NULL",
			query:    "MATCH (n) RETURN n.name IS NULL",
			wantType: "bool",
		},
		{
			name:     "IS NOT NULL",
			query:    "MATCH (n) RETURN n.name IS NOT NULL",
			wantType: "bool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()
			metadata, err := analyzer.AnalyzeQuery(tt.query)
			if err != nil {
				t.Fatalf("AnalyzeQuery() error: %v", err)
			}

			if len(metadata.Returns) != 1 {
				t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
			}

			gotType := typeString(metadata.Returns[0].Type)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
		})
	}
}

func TestTypeInference_ListOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		wantType string
	}{
		{
			name:     "IN operator",
			query:    "RETURN 1 IN [1, 2, 3]",
			wantType: "bool",
		},
		{
			name:     "list indexing",
			query:    "RETURN [1, 2, 3][0]",
			wantType: "int",
		},
		{
			name:     "string list indexing",
			query:    `RETURN ["a", "b", "c"][1]`,
			wantType: "string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()
			metadata, err := analyzer.AnalyzeQuery(tt.query)
			if err != nil {
				t.Fatalf("AnalyzeQuery() error: %v", err)
			}

			if len(metadata.Returns) != 1 {
				t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
			}

			gotType := typeString(metadata.Returns[0].Type)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
		})
	}
}

func TestTypeInference_Predicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		wantType string
	}{
		{
			name:     "ALL predicate",
			query:    "RETURN all(x IN [1, 2, 3] WHERE x > 0)",
			wantType: "bool",
		},
		{
			name:     "ANY predicate",
			query:    "RETURN any(x IN [1, 2, 3] WHERE x > 2)",
			wantType: "bool",
		},
		{
			name:     "NONE predicate",
			query:    "RETURN none(x IN [1, 2, 3] WHERE x < 0)",
			wantType: "bool",
		},
		{
			name:     "SINGLE predicate",
			query:    "RETURN single(x IN [1, 2, 3] WHERE x = 2)",
			wantType: "bool",
		},
		{
			name:     "EXISTS subquery",
			query:    "RETURN exists { MATCH (n) WHERE n.name = 'test' }",
			wantType: "bool",
		},
		// Note: exists(property) as a function is deprecated in Neo4j 5+.
		// Use IS NOT NULL or EXISTS { ... } subquery instead.
		// The grammar only supports EXISTS { ... } subquery form.
		{
			name:     "isEmpty function",
			query:    "RETURN isEmpty([])",
			wantType: "bool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()
			metadata, err := analyzer.AnalyzeQuery(tt.query)
			if err != nil {
				t.Fatalf("AnalyzeQuery() error: %v", err)
			}

			if len(metadata.Returns) != 1 {
				t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
			}

			gotType := typeString(metadata.Returns[0].Type)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
		})
	}
}

func TestTypeInference_CaseExpression(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		wantType string
	}{
		{
			name:     "CASE with string result",
			query:    `RETURN CASE WHEN true THEN "yes" ELSE "no" END`,
			wantType: "string",
		},
		{
			name:     "CASE with int result",
			query:    "RETURN CASE WHEN true THEN 1 ELSE 0 END",
			wantType: "int",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()
			metadata, err := analyzer.AnalyzeQuery(tt.query)
			if err != nil {
				t.Fatalf("AnalyzeQuery() error: %v", err)
			}

			if len(metadata.Returns) != 1 {
				t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
			}

			gotType := typeString(metadata.Returns[0].Type)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
		})
	}
}

func TestTypeInference_ComplexExpressions(t *testing.T) {
	t.Parallel()

	schema := testSchema()

	tests := []struct {
		name     string
		query    string
		wantType string
	}{
		{
			name:     "arithmetic on property",
			query:    "MATCH (u:User) RETURN u.age + 10",
			wantType: "int",
		},
		{
			name:     "comparison on property",
			query:    "MATCH (u:User) RETURN u.age > 18",
			wantType: "bool",
		},
		{
			name:     "function on property",
			query:    "MATCH (u:User) RETURN toUpper(u.name)",
			wantType: "string",
		},
		{
			name:     "nested function calls",
			query:    "MATCH (u:User) RETURN size(collect(u.name))",
			wantType: "int",
		},
		{
			name:     "parenthesized expression",
			query:    "RETURN (1 + 2) * 3",
			wantType: "int",
		},
		{
			name:     "chained comparisons produce bool",
			query:    "MATCH (u:User) RETURN u.age > 18 AND u.active",
			wantType: "bool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()
			metadata, err := analyzer.AnalyzeQueryWithSchema(tt.query, schema)
			if err != nil {
				t.Fatalf("AnalyzeQueryWithSchema() error: %v", err)
			}

			if len(metadata.Returns) != 1 {
				t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
			}

			gotType := typeString(metadata.Returns[0].Type)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
		})
	}
}

func TestTypeInference_NoSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		wantType string
	}{
		{
			name:     "property access without schema returns empty",
			query:    "MATCH (u:User) RETURN u.name",
			wantType: "",
		},
		{
			name:     "function still works without schema",
			query:    "MATCH (u:User) RETURN count(u)",
			wantType: "int",
		},
		{
			name:     "literal works without schema",
			query:    "RETURN 42",
			wantType: "int",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()
			metadata, err := analyzer.AnalyzeQuery(tt.query)
			if err != nil {
				t.Fatalf("AnalyzeQuery() error: %v", err)
			}

			if len(metadata.Returns) != 1 {
				t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
			}

			gotType := typeString(metadata.Returns[0].Type)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
		})
	}
}

func TestTypeInference_MultipleReturns(t *testing.T) {
	t.Parallel()

	schema := testSchema()

	analyzer := cypher.NewAnalyzer()
	metadata, err := analyzer.AnalyzeQueryWithSchema(
		"MATCH (u:User) RETURN u.name, u.age, count(*) AS total, u.active",
		schema,
	)
	if err != nil {
		t.Fatalf("AnalyzeQueryWithSchema() error: %v", err)
	}

	if len(metadata.Returns) != 4 {
		t.Fatalf("expected 4 returns, got %d", len(metadata.Returns))
	}

	expectedTypes := []string{"string", "int", "int", "bool"}
	for i, expected := range expectedTypes {
		gotType := typeString(metadata.Returns[i].Type)
		if gotType != expected {
			t.Errorf("return[%d].Type = %q, want %q", i, gotType, expected)
		}
	}
}

func TestTypeInference_SliceTypes(t *testing.T) {
	t.Parallel()

	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"User": {
				Name: "User",
				Fields: []*analysis.Field{
					{Name: "name", Type: analysis.TypeString},
					{Name: "tags", Type: analysis.SliceOf(analysis.TypeString)},
					{Name: "scores", Type: analysis.SliceOf(analysis.TypeInt)},
				},
			},
		},
	}

	tests := []struct {
		name     string
		query    string
		wantType string
	}{
		// Basic slice property access
		{
			name:     "schema slice property - string",
			query:    "MATCH (u:User) RETURN u.tags",
			wantType: "[]string",
		},
		{
			name:     "schema slice property - int",
			query:    "MATCH (u:User) RETURN u.scores",
			wantType: "[]int",
		},

		// Indexing into schema slices
		{
			name:     "indexing schema slice returns element",
			query:    "MATCH (u:User) RETURN u.tags[0]",
			wantType: "string",
		},
		{
			name:     "indexing int slice returns element",
			query:    "MATCH (u:User) RETURN u.scores[0]",
			wantType: "int",
		},

		// Functions on schema slices
		{
			name:     "head of schema slice",
			query:    "MATCH (u:User) RETURN head(u.tags)",
			wantType: "string",
		},
		{
			name:     "last of schema slice",
			query:    "MATCH (u:User) RETURN last(u.scores)",
			wantType: "int",
		},
		{
			name:     "tail preserves schema slice type",
			query:    "MATCH (u:User) RETURN tail(u.tags)",
			wantType: "[]string",
		},
		{
			name:     "size of schema slice",
			query:    "MATCH (u:User) RETURN size(u.tags)",
			wantType: "int",
		},

		// Collect creates nested slices
		{
			name:     "collect on slice property creates nested slice",
			query:    "MATCH (u:User) RETURN collect(u.tags)",
			wantType: "[][]string",
		},
		{
			name:     "collect on primitive creates slice",
			query:    "MATCH (u:User) RETURN collect(u.name)",
			wantType: "[]string",
		},

		// List comprehensions
		{
			name:     "list comprehension filter preserves type",
			query:    "MATCH (u:User) RETURN [x IN u.tags WHERE size(x) > 3]",
			wantType: "[]string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()
			metadata, err := analyzer.AnalyzeQueryWithSchema(tt.query, schema)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if len(metadata.Returns) != 1 {
				t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
			}
			gotType := typeString(metadata.Returns[0].Type)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
		})
	}
}

// TestTypeInference_SliceTypesFromYAML tests that slice types loaded from YAML schema files
// are correctly used for type inference.
func TestTypeInference_SliceTypesFromYAML(t *testing.T) {
	t.Parallel()

	// Create a YAML schema file
	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "schema.yaml")

	schemaYAML := `
models:
  Person:
    fields:
      name:
        type: string
      tags:
        type: "[]string"
      scores:
        type: "[]int"
      matrix:
        type: "[][]float64"
`

	err := os.WriteFile(schemaPath, []byte(schemaYAML), 0o644)
	if err != nil {
		t.Fatalf("failed to write schema file: %v", err)
	}

	// Load schema from YAML
	schema, err := analysis.LoadSchema(schemaPath, "")
	if err != nil {
		t.Fatalf("failed to load schema: %v", err)
	}

	tests := []struct {
		name     string
		query    string
		wantType string
	}{
		{
			name:     "YAML schema slice property",
			query:    "MATCH (p:Person) RETURN p.tags",
			wantType: "[]string",
		},
		{
			name:     "YAML schema slice indexing",
			query:    "MATCH (p:Person) RETURN p.tags[0]",
			wantType: "string",
		},
		{
			name:     "YAML schema int slice",
			query:    "MATCH (p:Person) RETURN p.scores",
			wantType: "[]int",
		},
		{
			name:     "YAML schema nested slice",
			query:    "MATCH (p:Person) RETURN p.matrix",
			wantType: "[][]float64",
		},
		{
			name:     "YAML schema nested slice indexing",
			query:    "MATCH (p:Person) RETURN p.matrix[0]",
			wantType: "[]float64",
		},
		{
			name:     "YAML schema head of slice",
			query:    "MATCH (p:Person) RETURN head(p.tags)",
			wantType: "string",
		},
		{
			name:     "YAML schema collect creates nested",
			query:    "MATCH (p:Person) RETURN collect(p.tags)",
			wantType: "[][]string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()
			metadata, err := analyzer.AnalyzeQueryWithSchema(tt.query, schema)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if len(metadata.Returns) != 1 {
				t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
			}
			gotType := typeString(metadata.Returns[0].Type)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
		})
	}
}

// TestTypeInference_ListComprehensionVariableBinding tests that list comprehension
// variables are correctly typed based on the source list element type.
func TestTypeInference_ListComprehensionVariableBinding(t *testing.T) {
	t.Parallel()

	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"User": {
				Name: "User",
				Fields: []*analysis.Field{
					{Name: "name", Type: analysis.TypeString},
					{Name: "tags", Type: analysis.SliceOf(analysis.TypeString)},
					{Name: "scores", Type: analysis.SliceOf(analysis.TypeInt)},
					{Name: "ratings", Type: analysis.SliceOf(analysis.TypeFloat64)},
				},
			},
			"Movie": {
				Name: "Movie",
				Fields: []*analysis.Field{
					{Name: "title", Type: analysis.TypeString},
					{Name: "genres", Type: analysis.SliceOf(analysis.TypeString)},
				},
			},
		},
	}

	tests := []struct {
		name     string
		query    string
		wantType string
	}{
		// Basic list comprehension with mapping - variable x should be typed from source list
		{
			name:     "list comprehension with toUpper on string element",
			query:    "MATCH (u:User) RETURN [x IN u.tags | toUpper(x)]",
			wantType: "[]string",
		},
		{
			name:     "list comprehension returns variable directly",
			query:    "MATCH (u:User) RETURN [x IN u.tags | x]",
			wantType: "[]string",
		},
		{
			name:     "list comprehension with int element",
			query:    "MATCH (u:User) RETURN [s IN u.scores | s * 2]",
			wantType: "[]int",
		},
		{
			name:     "list comprehension with float element",
			query:    "MATCH (u:User) RETURN [r IN u.ratings | r + 1.0]",
			wantType: "[]float64",
		},
		// Type conversion in mapping expression
		{
			name:     "list comprehension with toString conversion",
			query:    "MATCH (u:User) RETURN [s IN u.scores | toString(s)]",
			wantType: "[]string",
		},
		{
			name:     "list comprehension with toFloat conversion",
			query:    "MATCH (u:User) RETURN [s IN u.scores | toFloat(s)]",
			wantType: "[]float64",
		},
		// Filtering only (no mapping)
		{
			name:     "list comprehension filter only",
			query:    "MATCH (u:User) RETURN [x IN u.tags WHERE size(x) > 3]",
			wantType: "[]string",
		},
		{
			name:     "list comprehension filter on int slice",
			query:    "MATCH (u:User) RETURN [s IN u.scores WHERE s > 50]",
			wantType: "[]int",
		},
		// Nested list comprehensions
		{
			name:     "size of list comprehension variable",
			query:    "MATCH (u:User) RETURN [x IN u.tags | size(x)]",
			wantType: "[]int",
		},
		// List literal as source
		{
			name:     "list comprehension on literal string list",
			query:    `RETURN [x IN ["a", "b", "c"] | toUpper(x)]`,
			wantType: "[]string",
		},
		{
			name:     "list comprehension on literal int list",
			query:    "RETURN [n IN [1, 2, 3] | n * 2]",
			wantType: "[]int",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()
			metadata, err := analyzer.AnalyzeQueryWithSchema(tt.query, schema)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if len(metadata.Returns) != 1 {
				t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
			}
			gotType := typeString(metadata.Returns[0].Type)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
		})
	}
}

// TestTypeInference_APOCFunctions tests type inference for APOC functions.
func TestTypeInference_APOCFunctions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		wantType string
	}{
		// APOC Text functions
		{
			name:     "apoc.text.join",
			query:    `RETURN apoc.text.join(["a", "b", "c"], ",")`,
			wantType: "string",
		},
		{
			name:     "apoc.text.replace",
			query:    `RETURN apoc.text.replace("hello", "l", "x")`,
			wantType: "string",
		},
		{
			name:     "apoc.text.split",
			query:    `RETURN apoc.text.split("a,b,c", ",")`,
			wantType: "[]string",
		},
		{
			name:     "apoc.text.capitalize",
			query:    `RETURN apoc.text.capitalize("hello")`,
			wantType: "string",
		},
		{
			name:     "apoc.text.camelCase",
			query:    `RETURN apoc.text.camelCase("hello_world")`,
			wantType: "string",
		},
		{
			name:     "apoc.text.snakeCase",
			query:    `RETURN apoc.text.snakeCase("helloWorld")`,
			wantType: "string",
		},
		{
			name:     "apoc.text.distance (Levenshtein)",
			query:    `RETURN apoc.text.distance("hello", "hallo")`,
			wantType: "int",
		},
		{
			name:     "apoc.text.fuzzyMatch",
			query:    `RETURN apoc.text.fuzzyMatch("hello", "hallo")`,
			wantType: "bool",
		},
		{
			name:     "apoc.text.urlencode",
			query:    `RETURN apoc.text.urlencode("hello world")`,
			wantType: "string",
		},
		{
			name:     "apoc.text.bytes",
			query:    `RETURN apoc.text.bytes("hello")`,
			wantType: "[]int",
		},
		{
			name:     "apoc.text.regexGroups",
			query:    `RETURN apoc.text.regexGroups("abc123", "([a-z]+)([0-9]+)")`,
			wantType: "[][]string",
		},

		// APOC Collection functions
		// Note: Queries using list/map literals as first arg have grammar issues,
		// so we use UNWIND/WITH to pass variables instead
		{
			name:     "apoc.coll.sum",
			query:    "UNWIND [[1, 2, 3]] AS nums RETURN apoc.coll.sum(nums)",
			wantType: "float64",
		},
		{
			name:     "apoc.coll.avg",
			query:    "UNWIND [[1, 2, 3]] AS nums RETURN apoc.coll.avg(nums)",
			wantType: "float64",
		},
		{
			name:     "apoc.coll.contains",
			query:    `UNWIND [["a", "b"]] AS items RETURN apoc.coll.contains(items, "a")`,
			wantType: "bool",
		},
		{
			name:     "apoc.coll.containsAll",
			query:    `UNWIND [["a", "b", "c"]] AS items RETURN apoc.coll.containsAll(items, items)`,
			wantType: "bool",
		},
		{
			name:     "apoc.coll.different",
			query:    `UNWIND [["a", "b", "c"]] AS items RETURN apoc.coll.different(items)`,
			wantType: "bool",
		},
		{
			name:     "apoc.coll.occurrences",
			query:    `UNWIND [["a", "b", "a"]] AS items RETURN apoc.coll.occurrences(items, "a")`,
			wantType: "int",
		},
		{
			name:     "apoc.coll.indexOf",
			query:    `UNWIND [["a", "b", "c"]] AS items RETURN apoc.coll.indexOf(items, "b")`,
			wantType: "int",
		},
		{
			name:     "apoc.coll.isEmpty",
			query:    "UNWIND [[]] AS items RETURN apoc.coll.isEmpty(items)",
			wantType: "bool",
		},
		{
			name:     "apoc.coll.sumLongs",
			query:    "UNWIND [[1, 2, 3]] AS nums RETURN apoc.coll.sumLongs(nums)",
			wantType: "int64",
		},

		// APOC Map functions
		// Note: Map literals as first arg have grammar issues, using properties() instead
		{
			name:     "apoc.map.merge",
			query:    `MATCH (n) RETURN apoc.map.merge(properties(n), properties(n))`,
			wantType: "map[string]",
		},
		{
			name:     "apoc.map.flatten",
			query:    `RETURN apoc.map.flatten({a: {b: 1}})`,
			wantType: "map[string]",
		},
		{
			name:     "apoc.map.fromLists",
			query:    `RETURN apoc.map.fromLists(["a", "b"], [1, 2])`,
			wantType: "map[string]",
		},
		{
			name:     "apoc.map.removeKey",
			query:    `RETURN apoc.map.removeKey({a: 1, b: 2}, "a")`,
			wantType: "map[string]",
		},

		// APOC Conversion functions
		{
			name:     "apoc.convert.toJson",
			query:    `RETURN apoc.convert.toJson({name: "test"})`,
			wantType: "string",
		},
		{
			name:     "apoc.convert.fromJsonMap",
			query:    `RETURN apoc.convert.fromJsonMap('{"name": "test"}')`,
			wantType: "map[string]",
		},
		{
			name:     "apoc.convert.toBoolean",
			query:    `RETURN apoc.convert.toBoolean("true")`,
			wantType: "bool",
		},
		{
			name:     "apoc.convert.toFloat",
			query:    `RETURN apoc.convert.toFloat("3.14")`,
			wantType: "float64",
		},
		{
			name:     "apoc.convert.toInteger",
			query:    `RETURN apoc.convert.toInteger("42")`,
			wantType: "int",
		},
		{
			name:     "apoc.convert.toString",
			query:    "RETURN apoc.convert.toString(42)",
			wantType: "string",
		},

		// APOC Date functions
		{
			name:     "apoc.date.format",
			query:    "RETURN apoc.date.format(timestamp())",
			wantType: "string",
		},
		{
			name:     "apoc.date.parse",
			query:    `RETURN apoc.date.parse("2023-01-01", "ms", "yyyy-MM-dd")`,
			wantType: "int",
		},
		{
			name:     "apoc.date.currentTimestamp",
			query:    "RETURN apoc.date.currentTimestamp()",
			wantType: "int",
		},
		{
			name:     "apoc.date.toISO8601",
			query:    "RETURN apoc.date.toISO8601(timestamp())",
			wantType: "string",
		},
		{
			name:     "apoc.temporal.format",
			query:    "RETURN apoc.temporal.format(date(), 'yyyy-MM-dd')",
			wantType: "string",
		},

		// APOC Hashing functions
		{
			name:     "apoc.util.md5",
			query:    `RETURN apoc.util.md5(["hello"])`,
			wantType: "string",
		},
		{
			name:     "apoc.util.sha256",
			query:    `RETURN apoc.util.sha256(["hello"])`,
			wantType: "string",
		},
		{
			name:     "apoc.hashing.fingerprint",
			query:    `RETURN apoc.hashing.fingerprint({name: "test"})`,
			wantType: "string",
		},

		// APOC Node functions
		{
			name:     "apoc.node.degree",
			query:    "MATCH (n) RETURN apoc.node.degree(n)",
			wantType: "int",
		},
		{
			name:     "apoc.node.labels",
			query:    "MATCH (n) RETURN apoc.node.labels(n)",
			wantType: "[]string",
		},
		{
			name:     "apoc.nodes.connected",
			query:    "MATCH (a), (b) RETURN apoc.nodes.connected(a, b)",
			wantType: "bool",
		},

		// APOC Math functions
		{
			name:     "apoc.math.maxLong",
			query:    "RETURN apoc.math.maxLong()",
			wantType: "int64",
		},
		{
			name:     "apoc.math.maxDouble",
			query:    "RETURN apoc.math.maxDouble()",
			wantType: "float64",
		},

		// APOC Number functions
		{
			name:     "apoc.number.format",
			query:    "RETURN apoc.number.format(1234.567)",
			wantType: "string",
		},
		{
			name:     "apoc.number.parseFloat",
			query:    `RETURN apoc.number.parseFloat("3.14")`,
			wantType: "float64",
		},
		{
			name:     "apoc.number.parseInt",
			query:    `RETURN apoc.number.parseInt("42")`,
			wantType: "int",
		},

		// APOC Scoring functions
		{
			name:     "apoc.scoring.existence",
			query:    "RETURN apoc.scoring.existence(10, true)",
			wantType: "float64",
		},

		// APOC Bitwise functions
		{
			name:     "apoc.bitwise.op",
			query:    "RETURN apoc.bitwise.op(5, '&', 3)",
			wantType: "int64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()
			metadata, err := analyzer.AnalyzeQuery(tt.query)
			if err != nil {
				t.Fatalf("AnalyzeQuery() error: %v", err)
			}

			if len(metadata.Returns) != 1 {
				t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
			}

			gotType := typeString(metadata.Returns[0].Type)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
		})
	}
}

// TestTypeInference_PatternComprehensionVariableBinding tests that pattern comprehension
// variables are correctly typed based on the node labels in the pattern.
func TestTypeInference_PatternComprehensionVariableBinding(t *testing.T) {
	t.Parallel()

	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"Person": {
				Name: "Person",
				Fields: []*analysis.Field{
					{Name: "name", Type: analysis.TypeString},
					{Name: "age", Type: analysis.TypeInt},
				},
			},
			"Movie": {
				Name: "Movie",
				Fields: []*analysis.Field{
					{Name: "title", Type: analysis.TypeString},
					{Name: "year", Type: analysis.TypeInt},
				},
			},
		},
	}

	tests := []struct {
		name     string
		query    string
		wantType string
	}{
		// Pattern comprehension with typed nodes
		{
			name:     "pattern comprehension returns property of matched node",
			query:    "MATCH (p:Person) RETURN [(p)-[:ACTED_IN]->(m:Movie) | m.title]",
			wantType: "[]string",
		},
		{
			name:     "pattern comprehension returns int property",
			query:    "MATCH (p:Person) RETURN [(p)-[:ACTED_IN]->(m:Movie) | m.year]",
			wantType: "[]int",
		},
		{
			name:     "pattern comprehension with source node property",
			query:    "MATCH (m:Movie) RETURN [(p:Person)-[:ACTED_IN]->(m) | p.name]",
			wantType: "[]string",
		},
		{
			name:     "pattern comprehension with source node int property",
			query:    "MATCH (m:Movie) RETURN [(p:Person)-[:ACTED_IN]->(m) | p.age]",
			wantType: "[]int",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()
			metadata, err := analyzer.AnalyzeQueryWithSchema(tt.query, schema)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if len(metadata.Returns) != 1 {
				t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
			}
			gotType := typeString(metadata.Returns[0].Type)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
		})
	}
}

// TestTypeInference_RequiredField tests that ReturnInfo.Required is correctly set
// based on field.Required from the schema. The type inference layer passes Required
// through ReturnInfo, and signature.go uses it to decide pointer wrapping.
func TestTypeInference_RequiredField(t *testing.T) {
	t.Parallel()

	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"User": {
				Name: "User",
				Fields: []*analysis.Field{
					// Required fields
					{Name: "id", Type: analysis.TypeString, Required: true},
					{Name: "age", Type: analysis.TypeInt, Required: true},
					// Nullable fields (Required: false is default)
					{Name: "bio", Type: analysis.TypeString, Required: false},
					{Name: "score", Type: analysis.TypeFloat64, Required: false},
					// Reference types (nil-able regardless of Required)
					{Name: "tags", Type: analysis.SliceOf(analysis.TypeString), Required: false},
				},
			},
		},
	}

	tests := []struct {
		name         string
		query        string
		wantType     string
		wantRequired bool
	}{
		{
			name:         "required string field",
			query:        "MATCH (u:User) RETURN u.id",
			wantType:     "string",
			wantRequired: true,
		},
		{
			name:         "required int field",
			query:        "MATCH (u:User) RETURN u.age",
			wantType:     "int",
			wantRequired: true,
		},
		{
			name:         "nullable string field",
			query:        "MATCH (u:User) RETURN u.bio",
			wantType:     "string", // Type stays string, Required=false signals nullable
			wantRequired: false,
		},
		{
			name:         "nullable float64 field",
			query:        "MATCH (u:User) RETURN u.score",
			wantType:     "float64",
			wantRequired: false,
		},
		{
			name:         "nullable slice field",
			query:        "MATCH (u:User) RETURN u.tags",
			wantType:     "[]string",
			wantRequired: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := cypher.NewAnalyzer()
			metadata, err := analyzer.AnalyzeQueryWithSchema(tt.query, schema)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if len(metadata.Returns) != 1 {
				t.Fatalf("expected 1 return, got %d", len(metadata.Returns))
			}

			ret := metadata.Returns[0]
			gotType := typeString(ret.Type)
			if gotType != tt.wantType {
				t.Errorf("type = %q, want %q", gotType, tt.wantType)
			}
			if ret.Required != tt.wantRequired {
				t.Errorf("Required = %v, want %v", ret.Required, tt.wantRequired)
			}
		})
	}
}
