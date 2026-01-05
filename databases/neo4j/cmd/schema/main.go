// Command schema extracts schema from a Neo4j database.
//
// Usage:
//
//	go run github.com/rlch/scaf/databases/neo4j/cmd/schema > .scaf-schema.yaml
//
// Or with explicit connection:
//
//	go run github.com/rlch/scaf/databases/neo4j/cmd/schema -uri bolt://localhost:7687 -user neo4j -password secret
//
// The command reads .scaf.yaml for connection details if no flags are provided.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"
)

func main() {
	uri := flag.String("uri", "", "Neo4j URI (default: from .scaf.yaml)")
	user := flag.String("user", "", "Neo4j username (default: from .scaf.yaml)")
	password := flag.String("password", "", "Neo4j password (default: from .scaf.yaml)")
	flag.Parse()

	// Load from config if flags not provided
	if *uri == "" {
		cfg, err := loadConfig()
		if err != nil {
			log.Fatalf("failed to load config: %v", err)
		}
		*uri = cfg.Neo4j.URI
		*user = cfg.Neo4j.Username
		*password = cfg.Neo4j.Password
	}

	if *uri == "" {
		log.Fatal("Neo4j URI required: provide -uri flag or configure .scaf.yaml")
	}

	ctx := context.Background()

	// Connect to Neo4j
	driver, err := neo4j.NewDriverWithContext(*uri, neo4j.BasicAuth(*user, *password, ""))
	if err != nil {
		log.Fatalf("failed to connect to Neo4j: %v", err)
	}
	defer driver.Close(ctx)

	// Verify connection
	if err := driver.VerifyConnectivity(ctx); err != nil {
		log.Fatalf("failed to verify Neo4j connection: %v", err)
	}

	fmt.Fprintf(os.Stderr, "Connected to %s\n", *uri)

	// Extract schema
	schema, err := introspectSchema(ctx, driver)
	if err != nil {
		log.Fatalf("failed to extract schema: %v", err)
	}

	// Write to stdout
	if err := analysis.WriteSchema(os.Stdout, schema); err != nil {
		log.Fatalf("failed to write schema: %v", err)
	}

	fmt.Fprintf(os.Stderr, "Schema extracted: %d models\n", len(schema.Models))
}

func loadConfig() (*scaf.Config, error) {
	configPath, err := scaf.FindConfig(".")
	if err != nil {
		return nil, err
	}
	return scaf.LoadConfig(configPath)
}

func introspectSchema(ctx context.Context, driver neo4j.DriverWithContext) (*analysis.TypeSchema, error) {
	schema := analysis.NewTypeSchema()

	session := driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx)

	// Get node properties
	nodeResult, err := session.Run(ctx, `
		CALL db.schema.nodeTypeProperties()
		YIELD nodeType, propertyName, propertyTypes, mandatory
		RETURN nodeType, propertyName, propertyTypes, mandatory
		ORDER BY nodeType, propertyName
	`, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get node properties: %w", err)
	}

	for nodeResult.Next(ctx) {
		record := nodeResult.Record()
		nodeType := extractLabel(record.Values[0])
		propName, _ := record.Values[1].(string)
		propTypes, _ := record.Values[2].([]any)
		mandatory, _ := record.Values[3].(bool)

		if nodeType == "" || propName == "" {
			continue
		}

		if schema.Models[nodeType] == nil {
			schema.Models[nodeType] = &analysis.Model{
				Name:   nodeType,
				Fields: make([]*analysis.Field, 0),
			}
		}

		schema.Models[nodeType].Fields = append(schema.Models[nodeType].Fields, &analysis.Field{
			Name:     propName,
			Type:     mapNeo4jType(propTypes),
			Required: mandatory,
		})
	}

	if err := nodeResult.Err(); err != nil {
		return nil, fmt.Errorf("error reading node properties: %w", err)
	}

	// Get relationship types
	relResult, err := session.Run(ctx, `
		CALL db.schema.relTypeProperties()
		YIELD relType, propertyName, propertyTypes, mandatory
		RETURN DISTINCT relType
	`, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get relationship types: %w", err)
	}

	for relResult.Next(ctx) {
		record := relResult.Record()
		relType := extractRelType(record.Values[0])
		if relType == "" {
			continue
		}
		modelName := toCamelCase(relType)
		if schema.Models[modelName] == nil {
			schema.Models[modelName] = &analysis.Model{
				Name:   modelName,
				Fields: make([]*analysis.Field, 0),
			}
		}
	}

	if err := relResult.Err(); err != nil {
		return nil, fmt.Errorf("error reading relationship types: %w", err)
	}

	// Sort fields for deterministic output
	for _, model := range schema.Models {
		sort.Slice(model.Fields, func(i, j int) bool {
			return model.Fields[i].Name < model.Fields[j].Name
		})
	}

	return schema, nil
}

func extractLabel(v any) string {
	switch t := v.(type) {
	case string:
		s := strings.TrimPrefix(t, ":")
		s = strings.Trim(s, `"`)
		s = strings.Trim(s, "`")
		return s
	case []any:
		if len(t) > 0 {
			if s, ok := t[0].(string); ok {
				return s
			}
		}
	}
	return ""
}

func extractRelType(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	s = strings.TrimPrefix(s, ":")
	s = strings.Trim(s, `"`)
	s = strings.Trim(s, "`")
	return s
}

func mapNeo4jType(types []any) *analysis.Type {
	if len(types) == 0 {
		return &analysis.Type{Kind: analysis.TypeKindPrimitive, Name: "any"}
	}
	t, ok := types[0].(string)
	if !ok {
		return &analysis.Type{Kind: analysis.TypeKindPrimitive, Name: "any"}
	}
	switch {
	case strings.Contains(t, "Long"), strings.Contains(t, "Integer"):
		return analysis.TypeInt
	case strings.Contains(t, "Double"), strings.Contains(t, "Float"):
		return analysis.TypeFloat64
	case strings.Contains(t, "Boolean"):
		return analysis.TypeBool
	case strings.Contains(t, "String"):
		return analysis.TypeString
	case strings.Contains(t, "List"):
		return analysis.SliceOf(analysis.TypeString)
	default:
		return &analysis.Type{Kind: analysis.TypeKindPrimitive, Name: "any"}
	}
}

func toCamelCase(s string) string {
	parts := strings.Split(strings.ToLower(s), "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}
