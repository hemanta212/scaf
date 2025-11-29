package analysis

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSchema(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "schema.yaml")

	schemaYAML := `
models:
  User:
    fields:
      id:
        type: string
        required: true
        unique: true
      name:
        type: string
        required: true
      age:
        type: int
        required: true
`

	err := os.WriteFile(schemaPath, []byte(schemaYAML), 0o644)
	require.NoError(t, err)

	schema, err := LoadSchema(schemaPath, "")
	require.NoError(t, err)
	require.NotNil(t, schema)

	// Verify User model
	user, ok := schema.Models["User"]
	require.True(t, ok, "User model should exist")
	assert.Equal(t, "User", user.Name)
	assert.Len(t, user.Fields, 3)

	// Verify id field is unique
	var idField *Field
	for _, f := range user.Fields {
		if f.Name == "id" {
			idField = f

			break
		}
	}
	require.NotNil(t, idField)
	assert.True(t, idField.Unique)
	assert.True(t, idField.Required)
	assert.Equal(t, "string", idField.Type.Name)
	assert.Equal(t, TypeKindPrimitive, idField.Type.Kind)
}

func TestLoadSchemaWithRelationships(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "schema.yaml")

	schemaYAML := `
models:
  Person:
    fields:
      name:
        type: string
        required: true
    relationships:
      ActedIn:
        rel_type: ACTED_IN
        target: Movie
        many: true
        direction: outgoing
  Movie:
    fields:
      title:
        type: string
        required: true
`

	err := os.WriteFile(schemaPath, []byte(schemaYAML), 0o644)
	require.NoError(t, err)

	schema, err := LoadSchema(schemaPath, "")
	require.NoError(t, err)
	require.NotNil(t, schema)

	// Verify Person model and relationship
	person, ok := schema.Models["Person"]
	require.True(t, ok)
	assert.Len(t, person.Relationships, 1)

	rel := person.Relationships[0]
	assert.Equal(t, "ActedIn", rel.Name)
	assert.Equal(t, "ACTED_IN", rel.RelType)
	assert.Equal(t, "Movie", rel.Target)
	assert.True(t, rel.Many)
	assert.Equal(t, DirectionOutgoing, rel.Direction)

	// Verify Movie model
	movie, ok := schema.Models["Movie"]
	require.True(t, ok)
	assert.Len(t, movie.Relationships, 0)
}

func TestLoadSchemaComplexTypes(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "schema.yaml")

	schemaYAML := `
models:
  TestModel:
    fields:
      stringSlice:
        type: "[]string"
        required: true
      intPointer:
        type: "*int"
      stringMap:
        type: "map[string]int"
      namedType:
        type: time.Time
      intArray:
        type: "[5]int"
      nestedSlice:
        type: "[][]string"
`

	err := os.WriteFile(schemaPath, []byte(schemaYAML), 0o644)
	require.NoError(t, err)

	schema, err := LoadSchema(schemaPath, "")
	require.NoError(t, err)
	require.NotNil(t, schema)

	model, ok := schema.Models["TestModel"]
	require.True(t, ok)

	fieldMap := make(map[string]*Field)
	for _, f := range model.Fields {
		fieldMap[f.Name] = f
	}

	// Test slice type
	f := fieldMap["stringSlice"]
	require.NotNil(t, f)
	assert.Equal(t, TypeKindSlice, f.Type.Kind)
	assert.Equal(t, TypeKindPrimitive, f.Type.Elem.Kind)
	assert.Equal(t, "string", f.Type.Elem.Name)

	// Test pointer type
	f = fieldMap["intPointer"]
	require.NotNil(t, f)
	assert.Equal(t, TypeKindPointer, f.Type.Kind)
	assert.Equal(t, TypeKindPrimitive, f.Type.Elem.Kind)
	assert.Equal(t, "int", f.Type.Elem.Name)

	// Test map type
	f = fieldMap["stringMap"]
	require.NotNil(t, f)
	assert.Equal(t, TypeKindMap, f.Type.Kind)
	assert.Equal(t, "string", f.Type.Key.Name)
	assert.Equal(t, "int", f.Type.Elem.Name)

	// Test named type
	f = fieldMap["namedType"]
	require.NotNil(t, f)
	assert.Equal(t, TypeKindNamed, f.Type.Kind)
	assert.Equal(t, "time", f.Type.Package)
	assert.Equal(t, "Time", f.Type.Name)

	// Test array type
	f = fieldMap["intArray"]
	require.NotNil(t, f)
	assert.Equal(t, TypeKindArray, f.Type.Kind)
	assert.Equal(t, 5, f.Type.ArrayLen)
	assert.Equal(t, "int", f.Type.Elem.Name)

	// Test nested slice
	f = fieldMap["nestedSlice"]
	require.NotNil(t, f)
	assert.Equal(t, TypeKindSlice, f.Type.Kind)
	assert.Equal(t, TypeKindSlice, f.Type.Elem.Kind)
	assert.Equal(t, "string", f.Type.Elem.Elem.Name)
}

func TestLoadSchemaRelativePath(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "schema.yaml")

	schemaYAML := `models:
  Empty: {}
`
	err := os.WriteFile(schemaPath, []byte(schemaYAML), 0o644)
	require.NoError(t, err)

	// Test loading with relative path
	schema, err := LoadSchema("schema.yaml", tmpDir)
	require.NoError(t, err)
	require.NotNil(t, schema)
}

func TestLoadSchemaEmptyPath(t *testing.T) {
	t.Parallel()

	// Empty path should return nil without error
	schema, err := LoadSchema("", "/some/dir")
	require.NoError(t, err)
	assert.Nil(t, schema)
}

func TestLoadSchemaNotFound(t *testing.T) {
	t.Parallel()

	_, err := LoadSchema("/nonexistent/path/schema.yaml", "")
	require.Error(t, err)
}

func TestLoadSchemaInvalid(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "invalid.yaml")

	err := os.WriteFile(schemaPath, []byte("not: [valid: yaml: {{"), 0o644)
	require.NoError(t, err)

	_, err = LoadSchema(schemaPath, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing schema")
}

func TestWriteSchema(t *testing.T) {
	t.Parallel()

	schema := &TypeSchema{
		Models: map[string]*Model{
			"Person": {
				Name: "Person",
				Fields: []*Field{
					{Name: "id", Type: TypeString, Required: true, Unique: true},
					{Name: "name", Type: TypeString, Required: true},
					{Name: "age", Type: TypeInt},
				},
				Relationships: []*Relationship{
					{
						Name:      "ActedIn",
						RelType:   "ACTED_IN",
						Target:    "Movie",
						Many:      true,
						Direction: DirectionOutgoing,
					},
				},
			},
			"Movie": {
				Name: "Movie",
				Fields: []*Field{
					{Name: "title", Type: TypeString, Required: true},
					{Name: "genres", Type: SliceOf(TypeString)},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := WriteSchema(&buf, schema)
	require.NoError(t, err)

	output := buf.String()

	// Verify output contains yaml-language-server schema comment
	assert.Contains(t, output, "# yaml-language-server: $schema=")

	// Verify output contains expected content
	assert.Contains(t, output, "Movie:")
	assert.Contains(t, output, "Person:")
	assert.Contains(t, output, "id:")
	assert.Contains(t, output, "name:")
	assert.Contains(t, output, "title:")
	assert.Contains(t, output, "type: string")
	// YAML encoder uses single quotes for strings with special characters
	assert.Contains(t, output, "type: '[]string'")
	assert.Contains(t, output, "required: true")
	assert.Contains(t, output, "unique: true")
	assert.Contains(t, output, "ActedIn:")
	assert.Contains(t, output, "rel_type: ACTED_IN")
	assert.Contains(t, output, "target: Movie")
	assert.Contains(t, output, "many: true")
	assert.Contains(t, output, "direction: outgoing")
}

func TestWriteSchemaRoundTrip(t *testing.T) {
	t.Parallel()

	// Create a schema
	original := &TypeSchema{
		Models: map[string]*Model{
			"User": {
				Name: "User",
				Fields: []*Field{
					{Name: "id", Type: TypeString, Required: true, Unique: true},
					{Name: "emails", Type: SliceOf(TypeString), Required: true},
					{Name: "metadata", Type: MapOf(TypeString, TypeString)},
				},
				Relationships: []*Relationship{
					{
						Name:      "Friends",
						RelType:   "FRIENDS_WITH",
						Target:    "User",
						Many:      true,
						Direction: DirectionOutgoing,
					},
				},
			},
		},
	}

	// Write to YAML
	var buf bytes.Buffer
	err := WriteSchema(&buf, original)
	require.NoError(t, err)

	// Write to temp file and read back
	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "schema.yaml")
	err = os.WriteFile(schemaPath, buf.Bytes(), 0o644)
	require.NoError(t, err)

	loaded, err := LoadSchema(schemaPath, "")
	require.NoError(t, err)

	// Verify round-trip
	require.Len(t, loaded.Models, 1)

	user := loaded.Models["User"]
	require.NotNil(t, user)
	assert.Equal(t, "User", user.Name)
	assert.Len(t, user.Fields, 3)
	assert.Len(t, user.Relationships, 1)

	// Check field types survived round-trip
	fieldMap := make(map[string]*Field)
	for _, f := range user.Fields {
		fieldMap[f.Name] = f
	}

	assert.Equal(t, TypeKindPrimitive, fieldMap["id"].Type.Kind)
	assert.Equal(t, TypeKindSlice, fieldMap["emails"].Type.Kind)
	assert.Equal(t, TypeKindMap, fieldMap["metadata"].Type.Kind)

	// Check relationship
	rel := user.Relationships[0]
	assert.Equal(t, "Friends", rel.Name)
	assert.Equal(t, "FRIENDS_WITH", rel.RelType)
	assert.Equal(t, DirectionOutgoing, rel.Direction)
}

func TestWriteSchemaEmptyModel(t *testing.T) {
	t.Parallel()

	schema := &TypeSchema{
		Models: map[string]*Model{
			"Empty": {
				Name: "Empty",
			},
		},
	}

	var buf bytes.Buffer
	err := WriteSchema(&buf, schema)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Empty:")

	// Verify it can be loaded back
	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "schema.yaml")
	err = os.WriteFile(schemaPath, buf.Bytes(), 0o644)
	require.NoError(t, err)

	loaded, err := LoadSchema(schemaPath, "")
	require.NoError(t, err)
	require.NotNil(t, loaded.Models["Empty"])
}

func TestParseTypeString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected *Type
		wantErr  bool
	}{
		{"primitive string", "string", TypeString, false},
		{"primitive int", "int", TypeInt, false},
		{"primitive int64", "int64", TypeInt64, false},
		{"primitive bool", "bool", TypeBool, false},
		{"slice of string", "[]string", SliceOf(TypeString), false},
		{"slice of int", "[]int", SliceOf(TypeInt), false},
		{"nested slice", "[][]string", &Type{Kind: TypeKindSlice, Elem: SliceOf(TypeString)}, false},
		{"array", "[5]int", &Type{Kind: TypeKindArray, ArrayLen: 5, Elem: TypeInt}, false},
		{"pointer to int", "*int", PointerTo(TypeInt), false},
		{"pointer to string", "*string", PointerTo(TypeString), false},
		{"slice of pointers", "[]*int", SliceOf(PointerTo(TypeInt)), false},
		{"pointer to slice", "*[]string", PointerTo(SliceOf(TypeString)), false},
		{"map string to int", "map[string]int", MapOf(TypeString, TypeInt), false},
		{"map string to bool", "map[string]bool", MapOf(TypeString, TypeBool), false},
		{"named type time.Time", "time.Time", NamedType("time", "Time"), false},
		{"named type uuid.UUID", "uuid.UUID", NamedType("uuid", "UUID"), false},
		{"empty string", "", nil, true},
		{"invalid array syntax", "[abc]int", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseTypeString(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected.Kind, result.Kind)
			assert.Equal(t, tt.expected.Name, result.Name)
			assert.Equal(t, tt.expected.Package, result.Package)
			assert.Equal(t, tt.expected.ArrayLen, result.ArrayLen)

			if tt.expected.Elem != nil {
				require.NotNil(t, result.Elem)
				assert.Equal(t, tt.expected.Elem.Kind, result.Elem.Kind)
				assert.Equal(t, tt.expected.Elem.Name, result.Elem.Name)
			}

			if tt.expected.Key != nil {
				require.NotNil(t, result.Key)
				assert.Equal(t, tt.expected.Key.Kind, result.Key.Kind)
				assert.Equal(t, tt.expected.Key.Name, result.Key.Name)
			}
		})
	}
}

func TestTypeStringRoundTrip(t *testing.T) {
	t.Parallel()

	types := []*Type{
		TypeString,
		TypeInt,
		TypeInt64,
		TypeFloat64,
		TypeBool,
		SliceOf(TypeString),
		SliceOf(SliceOf(TypeInt)),
		PointerTo(TypeInt),
		PointerTo(SliceOf(TypeString)),
		SliceOf(PointerTo(TypeInt)),
		MapOf(TypeString, TypeInt),
		MapOf(TypeString, SliceOf(TypeBool)),
		NamedType("time", "Time"),
		NamedType("uuid", "UUID"),
		{Kind: TypeKindArray, ArrayLen: 5, Elem: TypeInt},
	}

	for _, typ := range types {
		t.Run(typ.String(), func(t *testing.T) {
			str := typ.String()
			parsed, err := ParseTypeString(str)
			require.NoError(t, err)

			// Verify the round-trip produces the same string
			assert.Equal(t, str, parsed.String())
		})
	}
}

func TestSchemaYAMLFormat(t *testing.T) {
	t.Parallel()

	// Test that the YAML output format is clean and readable
	schema := &TypeSchema{
		Models: map[string]*Model{
			"Person": {
				Name: "Person",
				Fields: []*Field{
					{Name: "name", Type: TypeString, Required: true},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := WriteSchema(&buf, schema)
	require.NoError(t, err)

	output := buf.String()

	// Should have 2-space indentation
	lines := strings.Split(output, "\n")
	foundIndent := false
	for _, line := range lines {
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") {
			foundIndent = true
			break
		}
	}
	assert.True(t, foundIndent, "YAML should use 2-space indentation")

	// Should start with schema comment
	assert.True(t, strings.HasPrefix(output, "# yaml-language-server:"))
}
