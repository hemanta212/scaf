// Command schema generates a .scaf-schema.hcl file from neogo models.
//
// Usage:
//
//	go run ./cmd/schema > .scaf-schema.hcl
package main

import (
	"log"
	"os"

	"github.com/rlch/scaf/adapters/neogo"
	"github.com/rlch/scaf/analysis"
	"github.com/rlch/scaf/example/neogo/models"
)

func main() {
	adapter := neogo.NewAdapter(
		&models.Person{},
		&models.Movie{},
		&models.ActedIn{},
		&models.Directed{},
		&models.Review{},
		&models.Follows{},
	)

	schema, err := adapter.ExtractSchema()
	if err != nil {
		log.Fatalf("failed to extract schema: %v", err)
	}

	if err := analysis.WriteSchema(os.Stdout, schema); err != nil {
		log.Fatalf("failed to write schema: %v", err)
	}
}
