// Package cyphergrammar provides a parser for Cypher queries built with participle.
//
// This package contains the lexer, AST types, and parser for the openCypher query
// language. It replaces the previous ANTLR-based parser to fix grammar ambiguities
// and provide better integration with Go code.
//
// # Key Features
//
//   - Proper disambiguation of list literals vs list comprehensions
//   - Case-insensitive keyword matching
//   - Support for Neo4j and APOC functions
//   - Type-safe AST with lexer.Position tracking
//
// # Usage
//
//	ast, err := cyphergrammar.Parse("MATCH (u:User) RETURN u.name")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	// Work with ast...
//
// # Grammar Origin
//
// The grammar is based on the openCypher specification:
// https://github.com/opencypher/openCypher
package cyphergrammar
