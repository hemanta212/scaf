package scaf

import "context"

// SchemaIntrospector is implemented by databases that support schema extraction.
type SchemaIntrospector interface {
	// IntrospectSchema extracts the database schema (node types, properties, relationships).
	IntrospectSchema(ctx context.Context) (*Schema, error)
}

// Schema represents the extracted database schema.
type Schema struct {
	Models map[string]*ModelSchema `yaml:"models"`
}

// ModelSchema represents a node/entity type.
type ModelSchema struct {
	Fields        map[string]*FieldSchema        `yaml:"fields,omitempty"`
	Relationships map[string]*RelationshipSchema `yaml:"relationships,omitempty"`
}

// FieldSchema represents a property/field.
type FieldSchema struct {
	Type     string `yaml:"type"`
	Required bool   `yaml:"required"`
}

// RelationshipSchema represents a relationship to another model.
type RelationshipSchema struct {
	RelType   string `yaml:"rel_type"`
	Target    string `yaml:"target"`
	Many      bool   `yaml:"many"`
	Direction string `yaml:"direction"` // "outgoing" or "incoming"
}
