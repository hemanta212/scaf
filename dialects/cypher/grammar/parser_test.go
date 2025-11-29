package cyphergrammar_test

import (
	"testing"

	"github.com/rlch/scaf/dialects/cypher/grammar"
)

func TestParse_BasicQueries(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"simple return", "RETURN 42"},
		{"return string", `RETURN "hello"`},
		{"return float", "RETURN 3.14"},
		{"return bool", "RETURN true"},
		{"return list", "RETURN [1, 2, 3]"},
		{"return map", `RETURN {name: "test", age: 25}`},
		{"simple match", "MATCH (n) RETURN n"},
		{"match with label", "MATCH (u:User) RETURN u"},
		{"match with properties", `MATCH (u:User {name: "Alice"}) RETURN u`},
		{"match with parameter", "MATCH (u:User {id: $userId}) RETURN u"},
		{"property access", "MATCH (u:User) RETURN u.name"},
		{"function call", "MATCH (u:User) RETURN count(u)"},
		{"namespaced function", `RETURN apoc.text.join(["a", "b"], ",")`},
		{"list comprehension", "MATCH (u:User) RETURN [x IN u.tags | toUpper(x)]"},
		{"list comprehension filter", "MATCH (u:User) RETURN [x IN u.tags WHERE size(x) > 3]"},
		{"arithmetic", "RETURN 1 + 2 * 3"},
		{"comparison", "RETURN 1 < 2"},
		{"boolean logic", "RETURN true AND false OR NOT true"},
		{"case expression", "RETURN CASE WHEN x > 0 THEN 'positive' ELSE 'non-positive' END"},
		{"order by", "MATCH (u:User) RETURN u.name ORDER BY u.name"},
		{"skip limit", "MATCH (u:User) RETURN u SKIP 10 LIMIT 5"},
		{"with clause", "MATCH (u:User) WITH u.name AS name RETURN name"},
		{"create", "CREATE (n:Person {name: 'Alice'})"},
		{"relationship pattern", "MATCH (a)-[:KNOWS]->(b) RETURN a, b"},
		{"optional match", "OPTIONAL MATCH (u:User) RETURN u"},
		{"unwind", "UNWIND [1, 2, 3] AS x RETURN x"},
		{"exists subquery", "MATCH (u:User) WHERE EXISTS { MATCH (u)-[:KNOWS]->() } RETURN u"},
		{"is null", "MATCH (u:User) WHERE u.email IS NULL RETURN u"},
		{"is not null", "MATCH (u:User) WHERE u.email IS NOT NULL RETURN u"},
		{"in list", "RETURN 1 IN [1, 2, 3]"},
		{"starts with", `RETURN "hello" STARTS WITH "he"`},
		{"contains", `RETURN "hello" CONTAINS "ll"`},
		{"return distinct", "MATCH (u:User) RETURN DISTINCT u.name"},
		{"count star", "MATCH (u:User) RETURN count(*)"},
		{"set property", "MATCH (u:User) SET u.name = $name RETURN u"},
		{"set variable", "MATCH (u:User) SET u = $props RETURN u"},
		{"set add assign", "MATCH (u:User) SET u += $props RETURN u"},
		{"set label", "MATCH (u) SET u:Admin RETURN u"},
		{"merge with on create", "MERGE (u:User {id: $id}) ON CREATE SET u.name = $name RETURN u"},
		{"merge with on match", "MERGE (u:User {id: $id}) ON MATCH SET u.updated = $updated RETURN u"},
		{"delete", "MATCH (u:User) DELETE u"},
		{"detach delete", "MATCH (u:User) DETACH DELETE u"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ast, err := cyphergrammar.Parse(tt.query)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.query, err)
			}
			if ast == nil {
				t.Fatalf("Parse(%q) returned nil AST", tt.query)
			}
		})
	}
}

func TestParse_ListLiteralVsComprehension(t *testing.T) {
	// This is the key test - list literals should parse correctly even as
	// the first argument to namespaced functions
	tests := []struct {
		name  string
		query string
	}{
		{"list literal in function", `RETURN apoc.coll.contains([1, 2, 3], 1)`},
		{"list literal first arg", `RETURN apoc.coll.sum([1, 2, 3])`},
		{"nested list literal", `RETURN [[1, 2], [3, 4]]`},
		{"empty list", `RETURN []`},
		{"list with expressions", `RETURN [1 + 2, 3 * 4]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ast, err := cyphergrammar.Parse(tt.query)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.query, err)
			}
			if ast == nil {
				t.Fatalf("Parse(%q) returned nil AST", tt.query)
			}
		})
	}
}

func TestParse_ListComprehension(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"basic comprehension", "RETURN [x IN [1, 2, 3] | x * 2]"},
		{"with filter", "RETURN [x IN [1, 2, 3] WHERE x > 1 | x * 2]"},
		{"filter only", "RETURN [x IN [1, 2, 3] WHERE x > 1]"},
		{"from variable", "MATCH (u:User) RETURN [x IN u.tags | toUpper(x)]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ast, err := cyphergrammar.Parse(tt.query)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.query, err)
			}
			if ast == nil {
				t.Fatalf("Parse(%q) returned nil AST", tt.query)
			}
		})
	}
}
