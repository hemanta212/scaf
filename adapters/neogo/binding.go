package neogo

import (
	"fmt"
	"strings"

	"github.com/rlch/scaf"
	golang "github.com/rlch/scaf/language/go"
)

// Binding implements golang.Binding for neogo.
// It generates function bodies that execute Cypher queries using neogo's driver.
type Binding struct{}

// NewBinding creates a new neogo binding.
func NewBinding() *Binding {
	return &Binding{}
}

// Name returns the binding identifier.
func (b *Binding) Name() string {
	return scaf.AdapterNeogo
}

// Imports returns the import paths needed by generated code.
func (b *Binding) Imports() []string {
	return []string{
		"context",
		"github.com/rlch/neogo",
	}
}

// PrependParams returns ctx and db as the first params for all functions.
func (b *Binding) PrependParams() []golang.BindingParam {
	return []golang.BindingParam{
		{Name: "ctx", Type: "context.Context"},
		{Name: "db", Type: "neogo.Driver"},
	}
}

// ReturnsError returns true - neogo functions always return error.
func (b *Binding) ReturnsError() bool {
	return true
}

// ReceiverType returns empty - neogo generates standalone functions, not methods.
func (b *Binding) ReceiverType() string {
	return ""
}

// GenerateBody generates the function body for executing a Cypher query.
//
// Generated code pattern (single row, struct return with params):
//
//	result := &getUserResult{}
//	err := db.Exec().
//		Cypher(`MATCH (u:User {id: $userId}) RETURN u.name AS name, u.age AS age`).
//		RunWithParams(ctx, map[string]any{"userId": userId}, "name", &result.Name, "age", &result.Age)
//	if err != nil {
//		return nil, err
//	}
//	return result, nil
//
// Generated code pattern (slice, struct return with params):
//
//	var results []*getUserResult
//	err := db.Exec().
//		Cypher(`MATCH (u:User) RETURN u.name AS name, u.age AS age`).
//		StreamWithParams(ctx, map[string]any{}, func(r neogo.Result) error {
//			row := &getUserResult{}
//			if err := r.Read("name", &row.Name, "age", &row.Age); err != nil {
//				return err
//			}
//			results = append(results, row)
//			return nil
//		})
//	if err != nil {
//		return nil, err
//	}
//	return results, nil
//
// Generated code pattern (single row, individual returns with params):
//
//	var name string
//	var age int
//	err := db.Exec().
//		Cypher(`MATCH (u:User {id: $userId}) RETURN u.name AS name, u.age AS age`).
//		RunWithParams(ctx, map[string]any{"userId": userId}, "name", &name, "age", &age)
//	if err != nil {
//		return "", 0, err
//	}
//	return name, age, nil
//
// Generated code pattern (no params):
//
//	var count int64
//	err := db.Exec().
//		Cypher(`MATCH (u:User) RETURN count(u) AS count`).
//		Run(ctx, "count", &count)
//	if err != nil {
//		return 0, err
//	}
//	return count, nil
func (b *Binding) GenerateBody(ctx *golang.BodyContext) (string, error) {
	sig := ctx.Signature
	if sig == nil {
		return "panic(\"no signature\")", nil
	}

	// Handle slice returns differently
	if sig.ReturnsSlice {
		if sig.ResultStruct != "" {
			return b.generateSliceStructBody(ctx)
		}
		return b.generateSliceIndividualBody(ctx)
	}

	// Single row returns
	if sig.ResultStruct != "" {
		return b.generateStructBody(ctx)
	}

	return b.generateIndividualBody(ctx)
}

// generateStructBody generates function body that returns a struct pointer.
func (b *Binding) generateStructBody(ctx *golang.BodyContext) (string, error) {
	sig := ctx.Signature
	queryParams := ctx.QueryParams

	var sb strings.Builder

	// Declare result struct
	fmt.Fprintf(&sb, "result := &%s{}\n", sig.ResultStruct)

	// Build the Cypher query execution
	if sig.ReturnsError {
		sb.WriteString("err := ")
	}

	fmt.Fprintf(&sb, "db.Exec().\n\tCypher(`%s`).\n\t", strings.TrimSpace(ctx.Query))

	// Use RunWithParams if we have query params, otherwise Run
	if len(queryParams) > 0 {
		sb.WriteString("RunWithParams(ctx, map[string]any{")
		for i, param := range queryParams {
			if i > 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "%q: %s", param.Name, param.Name)
		}
		sb.WriteString("}")
	} else {
		sb.WriteString("Run(ctx")
	}

	// Add return bindings - bind to struct fields
	for _, ret := range sig.Returns {
		columnName := ret.ColumnName
		if columnName == "" {
			columnName = ret.Name
		}
		// Convert db field name to Go exported field name (e.g., "name" -> "Name")
		fieldName := toExportedFieldName(ret.Name)
		fmt.Fprintf(&sb, ", %q, &result.%s", columnName, fieldName)
	}
	sb.WriteString(")")

	// Handle error if the signature returns error
	if sig.ReturnsError {
		sb.WriteString("\nif err != nil {\n\treturn nil, err\n}")
	}

	// Return statement
	sb.WriteString("\nreturn result")
	if sig.ReturnsError {
		sb.WriteString(", nil")
	}

	return sb.String(), nil
}

// generateIndividualBody generates function body that returns individual values.
func (b *Binding) generateIndividualBody(ctx *golang.BodyContext) (string, error) {
	sig := ctx.Signature
	queryParams := ctx.QueryParams

	var sb strings.Builder

	// Declare result variables
	for _, ret := range sig.Returns {
		varName := toLocalName(ret.Name)
		fmt.Fprintf(&sb, "var %s %s\n", varName, ret.Type)
	}

	// Build the Cypher query execution
	if sig.ReturnsError {
		sb.WriteString("err := ")
	}

	fmt.Fprintf(&sb, "db.Exec().\n\tCypher(`%s`).\n\t", strings.TrimSpace(ctx.Query))

	// Use RunWithParams if we have query params, otherwise Run
	if len(queryParams) > 0 {
		sb.WriteString("RunWithParams(ctx, map[string]any{")
		for i, param := range queryParams {
			if i > 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "%q: %s", param.Name, param.Name)
		}
		sb.WriteString("}")
	} else {
		sb.WriteString("Run(ctx")
	}

	// Add return bindings (use ColumnName for the db column, Name for the Go variable)
	for _, ret := range sig.Returns {
		varName := toLocalName(ret.Name)
		columnName := ret.ColumnName
		if columnName == "" {
			columnName = ret.Name
		}
		fmt.Fprintf(&sb, ", %q, &%s", columnName, varName)
	}
	sb.WriteString(")")

	// Handle error if the signature returns error
	if sig.ReturnsError {
		sb.WriteString("\nif err != nil {\n\treturn ")
		if len(sig.Returns) > 0 {
			b.writeZeroReturns(&sb, sig.Returns)
			sb.WriteString(", err\n}")
		} else {
			sb.WriteString("err\n}")
		}
	}

	// Return statement
	sb.WriteString("\nreturn ")
	for i, ret := range sig.Returns {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(toLocalName(ret.Name))
	}
	if sig.ReturnsError {
		if len(sig.Returns) > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("nil")
	}

	return sb.String(), nil
}

// generateSliceStructBody generates function body that returns a slice of struct pointers.
// For multiple return fields, we collect individual slices then zip them into structs.
func (b *Binding) generateSliceStructBody(ctx *golang.BodyContext) (string, error) {
	sig := ctx.Signature
	queryParams := ctx.QueryParams

	var sb strings.Builder

	// Declare temporary slices for each return field
	// Use "rows" prefix to avoid shadowing query parameters
	for _, ret := range sig.Returns {
		varName := "rows" + toExportedFieldName(ret.Name)
		fmt.Fprintf(&sb, "var %s []%s\n", varName, ret.Type)
	}

	// Build the Cypher query execution
	if sig.ReturnsError {
		sb.WriteString("err := ")
	}

	fmt.Fprintf(&sb, "db.Exec().\n\tCypher(`%s`).\n\t", strings.TrimSpace(ctx.Query))

	// Use RunWithParams if we have query params, otherwise Run
	if len(queryParams) > 0 {
		sb.WriteString("RunWithParams(ctx, map[string]any{")
		for i, param := range queryParams {
			if i > 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "%q: %s", param.Name, param.Name)
		}
		sb.WriteString("}")
	} else {
		sb.WriteString("Run(ctx")
	}

	// Add return bindings - bind to individual slices
	for _, ret := range sig.Returns {
		varName := "rows" + toExportedFieldName(ret.Name)
		columnName := ret.ColumnName
		if columnName == "" {
			columnName = ret.Name
		}
		fmt.Fprintf(&sb, ", %q, &%s", columnName, varName)
	}
	sb.WriteString(")")

	// Handle error
	if sig.ReturnsError {
		sb.WriteString("\nif err != nil {\n\treturn nil, err\n}")
	}

	// Zip slices into result structs
	// Use first field to determine length
	firstField := "rows" + toExportedFieldName(sig.Returns[0].Name)
	fmt.Fprintf(&sb, "\nresults := make([]*%s, len(%s))\n", sig.ResultStruct, firstField)
	fmt.Fprintf(&sb, "for i := range %s {\n", firstField)
	fmt.Fprintf(&sb, "\tresults[i] = &%s{\n", sig.ResultStruct)
	for _, ret := range sig.Returns {
		varName := "rows" + toExportedFieldName(ret.Name)
		fieldName := toExportedFieldName(ret.Name)
		fmt.Fprintf(&sb, "\t\t%s: %s[i],\n", fieldName, varName)
	}
	sb.WriteString("\t}\n}")

	// Return statement
	sb.WriteString("\nreturn results")
	if sig.ReturnsError {
		sb.WriteString(", nil")
	}

	return sb.String(), nil
}

// generateSliceIndividualBody generates function body that returns slices of individual values.
// neogo's Run automatically handles slices when you pass a pointer to a slice.
func (b *Binding) generateSliceIndividualBody(ctx *golang.BodyContext) (string, error) {
	sig := ctx.Signature
	queryParams := ctx.QueryParams

	var sb strings.Builder

	// Declare result slices
	// Use "rows" prefix to avoid shadowing query parameters
	for _, ret := range sig.Returns {
		varName := "rows" + toExportedFieldName(ret.Name)
		fmt.Fprintf(&sb, "var %s []%s\n", varName, ret.Type)
	}

	// Build the Cypher query execution
	if sig.ReturnsError {
		sb.WriteString("err := ")
	}

	fmt.Fprintf(&sb, "db.Exec().\n\tCypher(`%s`).\n\t", strings.TrimSpace(ctx.Query))

	// Use RunWithParams if we have query params, otherwise Run
	if len(queryParams) > 0 {
		sb.WriteString("RunWithParams(ctx, map[string]any{")
		for i, param := range queryParams {
			if i > 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "%q: %s", param.Name, param.Name)
		}
		sb.WriteString("}")
	} else {
		sb.WriteString("Run(ctx")
	}

	// Add return bindings (use ColumnName for the db column, varName for the Go variable)
	for _, ret := range sig.Returns {
		varName := "rows" + toExportedFieldName(ret.Name)
		columnName := ret.ColumnName
		if columnName == "" {
			columnName = ret.Name
		}
		fmt.Fprintf(&sb, ", %q, &%s", columnName, varName)
	}
	sb.WriteString(")")

	// Handle error if the signature returns error
	if sig.ReturnsError {
		sb.WriteString("\nif err != nil {\n\treturn ")
		if len(sig.Returns) > 0 {
			for i := range sig.Returns {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString("nil")
			}
			sb.WriteString(", err\n}")
		} else {
			sb.WriteString("err\n}")
		}
	}

	// Return statement
	sb.WriteString("\nreturn ")
	for i, ret := range sig.Returns {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("rows" + toExportedFieldName(ret.Name))
	}
	if sig.ReturnsError {
		if len(sig.Returns) > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("nil")
	}

	return sb.String(), nil
}

// writeZeroReturns writes zero values for all return types.
func (b *Binding) writeZeroReturns(sb *strings.Builder, returns []golang.BindingReturn) {
	for i, ret := range returns {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(zeroValue(ret.Type))
	}
}

// zeroValue returns the zero value literal for a Go type.
func zeroValue(typ string) string {
	switch typ {
	case "string":
		return `""`
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "byte", "rune":
		return "0"
	case "bool":
		return "false"
	default:
		// Pointers, slices, maps, interfaces, any
		if strings.HasPrefix(typ, "*") ||
			strings.HasPrefix(typ, "[]") ||
			strings.HasPrefix(typ, "map[") ||
			typ == "any" || typ == "error" {
			return "nil"
		}
		// Struct or unknown - use zero value syntax
		return typ + "{}"
	}
}

// toLocalName converts a return name to a local variable name.
func toLocalName(name string) string {
	if name == "" {
		return "result"
	}
	return name
}

// toExportedFieldName converts a database field name to an exported Go field name.
// Examples: "name" -> "Name", "user_id" -> "UserID", "id" -> "ID"
func toExportedFieldName(name string) string {
	if name == "" {
		return ""
	}

	// Handle snake_case
	if strings.Contains(name, "_") {
		return snakeToPascal(name)
	}

	// Check if it's a known acronym (case insensitive)
	if isCommonAcronym(strings.ToUpper(name)) {
		return strings.ToUpper(name)
	}

	// Just capitalize first letter for camelCase or lowercase names
	runes := []rune(name)
	runes[0] = toUpper(runes[0])
	return string(runes)
}

// snakeToPascal converts snake_case to PascalCase.
func snakeToPascal(s string) string {
	parts := strings.Split(s, "_")
	var result strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		// Handle common acronyms
		upper := strings.ToUpper(part)
		if isCommonAcronym(upper) {
			result.WriteString(upper)
		} else {
			runes := []rune(part)
			runes[0] = toUpper(runes[0])
			result.WriteString(string(runes))
		}
	}
	return result.String()
}

// toUpper converts a rune to uppercase.
func toUpper(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 32
	}
	return r
}

// isCommonAcronym returns true if the string is a common acronym.
func isCommonAcronym(s string) bool {
	switch s {
	case "ID", "URL", "API", "HTTP", "JSON", "XML", "SQL", "UUID", "DB":
		return true
	}
	return false
}

// Register the binding on package init.
//
//nolint:gochecknoinits // Registration pattern requires init.
func init() {
	golang.RegisterBinding(NewBinding())
}
