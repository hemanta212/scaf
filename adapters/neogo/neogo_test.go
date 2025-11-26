package neogo_test

import (
	"testing"

	"github.com/rlch/neogo"
	adapter "github.com/rlch/scaf/adapters/neogo"
	"github.com/rlch/scaf/analysis"
)

// Test models

// Person is a test node type.
type Person struct {
	neogo.Node `neo4j:"Person"`

	Name string `neo4j:"name"`
	Age  int    `neo4j:"age"`

	// Relationships
	Friends neogo.Many[Friendship] `neo4j:"->"`
	ActedIn neogo.Many[ActedIn]    `neo4j:"->"`
}

// Movie is a test node type.
type Movie struct {
	neogo.Node `neo4j:"Movie"`

	Title    string   `neo4j:"title"`
	Released int      `neo4j:"released"`
	Genres   []string `neo4j:"genres"`
}

// ActedIn is a test relationship type.
type ActedIn struct {
	neogo.Relationship `neo4j:"ACTED_IN"`

	Roles []string `neo4j:"roles"`

	Actor *Person `neo4j:"startNode"`
	Movie *Movie  `neo4j:"endNode"`
}

// Friendship is a self-referential relationship.
type Friendship struct {
	neogo.Relationship `neo4j:"FRIENDS_WITH"`

	Since int `neo4j:"since"`

	Person1 *Person `neo4j:"startNode"`
	Person2 *Person `neo4j:"endNode"`
}

// NodeWithOptionalFields tests optional (pointer) fields.
type NodeWithOptionalFields struct {
	neogo.Node `neo4j:"NodeWithOptionalFields"`

	Required string  `neo4j:"required"`
	Optional *string `neo4j:"optional"`
}

// NodeWithComplexTypes tests various type conversions.
// Note: Neo4j only supports scalar properties, so maps are not supported.
type NodeWithComplexTypes struct {
	neogo.Node `neo4j:"NodeWithComplexTypes"`

	StringSlice   []string `neo4j:"stringSlice"`
	IntSlice      []int    `neo4j:"intSlice"`
	PointerString *string  `neo4j:"pointerString"`
	PointerInt    *int     `neo4j:"pointerInt"`
}

// Test models for relationship direction tests.
type IncomingNode struct {
	neogo.Node `neo4j:"IncomingNode"`
	Name       string `neo4j:"name"`
}

type OutgoingNode struct {
	neogo.Node `neo4j:"OutgoingNode"`
	Name       string               `neo4j:"name"`
	Points     neogo.Many[PointsTo] `neo4j:"->"`
}

type PointsTo struct {
	neogo.Relationship `neo4j:"POINTS_TO"`
	From               *OutgoingNode `neo4j:"startNode"`
	To                 *IncomingNode `neo4j:"endNode"`
}

// Test models for One[T] cardinality.
type Owner struct {
	neogo.Node `neo4j:"Owner"`
	Name       string        `neo4j:"name"`
	Pet        neogo.One[Pet] `neo4j:"->"`
}

type Pet struct {
	neogo.Node `neo4j:"Pet"`
	Name       string `neo4j:"name"`
}

// Test model for nodes without relationships.
type Standalone struct {
	neogo.Node `neo4j:"Standalone"`
	Name       string `neo4j:"name"`
}

// Test models for relationship without custom properties.
type Start struct {
	neogo.Node `neo4j:"Start"`
	Name       string           `neo4j:"name"`
	Links      neogo.Many[Link] `neo4j:"->"`
}

type End struct {
	neogo.Node `neo4j:"End"`
	Name       string `neo4j:"name"`
}

type Link struct {
	neogo.Relationship `neo4j:"LINKS_TO"`
	From               *Start `neo4j:"startNode"`
	To                 *End   `neo4j:"endNode"`
}

func TestNewAdapter(t *testing.T) {
	tests := []struct {
		name  string
		types []any
	}{
		{
			name:  "empty types",
			types: []any{},
		},
		{
			name:  "single node",
			types: []any{&Person{}},
		},
		{
			name:  "multiple nodes",
			types: []any{&Person{}, &Movie{}},
		},
		{
			name:  "nodes and relationships",
			types: []any{&Person{}, &Movie{}, &ActedIn{}, &Friendship{}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := adapter.NewAdapter(tt.types...)
			if a == nil {
				t.Fatal("NewAdapter returned nil")
			}
		})
	}
}

func TestExtractSchema_Nodes(t *testing.T) {
	a := adapter.NewAdapter(&Person{}, &Movie{})
	schema, err := a.ExtractSchema()
	if err != nil {
		t.Fatalf("ExtractSchema failed: %v", err)
	}

	// Check Person node
	person, ok := schema.Models["Person"]
	if !ok {
		t.Fatal("Person model not found")
	}

	if person.Name != "Person" {
		t.Errorf("Person.Name = %q, want %q", person.Name, "Person")
	}

	// Check Person fields
	personFields := fieldMap(person.Fields)
	if _, ok := personFields["name"]; !ok {
		t.Error("Person should have 'name' field")
	}
	if _, ok := personFields["age"]; !ok {
		t.Error("Person should have 'age' field")
	}

	// Check Movie node
	movie, ok := schema.Models["Movie"]
	if !ok {
		t.Fatal("Movie model not found")
	}

	if movie.Name != "Movie" {
		t.Errorf("Movie.Name = %q, want %q", movie.Name, "Movie")
	}

	// Check Movie fields
	movieFields := fieldMap(movie.Fields)
	if _, ok := movieFields["title"]; !ok {
		t.Error("Movie should have 'title' field")
	}
	if _, ok := movieFields["released"]; !ok {
		t.Error("Movie should have 'released' field")
	}
	if _, ok := movieFields["genres"]; !ok {
		t.Error("Movie should have 'genres' field")
	}
}

func TestExtractSchema_RelationshipModels(t *testing.T) {
	a := adapter.NewAdapter(&Person{}, &Movie{}, &ActedIn{}, &Friendship{})
	schema, err := a.ExtractSchema()
	if err != nil {
		t.Fatalf("ExtractSchema failed: %v", err)
	}

	// Check ActedIn relationship model (stored by Go struct name, not Neo4j relationship type)
	actedIn, ok := schema.Models["ActedIn"]
	if !ok {
		t.Fatal("ActedIn model not found")
	}

	if actedIn.Name != "ActedIn" {
		t.Errorf("ActedIn.Name = %q, want %q", actedIn.Name, "ActedIn")
	}

	// Check ActedIn fields (should have roles, not startNode/endNode)
	actedInFields := fieldMap(actedIn.Fields)
	if _, ok := actedInFields["roles"]; !ok {
		t.Error("ActedIn should have 'roles' field")
	}
	if _, ok := actedInFields["startNode"]; ok {
		t.Error("ActedIn should not expose 'startNode' as a field")
	}
	if _, ok := actedInFields["endNode"]; ok {
		t.Error("ActedIn should not expose 'endNode' as a field")
	}

	// Check Friendship relationship model (stored by Go struct name)
	friendship, ok := schema.Models["Friendship"]
	if !ok {
		t.Fatal("Friendship model not found")
	}

	friendshipFields := fieldMap(friendship.Fields)
	if _, ok := friendshipFields["since"]; !ok {
		t.Error("Friendship should have 'since' field")
	}
}

func TestExtractSchema_NodeRelationships(t *testing.T) {
	a := adapter.NewAdapter(&Person{}, &Movie{}, &ActedIn{}, &Friendship{})
	schema, err := a.ExtractSchema()
	if err != nil {
		t.Fatalf("ExtractSchema failed: %v", err)
	}

	person, ok := schema.Models["Person"]
	if !ok {
		t.Fatal("Person model not found")
	}

	if len(person.Relationships) == 0 {
		t.Fatal("Person should have relationships")
	}

	// Check relationships exist
	rels := relationshipMap(person.Relationships)

	// Friends relationship
	if friends, ok := rels["Friends"]; ok {
		if friends.RelType != "FRIENDS_WITH" {
			t.Errorf("Friends.RelType = %q, want %q", friends.RelType, "FRIENDS_WITH")
		}
		if friends.Direction != analysis.DirectionOutgoing {
			t.Errorf("Friends.Direction = %q, want %q", friends.Direction, analysis.DirectionOutgoing)
		}
		if !friends.Many {
			t.Error("Friends should be Many")
		}
	} else {
		t.Error("Person should have 'Friends' relationship")
	}

	// ActedIn relationship
	if actedIn, ok := rels["ActedIn"]; ok {
		if actedIn.RelType != "ACTED_IN" {
			t.Errorf("ActedIn.RelType = %q, want %q", actedIn.RelType, "ACTED_IN")
		}
		if actedIn.Direction != analysis.DirectionOutgoing {
			t.Errorf("ActedIn.Direction = %q, want %q", actedIn.Direction, analysis.DirectionOutgoing)
		}
		if !actedIn.Many {
			t.Error("ActedIn should be Many")
		}
	} else {
		t.Error("Person should have 'ActedIn' relationship")
	}
}

func TestExtractSchema_FieldTypes(t *testing.T) {
	a := adapter.NewAdapter(&NodeWithComplexTypes{})
	schema, err := a.ExtractSchema()
	if err != nil {
		t.Fatalf("ExtractSchema failed: %v", err)
	}

	model, ok := schema.Models["NodeWithComplexTypes"]
	if !ok {
		t.Fatal("NodeWithComplexTypes model not found")
	}

	fields := fieldMap(model.Fields)

	tests := []struct {
		fieldName    string
		expectedKind analysis.TypeKind
		description  string
	}{
		{"stringSlice", analysis.TypeKindSlice, "slice of string"},
		{"intSlice", analysis.TypeKindSlice, "slice of int"},
		{"pointerString", analysis.TypeKindPointer, "pointer to string"},
		{"pointerInt", analysis.TypeKindPointer, "pointer to int"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			field, ok := fields[tt.fieldName]
			if !ok {
				t.Fatalf("field %q not found", tt.fieldName)
			}

			if field.Type == nil {
				t.Fatalf("field %q has nil type", tt.fieldName)
			}

			if field.Type.Kind != tt.expectedKind {
				t.Errorf("field %q type kind = %q, want %q", tt.fieldName, field.Type.Kind, tt.expectedKind)
			}
		})
	}

	// Verify slice element types
	if field := fields["stringSlice"]; field != nil && field.Type != nil && field.Type.Elem != nil {
		if field.Type.Elem.Name != "string" {
			t.Errorf("stringSlice elem type = %q, want %q", field.Type.Elem.Name, "string")
		}
	}

	if field := fields["intSlice"]; field != nil && field.Type != nil && field.Type.Elem != nil {
		if field.Type.Elem.Name != "int" {
			t.Errorf("intSlice elem type = %q, want %q", field.Type.Elem.Name, "int")
		}
	}

	// Verify pointer element types
	if field := fields["pointerString"]; field != nil && field.Type != nil && field.Type.Elem != nil {
		if field.Type.Elem.Name != "string" {
			t.Errorf("pointerString elem type = %q, want %q", field.Type.Elem.Name, "string")
		}
	}

	if field := fields["pointerInt"]; field != nil && field.Type != nil && field.Type.Elem != nil {
		if field.Type.Elem.Name != "int" {
			t.Errorf("pointerInt elem type = %q, want %q", field.Type.Elem.Name, "int")
		}
	}
}

func TestExtractSchema_RequiredFields(t *testing.T) {
	a := adapter.NewAdapter(&NodeWithOptionalFields{})
	schema, err := a.ExtractSchema()
	if err != nil {
		t.Fatalf("ExtractSchema failed: %v", err)
	}

	model, ok := schema.Models["NodeWithOptionalFields"]
	if !ok {
		t.Fatal("NodeWithOptionalFields model not found")
	}

	fields := fieldMap(model.Fields)

	// Required field (non-pointer) should be required
	if reqField, ok := fields["required"]; ok {
		if !reqField.Required {
			t.Error("'required' field should be Required=true")
		}
	} else {
		t.Error("'required' field not found")
	}

	// Optional field (pointer) should not be required
	if optField, ok := fields["optional"]; ok {
		if optField.Required {
			t.Error("'optional' field should be Required=false")
		}
	} else {
		t.Error("'optional' field not found")
	}
}

func TestExtractSchema_PrimitiveTypes(t *testing.T) {
	a := adapter.NewAdapter(&Person{}, &Movie{})
	schema, err := a.ExtractSchema()
	if err != nil {
		t.Fatalf("ExtractSchema failed: %v", err)
	}

	person := schema.Models["Person"]
	fields := fieldMap(person.Fields)

	// Check string type
	if nameField, ok := fields["name"]; ok {
		if nameField.Type == nil {
			t.Fatal("name field has nil type")
		}
		if nameField.Type.Kind != analysis.TypeKindPrimitive {
			t.Errorf("name type kind = %q, want %q", nameField.Type.Kind, analysis.TypeKindPrimitive)
		}
		if nameField.Type.Name != "string" {
			t.Errorf("name type name = %q, want %q", nameField.Type.Name, "string")
		}
	} else {
		t.Error("name field not found")
	}

	// Check int type
	if ageField, ok := fields["age"]; ok {
		if ageField.Type == nil {
			t.Fatal("age field has nil type")
		}
		if ageField.Type.Kind != analysis.TypeKindPrimitive {
			t.Errorf("age type kind = %q, want %q", ageField.Type.Kind, analysis.TypeKindPrimitive)
		}
		if ageField.Type.Name != "int" {
			t.Errorf("age type name = %q, want %q", ageField.Type.Name, "int")
		}
	} else {
		t.Error("age field not found")
	}
}

func TestExtractSchema_TypeString(t *testing.T) {
	a := adapter.NewAdapter(&NodeWithComplexTypes{})
	schema, err := a.ExtractSchema()
	if err != nil {
		t.Fatalf("ExtractSchema failed: %v", err)
	}

	model := schema.Models["NodeWithComplexTypes"]
	fields := fieldMap(model.Fields)

	tests := []struct {
		fieldName string
		expected  string
	}{
		{"stringSlice", "[]string"},
		{"intSlice", "[]int"},
		{"pointerString", "*string"},
		{"pointerInt", "*int"},
	}

	for _, tt := range tests {
		t.Run(tt.fieldName, func(t *testing.T) {
			field, ok := fields[tt.fieldName]
			if !ok {
				t.Fatalf("field %q not found", tt.fieldName)
			}

			if field.Type == nil {
				t.Fatalf("field %q has nil type", tt.fieldName)
			}

			str := field.Type.String()
			if str != tt.expected {
				t.Errorf("Type.String() = %q, want %q", str, tt.expected)
			}
		})
	}
}

func TestExtractSchema_ImplementsSchemaAdapter(t *testing.T) {
	// Verify that Adapter implements analysis.SchemaAdapter
	var _ analysis.SchemaAdapter = adapter.NewAdapter()
}

func TestExtractSchema_EmptyRegistry(t *testing.T) {
	a := adapter.NewAdapter()
	schema, err := a.ExtractSchema()
	if err != nil {
		t.Fatalf("ExtractSchema failed: %v", err)
	}

	if schema == nil {
		t.Fatal("schema should not be nil")
	}

	if schema.Models == nil {
		t.Fatal("schema.Models should not be nil")
	}

	if len(schema.Models) != 0 {
		t.Errorf("schema.Models should be empty, got %d models", len(schema.Models))
	}
}

func TestExtractSchema_RelationshipTarget(t *testing.T) {
	a := adapter.NewAdapter(&Person{}, &Movie{}, &ActedIn{}, &Friendship{})
	schema, err := a.ExtractSchema()
	if err != nil {
		t.Fatalf("ExtractSchema failed: %v", err)
	}

	person := schema.Models["Person"]
	rels := relationshipMap(person.Relationships)

	// ActedIn relationship should target Movie
	if actedIn, ok := rels["ActedIn"]; ok {
		if actedIn.Target != "Movie" {
			t.Errorf("ActedIn.Target = %q, want %q", actedIn.Target, "Movie")
		}
	}

	// Friends relationship should target Person (self-referential)
	if friends, ok := rels["Friends"]; ok {
		if friends.Target != "Person" {
			t.Errorf("Friends.Target = %q, want %q", friends.Target, "Person")
		}
	}
}

func TestExtractSchema_SliceFieldWithSliceElement(t *testing.T) {
	// Movie has genres which is []string
	a := adapter.NewAdapter(&Movie{})
	schema, err := a.ExtractSchema()
	if err != nil {
		t.Fatalf("ExtractSchema failed: %v", err)
	}

	movie := schema.Models["Movie"]
	fields := fieldMap(movie.Fields)

	genres, ok := fields["genres"]
	if !ok {
		t.Fatal("genres field not found")
	}

	if genres.Type.Kind != analysis.TypeKindSlice {
		t.Errorf("genres type kind = %q, want %q", genres.Type.Kind, analysis.TypeKindSlice)
	}

	if genres.Type.Elem == nil {
		t.Fatal("genres type elem is nil")
	}

	if genres.Type.Elem.Kind != analysis.TypeKindPrimitive {
		t.Errorf("genres elem type kind = %q, want %q", genres.Type.Elem.Kind, analysis.TypeKindPrimitive)
	}

	if genres.Type.Elem.Name != "string" {
		t.Errorf("genres elem type name = %q, want %q", genres.Type.Elem.Name, "string")
	}
}

func TestExtractSchema_RelationshipDirection(t *testing.T) {
	// Test that relationships are extracted with correct direction
	a := adapter.NewAdapter(&IncomingNode{}, &OutgoingNode{}, &PointsTo{})
	schema, err := a.ExtractSchema()
	if err != nil {
		t.Fatalf("ExtractSchema failed: %v", err)
	}

	outgoing := schema.Models["OutgoingNode"]
	if outgoing == nil {
		t.Fatal("OutgoingNode model not found")
	}

	rels := relationshipMap(outgoing.Relationships)
	if points, ok := rels["Points"]; ok {
		if points.Direction != analysis.DirectionOutgoing {
			t.Errorf("Points.Direction = %q, want %q", points.Direction, analysis.DirectionOutgoing)
		}
		if points.Target != "IncomingNode" {
			t.Errorf("Points.Target = %q, want %q", points.Target, "IncomingNode")
		}
	} else {
		t.Error("OutgoingNode should have 'Points' relationship")
	}
}

func TestExtractSchema_OneRelationship(t *testing.T) {
	// Test One[T] vs Many[T] cardinality
	a := adapter.NewAdapter(&Owner{}, &Pet{})
	schema, err := a.ExtractSchema()
	if err != nil {
		t.Fatalf("ExtractSchema failed: %v", err)
	}

	owner := schema.Models["Owner"]
	if owner == nil {
		t.Fatal("Owner model not found")
	}

	rels := relationshipMap(owner.Relationships)
	if pet, ok := rels["Pet"]; ok {
		if pet.Many {
			t.Error("Pet relationship should be One (Many=false)")
		}
	} else {
		t.Error("Owner should have 'Pet' relationship")
	}
}

func TestExtractSchema_NoRelationships(t *testing.T) {
	// Test node with no relationships
	a := adapter.NewAdapter(&Standalone{})
	schema, err := a.ExtractSchema()
	if err != nil {
		t.Fatalf("ExtractSchema failed: %v", err)
	}

	standalone := schema.Models["Standalone"]
	if standalone == nil {
		t.Fatal("Standalone model not found")
	}

	if len(standalone.Relationships) != 0 {
		t.Errorf("Standalone should have no relationships, got %d", len(standalone.Relationships))
	}
}

func TestExtractSchema_RelationshipWithoutProperties(t *testing.T) {
	// Test relationship struct that has no custom properties (only startNode/endNode)
	a := adapter.NewAdapter(&Start{}, &End{}, &Link{})
	schema, err := a.ExtractSchema()
	if err != nil {
		t.Fatalf("ExtractSchema failed: %v", err)
	}

	// Link should exist as a model with no fields (startNode/endNode are filtered out)
	link := schema.Models["Link"]
	if link == nil {
		t.Fatal("Link model not found")
	}

	if len(link.Fields) != 0 {
		t.Errorf("Link should have no fields, got %d", len(link.Fields))
	}
}

func TestExtractSchema_SelfReferentialRelationship(t *testing.T) {
	// Test relationships where start and end node are the same type

	a := adapter.NewAdapter(&Person{}, &Friendship{})
	schema, err := a.ExtractSchema()
	if err != nil {
		t.Fatalf("ExtractSchema failed: %v", err)
	}

	person := schema.Models["Person"]
	if person == nil {
		t.Fatal("Person model not found")
	}

	rels := relationshipMap(person.Relationships)
	if friends, ok := rels["Friends"]; ok {
		// Self-referential relationship should target the same type
		if friends.Target != "Person" {
			t.Errorf("Friends.Target = %q, want %q", friends.Target, "Person")
		}
	} else {
		t.Error("Person should have 'Friends' relationship")
	}
}

func TestType_String_Primitives(t *testing.T) {
	tests := []struct {
		typ      *analysis.Type
		expected string
	}{
		{analysis.TypeString, "string"},
		{analysis.TypeInt, "int"},
		{analysis.TypeInt64, "int64"},
		{analysis.TypeFloat64, "float64"},
		{analysis.TypeBool, "bool"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.typ.String(); got != tt.expected {
				t.Errorf("Type.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestType_String_Complex(t *testing.T) {
	tests := []struct {
		name     string
		typ      *analysis.Type
		expected string
	}{
		{"slice", analysis.SliceOf(analysis.TypeString), "[]string"},
		{"pointer", analysis.PointerTo(analysis.TypeInt), "*int"},
		{"map", analysis.MapOf(analysis.TypeString, analysis.TypeBool), "map[string]bool"},
		{"nested slice", analysis.SliceOf(analysis.SliceOf(analysis.TypeInt)), "[][]int"},
		{"pointer to slice", analysis.PointerTo(analysis.SliceOf(analysis.TypeString)), "*[]string"},
		{"slice of pointers", analysis.SliceOf(analysis.PointerTo(analysis.TypeInt)), "[]*int"},
		{"named type", analysis.NamedType("time", "Time"), "time.Time"},
		{"array", &analysis.Type{Kind: analysis.TypeKindArray, ArrayLen: 5, Elem: analysis.TypeInt}, "[5]int"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.typ.String(); got != tt.expected {
				t.Errorf("Type.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestType_String_Nil(t *testing.T) {
	var typ *analysis.Type
	if got := typ.String(); got != "" {
		t.Errorf("nil Type.String() = %q, want empty string", got)
	}
}

// Helper functions

func fieldMap(fields []*analysis.Field) map[string]*analysis.Field {
	m := make(map[string]*analysis.Field, len(fields))
	for _, f := range fields {
		m[f.Name] = f
	}
	return m
}

func relationshipMap(rels []*analysis.Relationship) map[string]*analysis.Relationship {
	m := make(map[string]*analysis.Relationship, len(rels))
	for _, r := range rels {
		m[r.Name] = r
	}
	return m
}
