package analysis

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/rlch/scaf"
	"gopkg.in/yaml.v3"
)

// TypeSchema represents the database schema extracted from user code.
// It is dialect-agnostic and populated by adapters (e.g., neogo) that crawl
// the user's codebase to discover models, fields, and relationships.
//
// The LSP uses this schema to provide completions and type information
// when editing queries. Dialects define what arguments/returns functions
// expect, and the TypeSchema maps those to concrete types from user code.
type TypeSchema struct {
	// Models maps model name (e.g., "Person", "ActedIn") to its definition.
	Models map[string]*Model
}

// NewTypeSchema creates an empty TypeSchema.
func NewTypeSchema() *TypeSchema {
	return &TypeSchema{
		Models: make(map[string]*Model),
	}
}

// yamlSchema is the YAML representation of TypeSchema.
type yamlSchema struct {
	Models map[string]*yamlModel `yaml:"models"`
}

// yamlModel is the YAML representation of Model.
type yamlModel struct {
	Fields        map[string]*yamlField        `yaml:"fields,omitempty"`
	Relationships map[string]*yamlRelationship `yaml:"relationships,omitempty"`
}

// yamlField is the YAML representation of Field.
type yamlField struct {
	Type     string `yaml:"type"`
	Required bool   `yaml:"required,omitempty"`
	Unique   bool   `yaml:"unique,omitempty"`
}

// yamlRelationship is the YAML representation of Relationship.
type yamlRelationship struct {
	RelType   string `yaml:"rel_type"`
	Target    string `yaml:"target"`
	Many      bool   `yaml:"many,omitempty"`
	Direction string `yaml:"direction"`
}

// LoadSchema loads a TypeSchema from a YAML file.
// The path can be absolute or relative to baseDir.
func LoadSchema(path, baseDir string) (*TypeSchema, error) {
	if path == "" {
		return nil, nil
	}

	// Resolve relative paths
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}

	cleanPath := filepath.Clean(path)

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("reading schema file: %w", err)
	}

	var ys yamlSchema
	if err := yaml.Unmarshal(data, &ys); err != nil {
		return nil, fmt.Errorf("parsing schema: %w", err)
	}

	return yamlSchemaToTypeSchema(&ys)
}

// yamlSchemaToTypeSchema converts the YAML representation to TypeSchema.
func yamlSchemaToTypeSchema(ys *yamlSchema) (*TypeSchema, error) {
	schema := NewTypeSchema()

	for modelName, ym := range ys.Models {
		model := &Model{
			Name:          modelName,
			Fields:        make([]*Field, 0),
			Relationships: make([]*Relationship, 0),
		}

		// Convert fields
		if ym.Fields != nil {
			for fieldName, yf := range ym.Fields {
				typ, err := ParseTypeString(yf.Type)
				if err != nil {
					return nil, fmt.Errorf("model %s, field %s: %w", modelName, fieldName, err)
				}

				model.Fields = append(model.Fields, &Field{
					Name:     fieldName,
					Type:     typ,
					Required: yf.Required,
					Unique:   yf.Unique,
				})
			}
		}

		// Convert relationships
		if ym.Relationships != nil {
			for relName, yr := range ym.Relationships {
				model.Relationships = append(model.Relationships, &Relationship{
					Name:      relName,
					RelType:   yr.RelType,
					Target:    yr.Target,
					Many:      yr.Many,
					Direction: Direction(yr.Direction),
				})
			}
		}

		schema.Models[model.Name] = model
	}

	return schema, nil
}

// WriteSchema writes a TypeSchema as YAML to the given writer.
// The output includes a yaml-language-server schema comment for editor validation.
func WriteSchema(w io.Writer, schema *TypeSchema) (err error) {
	// Write schema comment for editor validation
	if _, err := fmt.Fprintln(w, "# yaml-language-server: $schema=https://raw.githubusercontent.com/rlch/scaf/main/.scaf-type.schema.json"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	// Convert to YAML representation
	ys := &yamlSchema{
		Models: make(map[string]*yamlModel),
	}

	// Sort model names for deterministic output
	modelNames := make([]string, 0, len(schema.Models))
	for name := range schema.Models {
		modelNames = append(modelNames, name)
	}
	sort.Strings(modelNames)

	for _, name := range modelNames {
		model := schema.Models[name]

		ym := &yamlModel{}

		// Convert fields
		if len(model.Fields) > 0 {
			ym.Fields = make(map[string]*yamlField)
			for _, field := range model.Fields {
				ym.Fields[field.Name] = &yamlField{
					Type:     field.Type.String(),
					Required: field.Required,
					Unique:   field.Unique,
				}
			}
		}

		// Convert relationships
		if len(model.Relationships) > 0 {
			ym.Relationships = make(map[string]*yamlRelationship)
			for _, rel := range model.Relationships {
				ym.Relationships[rel.Name] = &yamlRelationship{
					RelType:   rel.RelType,
					Target:    rel.Target,
					Many:      rel.Many,
					Direction: string(rel.Direction),
				}
			}
		}

		ys.Models[name] = ym
	}

	encoder := yaml.NewEncoder(w)
	encoder.SetIndent(2)
	defer func() {
		if cerr := encoder.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	return encoder.Encode(ys)
}

// Type aliases - re-export from main scaf package for backward compatibility.
// These allow existing code using analysis.Type to continue working.
type (
	TypeKind = scaf.TypeKind
	Type     = scaf.Type
)

// Type kind constants - re-exported from main package.
const (
	TypeKindPrimitive = scaf.TypeKindPrimitive
	TypeKindSlice     = scaf.TypeKindSlice
	TypeKindArray     = scaf.TypeKindArray
	TypeKindMap       = scaf.TypeKindMap
	TypeKindPointer   = scaf.TypeKindPointer
	TypeKindNamed     = scaf.TypeKindNamed
)

// ParseTypeString parses a Go-style type string into a Type.
// Re-exported from main scaf package.
var ParseTypeString = scaf.ParseTypeString

// Primitive type constructors - re-exported from main package.
var (
	TypeString  = scaf.TypeString
	TypeInt     = scaf.TypeInt
	TypeInt64   = scaf.TypeInt64
	TypeFloat64 = scaf.TypeFloat64
	TypeBool    = scaf.TypeBool
)

// Type constructors - re-exported from main package.
var (
	SliceOf   = scaf.SliceOf
	PointerTo = scaf.PointerTo
	MapOf     = scaf.MapOf
	NamedType = scaf.NamedType
)

// Model represents a database entity (node, relationship, table, etc.).
// In graph databases, both nodes and relationships are models.
// In relational databases, tables are models.
type Model struct {
	// Name is the model identifier (e.g., "Person", "ACTED_IN").
	Name string

	// Fields are the properties/columns on this model.
	Fields []*Field

	// Relationships are edges from this model to other models.
	// Only applicable for node-like models in graph databases.
	Relationships []*Relationship
}

// Field represents a property/column on a model.
type Field struct {
	// Name is the field name as it appears in queries (e.g., "name", "age").
	Name string

	// Type is the field's type.
	Type *Type

	// Required indicates whether the field must have a value.
	Required bool

	// Unique indicates whether this field has a uniqueness constraint.
	// When a query filters on a unique field with equality, it returns at most one row.
	Unique bool
}

// Relationship represents an edge from one model to another.
type Relationship struct {
	// Name is the field name on the source model (e.g., "Friends", "ActedIn").
	Name string

	// RelType is the relationship type in the database (e.g., "FRIENDS", "ACTED_IN").
	RelType string

	// Target is the target model name.
	// For shorthand relationships: the target node (e.g., "Person").
	// For relationship structs: the relationship model (e.g., "ActedIn").
	Target string

	// Many indicates whether this is a one-to-many relationship.
	// true = Many[T], false = One[T]
	Many bool

	// Direction is the relationship direction.
	Direction Direction
}

// Direction represents the direction of a relationship.
type Direction string

const (
	// DirectionOutgoing represents an outgoing relationship (->).
	DirectionOutgoing Direction = "outgoing"
	// DirectionIncoming represents an incoming relationship (<-).
	DirectionIncoming Direction = "incoming"
)

// SchemaAdapter is the interface that adapter libraries implement to extract
// type schemas from user codebases.
//
// Adapters are created with the types to be registered, then ExtractSchema
// is called to generate the schema. Each adapter's constructor takes the
// types to register.
//
// Example usage in user's codebase:
//
//	// cmd/scaf-schema/main.go
//	package main
//
//	import (
//	    "log"
//	    "os"
//
//	    "github.com/rlch/scaf/adapters/neogo"
//	    "github.com/rlch/scaf/analysis"
//	    "myapp/models"
//	)
//
//	func main() {
//	    adapter := neogo.NewAdapter(
//	        &models.Person{},
//	        &models.Movie{},
//	        &models.ActedIn{},
//	    )
//	    schema, err := adapter.ExtractSchema()
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    if err := analysis.WriteSchema(os.Stdout, schema); err != nil {
//	        log.Fatal(err)
//	    }
//	}
//
// The user then runs: go run ./cmd/scaf-schema > .scaf-schema.yaml
// The LSP reads .scaf-schema.yaml to provide completions and type info.
type SchemaAdapter interface {
	// ExtractSchema discovers models from registered types and returns
	// a TypeSchema. The adapter is responsible for:
	// - Extracting fields and their types from registered types
	// - Discovering relationships between models
	// - Mapping library-specific metadata to the dialect-agnostic schema
	ExtractSchema() (*TypeSchema, error)
}

// SchemaAwareAnalyzer extends scaf.QueryAnalyzer with schema-aware analysis.
// When schema is provided, the analyzer can determine cardinality (ReturnsOne)
// by checking if the query filters on unique fields.
//
// Dialect analyzers that support cardinality inference should implement this interface.
// The Go code generator checks for this interface and uses it when available.
type SchemaAwareAnalyzer interface {
	// AnalyzeQueryWithSchema extracts metadata using schema for cardinality inference.
	// If schema is nil, behaves like AnalyzeQuery with ReturnsOne = false.
	AnalyzeQueryWithSchema(query string, schema *TypeSchema) (*scaf.QueryMetadata, error)
}
