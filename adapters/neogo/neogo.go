// Package neogo provides an adapter for the neogo ORM.
//
// This adapter is associated with the cypher dialect and provides:
//   - Struct mapping for Neo4j nodes and relationships
//   - Fluent query builder API
//   - Type-safe query construction
//
// # Usage
//
// The neogo adapter wraps github.com/rlch/neogo for application code,
// while scaf tests use the cypher dialect for simple value comparisons.
//
// # Example
//
//	// In your application code (uses neogo adapter)
//	type User struct {
//	    neogo.Node `neo4j:"User"`
//	    Name  string `neo4j:"name"`
//	    Email string `neo4j:"email"`
//	}
//
//	// In your .scaf test file (uses cypher dialect)
//	// query GetUser `MATCH (u:User {id: $id}) RETURN u.name, u.email`
//	// GetUser {
//	//     test "finds user" {
//	//         $id: 1
//	//         u.name: "Alice"
//	//     }
//	// }
package neogo

// Adapter provides neogo ORM integration for application code.
// Associated with the cypher dialect for test execution.
type Adapter struct {
	// TODO: Wrap neogo.Driver
}

// Dialect returns the associated dialect name.
func (a *Adapter) Dialect() string {
	return "cypher"
}
