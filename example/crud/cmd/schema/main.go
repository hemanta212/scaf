// Command schema generates .scaf-schema.yaml from neogo models.
//
// Usage:
//
//	go run ./cmd/schema > .scaf-schema.yaml
package main

import (
	"log"
	"os"

	"github.com/rlch/scaf/adapters/neogo"
	"github.com/rlch/scaf/analysis"
	"github.com/rlch/scaf/example/crud/internal"
)

func main() {
	adapter := neogo.NewAdapter(
		&internal.User{},
		&internal.Post{},
		&internal.Comment{},
		&internal.Authored{},
		&internal.Wrote{},
		&internal.Has{},
		&internal.Replies{},
	)

	schema, err := adapter.ExtractSchema()
	if err != nil {
		log.Fatalf("failed to extract schema: %v", err)
	}

	if err := analysis.WriteSchema(os.Stdout, schema); err != nil {
		log.Fatalf("failed to write schema: %v", err)
	}
}
