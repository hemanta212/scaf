package scaf

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Type parsing errors.
var (
	ErrEmptyTypeString    = errors.New("empty type string")
	ErrInvalidArrayType   = errors.New("invalid array type")
	ErrInvalidMapType     = errors.New("invalid map type")
	ErrUnrecognizedType   = errors.New("unrecognized type")
)

// TypeKind represents the kind of a type.
type TypeKind string

// Type kind constants.
const (
	TypeKindPrimitive TypeKind = "primitive" // string, int, bool, float64, etc.
	TypeKindSlice     TypeKind = "slice"     // []T
	TypeKindArray     TypeKind = "array"     // [N]T
	TypeKindMap       TypeKind = "map"       // map[K]V
	TypeKindPointer   TypeKind = "pointer"   // *T
	TypeKindNamed     TypeKind = "named"     // time.Time, uuid.UUID, etc.
)

// Type represents a type in the schema.
// This is a recursive structure that can represent complex types like []map[string]*Person.
type Type struct {
	// Kind is the category of this type.
	Kind TypeKind

	// Name is the type name.
	// For primitives: "string", "int", "bool", "float64", etc.
	// For named types: "Time", "UUID", etc.
	Name string

	// Package is the package path for named types (e.g., "time", "github.com/google/uuid").
	// Empty for primitives.
	Package string

	// Elem is the element type for slices, arrays, pointers, and map values.
	Elem *Type

	// Key is the key type for maps.
	Key *Type

	// ArrayLen is the length for array types.
	ArrayLen int
}

// String returns a Go-style string representation of the type.
func (t *Type) String() string {
	if t == nil {
		return ""
	}

	switch t.Kind {
	case TypeKindPrimitive:
		return t.Name
	case TypeKindSlice:
		return "[]" + t.Elem.String()
	case TypeKindArray:
		return "[" + strconv.Itoa(t.ArrayLen) + "]" + t.Elem.String()
	case TypeKindMap:
		return "map[" + t.Key.String() + "]" + t.Elem.String()
	case TypeKindPointer:
		return "*" + t.Elem.String()
	case TypeKindNamed:
		if t.Package != "" {
			return t.Package + "." + t.Name
		}

		return t.Name
	default:
		return t.Name
	}
}

// ParseTypeString parses a Go-style type string into a Type.
// Supports: string, int, int64, float64, bool, []T, [N]T, *T, map[K]V, pkg.Name
//
// Examples:
//
//	"string"           -> TypeKindPrimitive, Name="string"
//	"[]string"         -> TypeKindSlice, Elem=string
//	"[5]int"           -> TypeKindArray, ArrayLen=5, Elem=int
//	"*int"             -> TypeKindPointer, Elem=int
//	"map[string]int"   -> TypeKindMap, Key=string, Elem=int
//	"time.Time"        -> TypeKindNamed, Package="time", Name="Time"
//	"[]map[string]*int" -> nested types
func ParseTypeString(s string) (*Type, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, ErrEmptyTypeString
	}

	return parseType(s)
}

// parseType recursively parses a type string.
func parseType(s string) (*Type, error) {
	// Check for slice: []T
	if strings.HasPrefix(s, "[]") {
		elem, err := parseType(s[2:])
		if err != nil {
			return nil, err
		}
		return SliceOf(elem), nil
	}

	// Check for array: [N]T
	if strings.HasPrefix(s, "[") {
		closeIdx := strings.Index(s, "]")
		if closeIdx == -1 {
			return nil, fmt.Errorf("%w: %s", ErrInvalidArrayType, s)
		}

		lenStr := s[1:closeIdx]
		if lenStr == "" {
			return nil, fmt.Errorf("%w: %s", ErrInvalidArrayType, s)
		}

		arrayLen, err := strconv.Atoi(lenStr)
		if err != nil {
			return nil, fmt.Errorf("invalid array length %q: %w", lenStr, err)
		}

		elem, err := parseType(s[closeIdx+1:])
		if err != nil {
			return nil, err
		}

		return &Type{Kind: TypeKindArray, ArrayLen: arrayLen, Elem: elem}, nil
	}

	// Check for pointer: *T
	if strings.HasPrefix(s, "*") {
		elem, err := parseType(s[1:])
		if err != nil {
			return nil, err
		}
		return PointerTo(elem), nil
	}

	// Check for map: map[K]V
	if strings.HasPrefix(s, "map[") {
		// Find the closing bracket for the key type
		depth := 0
		keyEnd := -1

		for i := 4; i < len(s); i++ {
			switch s[i] {
			case '[':
				depth++
			case ']':
				if depth == 0 {
					keyEnd = i
				} else {
					depth--
				}
			}

			if keyEnd != -1 {
				break
			}
		}

		if keyEnd == -1 {
			return nil, fmt.Errorf("%w: %s", ErrInvalidMapType, s)
		}

		keyStr := s[4:keyEnd]
		valueStr := s[keyEnd+1:]

		key, err := parseType(keyStr)
		if err != nil {
			return nil, fmt.Errorf("invalid map key type: %w", err)
		}

		value, err := parseType(valueStr)
		if err != nil {
			return nil, fmt.Errorf("invalid map value type: %w", err)
		}

		return MapOf(key, value), nil
	}

	// Check for named type: pkg.Name
	if idx := strings.LastIndex(s, "."); idx > 0 {
		pkg := s[:idx]
		name := s[idx+1:]

		if isValidTypeIdentifier(pkg) && isValidTypeIdentifier(name) {
			return NamedType(pkg, name), nil
		}
	}

	// Check for primitive types
	if isPrimitiveType(s) {
		return &Type{Kind: TypeKindPrimitive, Name: s}, nil
	}

	// Treat as named type without package (user-defined type in same package)
	if isValidTypeIdentifier(s) {
		return NamedType("", s), nil
	}

	return nil, fmt.Errorf("%w: %s", ErrUnrecognizedType, s)
}

// isPrimitiveType returns true if s is a Go primitive type name.
func isPrimitiveType(s string) bool {
	switch s {
	case "bool", "string",
		"int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"byte", "rune",
		"float32", "float64",
		"complex64", "complex128",
		"any", "error":
		return true
	}
	return false
}

// isValidTypeIdentifier returns true if s is a valid Go identifier.
func isValidTypeIdentifier(s string) bool {
	if s == "" {
		return false
	}

	for i, r := range s {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return false
			}
		} else {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
				return false
			}
		}
	}

	return true
}

// Primitive type constructors for convenience.
var (
	TypeString  = &Type{Kind: TypeKindPrimitive, Name: "string"}
	TypeInt     = &Type{Kind: TypeKindPrimitive, Name: "int"}
	TypeInt64   = &Type{Kind: TypeKindPrimitive, Name: "int64"}
	TypeFloat64 = &Type{Kind: TypeKindPrimitive, Name: "float64"}
	TypeBool    = &Type{Kind: TypeKindPrimitive, Name: "bool"}
)

// SliceOf creates a slice type.
func SliceOf(elem *Type) *Type {
	return &Type{Kind: TypeKindSlice, Elem: elem}
}

// PointerTo creates a pointer type.
func PointerTo(elem *Type) *Type {
	return &Type{Kind: TypeKindPointer, Elem: elem}
}

// MapOf creates a map type.
func MapOf(key, value *Type) *Type {
	return &Type{Kind: TypeKindMap, Key: key, Elem: value}
}

// NamedType creates a named type.
func NamedType(pkg, name string) *Type {
	return &Type{Kind: TypeKindNamed, Package: pkg, Name: name}
}
