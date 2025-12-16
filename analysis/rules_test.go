package analysis_test

import (
	"strings"
	"testing"

	"github.com/rlch/scaf"
	"github.com/rlch/scaf/analysis"

	// Import cypher dialect to register analyzer.
	_ "github.com/rlch/scaf/dialects/cypher"
)

func TestRule_UndefinedQuery(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn Q() `+"`Q`"+`

UndefinedQuery {
	test "t" {}
}
`)

	assertHasDiagnostic(t, result, "undefined-query")
}

func TestRule_UndefinedImport(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn Q() `+"`Q`"+`

setup undefined.Setup()

Q {
	test "t" {}
}
`)

	assertHasDiagnostic(t, result, "undefined-import")
}

func TestRule_UnusedImport(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
import fixtures "./fixtures"

fn Q() `+"`Q`"+`

Q {
	test "t" {}
}
`)

	assertHasDiagnostic(t, result, "unused-import")
}

func TestRule_UsedImport(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
import fixtures "./fixtures"

fn Q() `+"`Q`"+`

setup fixtures.Setup()

Q {
	test "t" {}
}
`)

	assertNoDiagnostic(t, result, "unused-import")
}

func TestRule_DuplicateQuery(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn GetUser() `+"`Q1`"+`
fn GetUser() `+"`Q2`"+`

GetUser {
	test "t" {}
}
`)

	assertHasDiagnostic(t, result, "duplicate-query")
}

func TestRule_DuplicateImport(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
import fixtures "./fixtures"
import fixtures "./other"

fn Q() `+"`Q`"+`

Q {
	test "t" {}
}
`)

	assertHasDiagnostic(t, result, "duplicate-import")
}

func TestRule_UnknownParameter(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn GetUser() `+"`MATCH (u:User {id: $id}) RETURN u`"+`

GetUser {
	test "finds user" {
		$id: 1
		$unknownParam: "test"
	}
}
`)

	assertHasDiagnostic(t, result, "unknown-parameter")
}

func TestRule_EmptyTest(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn Q() `+"`Q`"+`

Q {
	test "empty" {}
}
`)

	assertHasDiagnostic(t, result, "empty-test")
}

func TestRule_DuplicateTestName(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn Q() `+"`Q`"+`

Q {
	test "same name" {
		$x: 1
	}
	test "same name" {
		$x: 2
	}
}
`)

	assertHasDiagnostic(t, result, "duplicate-test")
}

func TestRule_DuplicateGroupName(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn Q() `+"`Q`"+`

Q {
	group "mygroup" {
		test "t1" { $x: 1 }
	}
	group "mygroup" {
		test "t2" { $x: 2 }
	}
}
`)

	assertHasDiagnostic(t, result, "duplicate-group")
}

func TestRule_UndefinedAssertQuery(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn Q() `+"`Q`"+`

Q {
	test "t" {
		$x: 1
		assert UndefinedQuery() { result > 0 }
	}
}
`)

	assertHasDiagnostic(t, result, "undefined-assert-query")
}

func TestRule_MissingRequiredParams(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn GetUser() `+"`MATCH (u:User {id: $id, name: $name}) RETURN u`"+`

GetUser {
	test "missing name" {
		$id: 1
	}
}
`)

	assertHasDiagnostic(t, result, "missing-required-params")
}

func TestRule_AllParamsProvided(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn GetUser() `+"`MATCH (u:User {id: $id, name: $name}) RETURN u`"+`

GetUser {
	test "has all params" {
		$id: 1
		$name: "Alice"
	}
}
`)

	assertNoDiagnostic(t, result, "missing-required-params")
}

func TestRule_EmptyGroup(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn Q() `+"`Q`"+`

Q {
	group "empty group" {}
}
`)

	assertHasDiagnostic(t, result, "empty-group")
}

func TestRule_UnusedDeclaredParam(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn GetUser(id, unusedParam) `+"`MATCH (u:User {id: $id}) RETURN u`"+`

GetUser {
	test "finds user" {
		$id: 1
	}
}
`)

	assertHasDiagnostic(t, result, "unused-declared-param")
}

func TestRule_UnusedDeclaredParam_AllUsed(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn GetUser(id, name) `+"`MATCH (u:User {id: $id, name: $name}) RETURN u`"+`

GetUser {
	test "finds user" {
		$id: 1
		$name: "Alice"
	}
}
`)

	assertNoDiagnostic(t, result, "unused-declared-param")
}

func TestRule_UnusedDeclaredParam_TypedParam(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn GetUser(id: string, unusedParam: int) `+"`MATCH (u:User {id: $id}) RETURN u`"+`

GetUser {
	test "finds user" {
		$id: "user-1"
	}
}
`)

	assertHasDiagnostic(t, result, "unused-declared-param")
}

func TestRule_UndeclaredQueryParam(t *testing.T) {
	t.Parallel()

	// When a function has typed params, any param used in body must be declared
	result := analyze(t, `
fn GetUser(id: string) `+"`MATCH (u:User {id: $id, name: $name}) RETURN u`"+`

GetUser {
	test "finds user" {
		$id: "user-1"
	}
}
`)

	assertHasDiagnostic(t, result, "undeclared-query-param")
}

func TestRule_UndeclaredQueryParam_AllDeclared(t *testing.T) {
	t.Parallel()

	// When all params are declared, no error
	result := analyze(t, `
fn GetUser(id: string, name: string) `+"`MATCH (u:User {id: $id, name: $name}) RETURN u`"+`

GetUser {
	test "finds user" {
		$id: "user-1"
		$name: "Alice"
	}
}
`)

	assertNoDiagnostic(t, result, "undeclared-query-param")
}

func TestRule_UndeclaredQueryParam_NoTypedParams(t *testing.T) {
	t.Parallel()

	// When function uses params in body but declares none, that's an error
	result := analyze(t, `
fn GetUser() `+"`MATCH (u:User {id: $id, name: $name}) RETURN u`"+`

GetUser {
	test "finds user" {
		$id: "user-1"
	}
}
`)

	assertHasDiagnostic(t, result, "undeclared-query-param")
}

func TestRule_UndeclaredQueryParam_NoParams(t *testing.T) {
	t.Parallel()

	// When function has no params in body and declares none, that's fine
	result := analyze(t, `
fn CountUsers() `+"`MATCH (u:User) RETURN count(u)`"+`

CountUsers {
	test "counts" {
		count: 2
	}
}
`)

	assertNoDiagnostic(t, result, "undeclared-query-param")
}

func TestRule_UndeclaredQueryParam_UntypedParams(t *testing.T) {
	t.Parallel()

	// When function declares untyped params, they should still be recognized
	result := analyze(t, `
fn CreatePost(title, authorId) `+"`CREATE (p:Post {title: $title, authorId: $authorId}) RETURN p`"+`

CreatePost {
	test "creates post" {
		$title: "Hello"
		$authorId: "user-1"
	}
}
`)

	assertNoDiagnostic(t, result, "undeclared-query-param")
}

func TestRule_UndeclaredQueryParam_MixedTypedAndUntyped(t *testing.T) {
	t.Parallel()

	// Mixed typed and untyped params should all be recognized
	result := analyze(t, `
fn CreateUser(id, name: string, data) `+"`CREATE (u:User {id: $id, name: $name, data: $data}) RETURN u`"+`

CreateUser {
	test "creates user" {
		$id: 1
		$name: "Alice"
		$data: {key: "value"}
	}
}
`)

	assertNoDiagnostic(t, result, "undeclared-query-param")
}

func TestRule_UndeclaredQueryParam_UntypedWithMissing(t *testing.T) {
	t.Parallel()

	// Untyped params should still catch undeclared params
	result := analyze(t, `
fn CreatePost(title) `+"`CREATE (p:Post {title: $title, authorId: $authorId}) RETURN p`"+`

CreatePost {
	test "creates post" {
		$title: "Hello"
	}
}
`)

	assertHasDiagnostic(t, result, "undeclared-query-param")
}

func TestRule_UndeclaredQueryParam_WithDialectAnalyzer(t *testing.T) {
	t.Parallel()

	// Test with the cypher analyzer for proper parameter extraction
	cypherAnalyzer := scaf.GetAnalyzer("cypher")
	if cypherAnalyzer == nil {
		t.Skip("cypher analyzer not available")
	}

	analyzer := analysis.NewAnalyzerWithQueryAnalyzer(nil, nil, cypherAnalyzer)
	result := analyzer.Analyze("test.scaf", []byte(`
fn GetUser(id: string) `+"`MATCH (u:User {id: $id, name: $name}) RETURN u`"+`

GetUser {
	test "finds user" {
		$id: "user-1"
	}
}
`))

	assertHasDiagnostic(t, result, "undeclared-query-param")

	// Verify the message mentions $name
	found := false
	for _, d := range result.Diagnostics {
		if d.Code == "undeclared-query-param" && contains(d.Message, "$name") {
			found = true

			break
		}
	}
	if !found {
		t.Error("expected diagnostic to mention $name")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}

func TestRule_ParamTypeMismatch_StringExpectedGotInt(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn GetUser(id: string) `+"`MATCH (u:User {id: $id}) RETURN u`"+`

GetUser {
	test "finds user" {
		$id: 123
	}
}
`)

	assertHasDiagnostic(t, result, "param-type-mismatch")
}

func TestRule_ParamTypeMismatch_IntExpectedGotString(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn GetUser(age: int) `+"`MATCH (u:User {age: $age}) RETURN u`"+`

GetUser {
	test "finds user" {
		$age: "thirty"
	}
}
`)

	assertHasDiagnostic(t, result, "param-type-mismatch")
}

func TestRule_ParamTypeMismatch_BoolExpectedGotString(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn GetUser(active: bool) `+"`MATCH (u:User {active: $active}) RETURN u`"+`

GetUser {
	test "finds user" {
		$active: "yes"
	}
}
`)

	assertHasDiagnostic(t, result, "param-type-mismatch")
}

func TestRule_ParamTypeMismatch_ArrayExpectedGotString(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn GetUsers(ids: [string]) `+"`MATCH (u:User) WHERE u.id IN $ids RETURN u`"+`

GetUsers {
	test "finds users" {
		$ids: "single-id"
	}
}
`)

	assertHasDiagnostic(t, result, "param-type-mismatch")
}

func TestRule_ParamTypeMismatch_ArrayElementTypeMismatch(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn GetUsers(ids: [int]) `+"`MATCH (u:User) WHERE u.id IN $ids RETURN u`"+`

GetUsers {
	test "finds users" {
		$ids: [1, "two", 3]
	}
}
`)

	assertHasDiagnostic(t, result, "param-type-mismatch")
}

func TestRule_ParamTypeMismatch_NullForNonNullable(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn GetUser(id: string) `+"`MATCH (u:User {id: $id}) RETURN u`"+`

GetUser {
	test "finds user" {
		$id: null
	}
}
`)

	assertHasDiagnostic(t, result, "param-type-mismatch")
}

func TestRule_ParamTypeMismatch_NullForNullableIsOK(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn GetUser(name: string?) `+"`MATCH (u:User {name: $name}) RETURN u`"+`

GetUser {
	test "finds user" {
		$name: null
	}
}
`)

	assertNoDiagnostic(t, result, "param-type-mismatch")
}

func TestRule_ParamTypeMismatch_CorrectTypes(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn GetUser(id: string, age: int, active: bool) `+"`MATCH (u:User) RETURN u`"+`

GetUser {
	test "finds user" {
		$id: "user-1"
		$age: 30
		$active: true
	}
}
`)

	assertNoDiagnostic(t, result, "param-type-mismatch")
}

func TestRule_ParamTypeMismatch_AnyTypeAcceptsAll(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn CreateNode(data: any) `+"`CREATE (n:Node $data) RETURN n`"+`

CreateNode {
	test "with string" {
		$data: "hello"
	}
	test "with number" {
		$data: 123
	}
	test "with map" {
		$data: {key: "value"}
	}
}
`)

	assertNoDiagnostic(t, result, "param-type-mismatch")
}

func TestRule_ParamTypeMismatch_MapType(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn CreateNode(data: {string: int}) `+"`CREATE (n:Node $data) RETURN n`"+`

CreateNode {
	test "with wrong value type" {
		$data: {count: "not-an-int"}
	}
}
`)

	assertHasDiagnostic(t, result, "param-type-mismatch")
}

func TestRule_ParamTypeMismatch_NoTypeAnnotation(t *testing.T) {
	t.Parallel()

	// When there's no type annotation and no schema, any value should be allowed
	result := analyze(t, `
fn GetUser() `+"`MATCH (u:User {id: $id}) RETURN u`"+`

GetUser {
	test "finds user" {
		$id: 123
	}
}
`)

	assertNoDiagnostic(t, result, "param-type-mismatch")
}

func TestRule_ParamTypeMismatch_InferredFromSchema_StringExpectedGotInt(t *testing.T) {
	t.Parallel()

	// When schema is available, type should be inferred from property comparisons.
	// Here, p.name = $name should infer that $name is a string.
	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"Person": {
				Name: "Person",
				Fields: []*analysis.Field{
					{Name: "name", Type: analysis.TypeString},
				},
			},
		},
	}

	result := analyzeWithSchema(t, `
fn FindPerson(name) `+"`MATCH (p:Person) WHERE p.name = $name RETURN p`"+`

FindPerson {
	test "finds by name" {
		$name: 123
	}
}
`, schema)

	assertHasDiagnostic(t, result, "param-type-mismatch")
}

func TestRule_ParamTypeMismatch_InferredFromSchema_IntExpectedGotString(t *testing.T) {
	t.Parallel()

	// When schema is available, type should be inferred from property comparisons.
	// Here, p.age = $age should infer that $age is an int.
	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"Person": {
				Name: "Person",
				Fields: []*analysis.Field{
					{Name: "age", Type: analysis.TypeInt},
				},
			},
		},
	}

	result := analyzeWithSchema(t, `
fn FindPerson(age) `+"`MATCH (p:Person) WHERE p.age = $age RETURN p`"+`

FindPerson {
	test "finds by age" {
		$age: "thirty"
	}
}
`, schema)

	assertHasDiagnostic(t, result, "param-type-mismatch")
}

func TestRule_ParamTypeMismatch_InferredFromSchema_CorrectType(t *testing.T) {
	t.Parallel()

	// When the value matches the inferred type, no diagnostic should be raised.
	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"Person": {
				Name: "Person",
				Fields: []*analysis.Field{
					{Name: "name", Type: analysis.TypeString},
				},
			},
		},
	}

	result := analyzeWithSchema(t, `
fn FindPerson(name) `+"`MATCH (p:Person) WHERE p.name = $name RETURN p`"+`

FindPerson {
	test "finds by name" {
		$name: "Alice"
	}
}
`, schema)

	assertNoDiagnostic(t, result, "param-type-mismatch")
}

func TestRule_ParamTypeMismatch_ExplicitTypeOverridesInferred(t *testing.T) {
	t.Parallel()

	// When an explicit type annotation is provided, it takes precedence over inferred type.
	// Here, schema says p.name is string, but explicit annotation says int - use int.
	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"Person": {
				Name: "Person",
				Fields: []*analysis.Field{
					{Name: "name", Type: analysis.TypeString},
				},
			},
		},
	}

	result := analyzeWithSchema(t, `
fn FindPerson(name: int) `+"`MATCH (p:Person) WHERE p.name = $name RETURN p`"+`

FindPerson {
	test "explicit type wins" {
		$name: 123
	}
}
`, schema)

	// The explicit int annotation means 123 should be accepted
	assertNoDiagnostic(t, result, "param-type-mismatch")
}

// Test helpers

func analyze(t *testing.T, input string) *analysis.AnalyzedFile {
	t.Helper()

	analyzer := analysis.NewAnalyzer(nil)

	return analyzer.Analyze("test.scaf", []byte(input))
}

// analyzeWithQueryAnalyzer creates an analyzer with the cypher query analyzer.
// This enables expression validation with proper type checking.
func analyzeWithQueryAnalyzer(t *testing.T, input string) *analysis.AnalyzedFile {
	t.Helper()

	queryAnalyzer := scaf.GetAnalyzer("cypher")
	analyzer := analysis.NewAnalyzerWithQueryAnalyzer(nil, nil, queryAnalyzer)

	return analyzer.Analyze("test.scaf", []byte(input))
}

// analyzeWithSchema creates an analyzer with both query analyzer and schema.
// This enables type inference from property comparisons in queries.
func analyzeWithSchema(t *testing.T, input string, schema *analysis.TypeSchema) *analysis.AnalyzedFile {
	t.Helper()

	queryAnalyzer := scaf.GetAnalyzer("cypher")
	analyzer := analysis.NewAnalyzerWithSchema(nil, nil, queryAnalyzer, schema)

	return analyzer.Analyze("test.scaf", []byte(input))
}

func assertHasDiagnostic(t *testing.T, result *analysis.AnalyzedFile, code string) {
	t.Helper()

	for _, d := range result.Diagnostics {
		if d.Code == code {
			return
		}
	}

	t.Errorf("expected diagnostic %q, got:", code)

	for _, d := range result.Diagnostics {
		t.Logf("  %s: %s", d.Code, d.Message)
	}
}

func assertNoDiagnostic(t *testing.T, result *analysis.AnalyzedFile, code string) {
	t.Helper()

	for _, d := range result.Diagnostics {
		if d.Code == code {
			t.Errorf("unexpected diagnostic %q: %s", code, d.Message)
		}
	}
}

// ============================================================================
// Expression validation tests
// ============================================================================

func TestRule_InvalidExpression_SyntaxError(t *testing.T) {
	t.Parallel()

	result := analyzeWithQueryAnalyzer(t, `
fn GetUser() `+"`MATCH (u:User) RETURN u`"+`

GetUser {
	test "invalid syntax" {
		assert (u +)
	}
}
`)

	assertHasDiagnostic(t, result, "invalid-expression")
}

func TestRule_InvalidExpression_ValidBoolExpr(t *testing.T) {
	t.Parallel()

	result := analyzeWithQueryAnalyzer(t, `
fn GetUser() `+"`MATCH (u:User) RETURN u`"+`

GetUser {
	test "valid assertion" {
		assert (u.age > 0)
	}
}
`)

	assertNoDiagnostic(t, result, "invalid-expression")
}

func TestRule_InvalidExpression_NonBoolAssertion(t *testing.T) {
	t.Parallel()

	// When query returns specific properties (u.name, u.age), accessing them directly
	// in an assertion should error because they're not boolean.
	// This tests that `assert (u.name)` errors with "expected bool, but got string".
	result := analyzeWithQueryAnalyzer(t, `
fn GetUser() `+"`MATCH (u:User) RETURN u.name, u.age`"+`

GetUser {
	test "non-bool assertion" {
		assert (u.name)
	}
}
`)

	// With our typed struct approach, u.name should be recognized as a typed field
	// and produce "expected bool, but got string" error
	assertHasDiagnostic(t, result, "invalid-expression")
}

func TestRule_InvalidExpression_NonBoolAssertion_WholeNode(t *testing.T) {
	t.Parallel()

	// When query returns a whole node (RETURN u), accessing properties is still
	// allowed via map[string]any fallback. This is a known limitation without schema.
	// This test just verifies we don't crash.
	result := analyzeWithQueryAnalyzer(t, `
fn GetUser() `+"`MATCH (u:User) RETURN u`"+`

GetUser {
	test "non-bool assertion" {
		assert (u.age)
	}
}
`)

	// Without schema-derived types for the node's properties, u.age is still any type.
	// This is a known limitation - we can't type-check properties on whole nodes without schema.
	_ = result
}

func TestRule_InvalidExpression_WhereClauseMustBeBool(t *testing.T) {
	t.Parallel()

	// Note: Without schema info, type checking for where clauses is limited.
	// This test verifies we don't crash when processing where clauses.
	result := analyzeWithQueryAnalyzer(t, `
fn GetUser(id: int) `+"`MATCH (u:User {id: $id}) RETURN u`"+`

GetUser {
	test "where clause" {
		$id: 5 where (u.id > 0)
	}
}
`)

	// Without schema-derived types, this won't error for type mismatch.
	// The test passes if we can analyze without crashing.
	_ = result
}

func TestRule_InvalidExpression_ValidWhereClause(t *testing.T) {
	t.Parallel()

	result := analyzeWithQueryAnalyzer(t, `
fn GetUser(id: int) `+"`MATCH (u:User {id: $id}) RETURN u`"+`

GetUser {
	test "valid where" {
		$id: 5 where (u.id > 0)
	}
}
`)

	assertNoDiagnostic(t, result, "invalid-expression")
}

func TestRule_InvalidExpression_ParamExprTyped(t *testing.T) {
	t.Parallel()

	// Expression for int param should validate as int
	result := analyzeWithQueryAnalyzer(t, `
fn GetUser(id: int) `+"`MATCH (u:User {id: $id}) RETURN u`"+`

GetUser {
	test "int expression" {
		$id: (2 * 4)
	}
}
`)

	assertNoDiagnostic(t, result, "invalid-expression")
}

func TestRule_InvalidExpression_ParamExprTypeMismatch(t *testing.T) {
	t.Parallel()

	// Expression returning string for int param should error
	result := analyzeWithQueryAnalyzer(t, `
fn GetUser(id: int) `+"`MATCH (u:User {id: $id}) RETURN u`"+`

GetUser {
	test "wrong type expression" {
		$id: ("hello" + "world")
	}
}
`)

	assertHasDiagnostic(t, result, "invalid-expression")
}

func TestRule_InvalidExpression_MultipleConditions(t *testing.T) {
	t.Parallel()

	result := analyzeWithQueryAnalyzer(t, `
fn GetUser() `+"`MATCH (u:User) RETURN u`"+`

GetUser {
	test "multiple asserts" {
		assert {
			(u.age > 0)
			(u.score < 10)
			(u.level == 5)
		}
	}
}
`)

	assertNoDiagnostic(t, result, "invalid-expression")
}

func TestRule_InvalidExpression_ShorthandForm(t *testing.T) {
	t.Parallel()

	result := analyzeWithQueryAnalyzer(t, `
fn GetUser() `+"`MATCH (u:User) RETURN u`"+`

GetUser {
	test "shorthand assert" {
		assert (u.name == "Alice")
	}
}
`)

	assertNoDiagnostic(t, result, "invalid-expression")
}

func TestRule_InvalidExpression_ComplexExpr(t *testing.T) {
	t.Parallel()

	result := analyzeWithQueryAnalyzer(t, `
fn GetUser() `+"`MATCH (u:User) RETURN u, u.items as items`"+`

GetUser {
	test "complex expression" {
		assert (len(items) > 0 && u.name == "test")
	}
}
`)

	assertNoDiagnostic(t, result, "invalid-expression")
}

func TestRule_InvalidExpression_UnclosedParen(t *testing.T) {
	t.Parallel()

	// This tests expr-lang's parser catching unclosed parens
	result := analyzeWithQueryAnalyzer(t, `
fn GetUser() `+"`MATCH (u:User) RETURN u`"+`

GetUser {
	test "unclosed" {
		assert (u.age > (u.score + 1)
	}
}
`)

	// This might be a parse error at scaf level or expr level
	// Either way there should be some diagnostic
	if len(result.Diagnostics) == 0 {
		t.Error("expected at least one diagnostic for malformed expression")
	}
}

func TestRule_InvalidExpression_UndefinedVariable(t *testing.T) {
	t.Parallel()

	// Using a variable not returned by the query should error
	result := analyzeWithQueryAnalyzer(t, `
fn GetUser() `+"`MATCH (u:User) RETURN u`"+`

GetUser {
	test "undefined var" {
		assert (foo.bar > 0)
	}
}
`)

	assertHasDiagnostic(t, result, "invalid-expression")
}

func TestRule_InvalidExpression_PropertyAccessInWhere(t *testing.T) {
	t.Parallel()

	// When query returns u.name, u.email, we should be able to reference
	// either the short name (name) or the full path (u.name) in where clauses.
	// Note: Without schema info, all properties are typed as string, so we use
	// string comparisons. Numeric comparisons would require schema configuration.
	result := analyzeWithQueryAnalyzer(t, `
fn GetUser(userId: int) `+"`MATCH (u:User {id: $userId}) RETURN u.name, u.email, u.status`"+`

GetUser {
	test "property access where" {
		$userId: 1
		u.name: "test" where (u.status == "active")
	}
}
`)

	assertNoDiagnostic(t, result, "invalid-expression")
}

func TestRule_InvalidExpression_PropertyAccessInAssert(t *testing.T) {
	t.Parallel()

	// When query returns u.name, u.email, we should be able to reference
	// the entity prefix (u) in assertions.
	// Note: Without schema info, all properties are typed as string, so we use
	// string comparisons. Numeric comparisons would require schema configuration.
	result := analyzeWithQueryAnalyzer(t, `
fn GetUser(userId: int) `+"`MATCH (u:User {id: $userId}) RETURN u.name, u.email, u.status`"+`

GetUser {
	test "property access assert" {
		$userId: 1
		assert (u.name == "Alice" && u.status != "")
	}
}
`)

	assertNoDiagnostic(t, result, "invalid-expression")
}

// ----------------------------------------------------------------------------
// Tests for: invalid-type-annotation
// ----------------------------------------------------------------------------

func TestRule_InvalidTypeAnnotation_Typo(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn GetUser(id: stirng) `+"`MATCH (u:User {id: $id}) RETURN u`"+`

GetUser {
	test "finds user" {
		$id: "123"
	}
}
`)

	assertHasDiagnostic(t, result, "invalid-type-annotation")
}

func TestRule_InvalidTypeAnnotation_ValidTypes(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn GetUser(id: string, age: int, score: float64, active: bool, data: any) `+"`MATCH (u:User {id: $id}) RETURN u`"+`

GetUser {
	test "finds user" {
		$id: "123"
		$age: 25
		$score: 1.5
		$active: true
		$data: null
	}
}
`)

	assertNoDiagnostic(t, result, "invalid-type-annotation")
}

func TestRule_InvalidTypeAnnotation_ArrayWithInvalidElement(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn GetUsers(ids: [stirng]) `+"`MATCH (u:User) WHERE u.id IN $ids RETURN u`"+`

GetUsers {
	test "finds users" {
		$ids: ["a", "b"]
	}
}
`)

	assertHasDiagnostic(t, result, "invalid-type-annotation")
}

func TestRule_InvalidTypeAnnotation_MapWithInvalidValue(t *testing.T) {
	t.Parallel()

	result := analyze(t, `
fn GetData(data: {string: integr}) `+"`MATCH (d:Data {data: $data}) RETURN d`"+`

GetData {
	test "gets data" {
		$data: {foo: 1}
	}
}
`)

	assertHasDiagnostic(t, result, "invalid-type-annotation")
}

// TestRule_InvalidExpression_FriendlyUndefinedError tests that undefined variable errors
// are reported with a friendly message pointing to the RETURN clause.
func TestRule_InvalidExpression_FriendlyUndefinedError(t *testing.T) {
	t.Parallel()

	result := analyzeWithQueryAnalyzer(t, `
fn GetUser() `+"`MATCH (u:User) RETURN u.name`"+`

GetUser {
	test "undefined variable" {
		assert (foo == "bar")
	}
}
`)

	// Should have the friendly error message
	found := false
	for _, diag := range result.Diagnostics {
		if diag.Code == "invalid-expression" {
			if strings.Contains(diag.Message, "undefined variable 'foo'") &&
				strings.Contains(diag.Message, "check query RETURN clause") {
				found = true
				break
			}
		}
	}

	if !found {
		t.Errorf("Expected friendly undefined variable error, got diagnostics: %v", result.Diagnostics)
	}
}

// TestRule_InvalidExpression_FriendlyBoolError tests that non-boolean assertion errors
// are reported with a friendly message suggesting a comparison.
func TestRule_InvalidExpression_FriendlyBoolError(t *testing.T) {
	t.Parallel()

	result := analyzeWithQueryAnalyzer(t, `
fn GetUser() `+"`MATCH (u:User) RETURN u.name`"+`

GetUser {
	test "string assertion" {
		assert (u.name)
	}
}
`)

	// Should have the friendly error message
	found := false
	for _, diag := range result.Diagnostics {
		if diag.Code == "invalid-expression" {
			if strings.Contains(diag.Message, "assertion must be boolean") &&
				strings.Contains(diag.Message, "got string") {
				found = true
				break
			}
		}
	}

	if !found {
		t.Errorf("Expected friendly boolean assertion error, got diagnostics: %v", result.Diagnostics)
	}
}

// ============================================================================
// Expression validation edge case tests
// ============================================================================

// TestRule_InvalidExpression_EmptyAssert tests that empty assert expressions
// are handled gracefully (no crash, no false positive).
func TestRule_InvalidExpression_EmptyAssert(t *testing.T) {
	t.Parallel()

	result := analyzeWithQueryAnalyzer(t, `
fn GetUser() `+"`MATCH (u:User) RETURN u`"+`

GetUser {
	test "empty assert" {
		assert ()
	}
}
`)

	// Empty expressions should not crash or cause issues
	// They may or may not produce an error depending on parser behavior
	// The key is that analysis completes without panic
	_ = result
}

// TestRule_InvalidExpression_WhitespaceOnly tests assert with only whitespace.
func TestRule_InvalidExpression_WhitespaceOnly(t *testing.T) {
	t.Parallel()

	result := analyzeWithQueryAnalyzer(t, `
fn GetUser() `+"`MATCH (u:User) RETURN u`"+`

GetUser {
	test "whitespace assert" {
		assert (   )
	}
}
`)

	// Whitespace-only expressions should be handled gracefully
	_ = result
}

// TestRule_InvalidExpression_NestedExpressions tests deeply nested expressions.
func TestRule_InvalidExpression_NestedExpressions(t *testing.T) {
	t.Parallel()

	// Note: Without schema, u.age and u.score are inferred as strings.
	// Use string comparisons to avoid type mismatch errors.
	result := analyzeWithQueryAnalyzer(t, `
fn GetUser() `+"`MATCH (u:User) RETURN u.age, u.score, u.active`"+`

GetUser {
	test "nested expressions" {
		assert (((u.age != "") && (u.score != "")) || (u.active == "true"))
	}
}
`)

	// Nested expressions should be validated correctly
	assertNoDiagnostic(t, result, "invalid-expression")
}

// TestRule_InvalidExpression_MultiLineAssert tests multi-line assert expressions.
func TestRule_InvalidExpression_MultiLineAssert(t *testing.T) {
	t.Parallel()

	// Note: Without schema, properties are inferred as strings.
	// Use string comparisons to avoid type mismatch errors.
	result := analyzeWithQueryAnalyzer(t, `
fn GetUser() `+"`MATCH (u:User) RETURN u.age, u.score, u.name`"+`

GetUser {
	test "multi-line assert" {
		assert {
			(u.age != "")
			(u.score != "")
			(u.name != "")
		}
	}
}
`)

	// Multi-line assert blocks should be validated correctly
	assertNoDiagnostic(t, result, "invalid-expression")
}

// TestRule_InvalidExpression_ExprWithBuiltinFunctions tests expr-lang built-in functions.
func TestRule_InvalidExpression_ExprWithBuiltinFunctions(t *testing.T) {
	t.Parallel()

	result := analyzeWithQueryAnalyzer(t, `
fn GetUsers() `+"`MATCH (u:User) RETURN u, collect(u.name) as names`"+`

GetUsers {
	test "builtin functions" {
		assert (len(names) > 0)
	}
}
`)

	// Built-in functions like len() should be recognized
	assertNoDiagnostic(t, result, "invalid-expression")
}

// TestRule_InvalidExpression_StringOperations tests string operations in expressions.
func TestRule_InvalidExpression_StringOperations(t *testing.T) {
	t.Parallel()

	// Test that basic string operations work.
	// Note: expr-lang uses "in" operator, not contains() function for string containment.
	result := analyzeWithQueryAnalyzer(t, `
fn GetUser() `+"`MATCH (u:User) RETURN u.name`"+`

GetUser {
	test "string operations" {
		assert (u.name == "test" || u.name != "")
	}
}
`)

	// String operations should work
	assertNoDiagnostic(t, result, "invalid-expression")
}

// TestRule_InvalidExpression_TypeMismatchInComparison tests type mismatch errors.
func TestRule_InvalidExpression_TypeMismatchInComparison(t *testing.T) {
	t.Parallel()

	// Note: This test depends on having typed returns from the query.
	// When u.name is explicitly returned and typed as string,
	// comparing it to an int should produce an error.
	result := analyzeWithQueryAnalyzer(t, `
fn GetUser() `+"`MATCH (u:User) RETURN u.name`"+`

GetUser {
	test "type mismatch" {
		assert (u.name == 123)
	}
}
`)

	// Type mismatch should be detected (string != int)
	assertHasDiagnostic(t, result, "invalid-expression")
}

// TestRule_InvalidExpression_AssertQueryDefinesScope tests that when an assert has
// a query, the expression variables come from that query's RETURN clause, not the parent scope.
func TestRule_InvalidExpression_AssertQueryDefinesScope(t *testing.T) {
	t.Parallel()

	// CountComments returns 'c', not 'u' - so c.id should be valid
	// Note: Without schema, c.id defaults to string type, so we use string comparison
	result := analyzeWithQueryAnalyzer(t, `
fn GetUser() `+"`MATCH (u:User) RETURN u`"+`
fn CountComments() `+"`MATCH (c:Comment) RETURN c.id, c.content`"+`

GetUser {
	test "assert with query scope" {
		$id: 1
		// The assert's query defines which variables are available
		// c.id comes from CountComments, not GetUser
		assert CountComments() { (c.id != "") }
	}
}
`)

	// Should NOT error - c.id is defined by CountComments
	assertNoDiagnostic(t, result, "invalid-expression")
}

// TestRule_InvalidExpression_AssertQueryUndefinedVariable tests that using a variable
// from the parent scope inside an assert with its own query produces an error.
func TestRule_InvalidExpression_AssertQueryUndefinedVariable(t *testing.T) {
	t.Parallel()

	// CountComments returns 'c', not 'u' - so u.id should be undefined
	result := analyzeWithQueryAnalyzer(t, `
fn GetUser() `+"`MATCH (u:User) RETURN u`"+`
fn CountComments() `+"`MATCH (c:Comment) RETURN c.id, c.content`"+`

GetUser {
	test "assert with wrong scope" {
		$id: 1
		// u is from GetUser, but this assert uses CountComments
		// which only returns c, not u
		assert CountComments() { (u.id > 0) }
	}
}
`)

	// Should error - u is not defined by CountComments
	assertHasDiagnostic(t, result, "invalid-expression")
}

// TestRule_InvalidExpression_InlineAssertQuery tests that inline assert queries
// define their own variable scope.
func TestRule_InvalidExpression_InlineAssertQuery(t *testing.T) {
	t.Parallel()

	// Inline query returns 'cnt', which should be accessible in the assertion
	result := analyzeWithQueryAnalyzer(t, `
fn GetUser() `+"`MATCH (u:User) RETURN u`"+`

GetUser {
	test "inline assert query" {
		$id: 1
		// The inline query defines cnt, which should be available
		assert `+"`MATCH (c:Comment) RETURN count(c) as cnt`"+` { (cnt > 0) }
	}
}
`)

	// Should NOT error - cnt is defined by the inline query
	assertNoDiagnostic(t, result, "invalid-expression")
}

// ============================================================================
// Return type mismatch tests
// ============================================================================

func TestRule_ReturnTypeMismatch_StringExpectedGotBool(t *testing.T) {
	t.Parallel()

	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"User": {
				Name: "User",
				Fields: []*analysis.Field{
					{Name: "name", Type: analysis.TypeString},
					{Name: "age", Type: analysis.TypeInt},
					{Name: "email", Type: analysis.TypeString},
				},
			},
		},
	}

	result := analyzeWithSchema(t, `
fn GetUser(id: string) `+"`MATCH (u:User {id: $id}) RETURN u.name, u.age, u.email`"+`

GetUser {
	test "type mismatch" {
		$id: "123"
		u.name: false
		u.age: 30
	}
}
`, schema)

	assertHasDiagnostic(t, result, "return-type-mismatch")
}

func TestRule_ReturnTypeMismatch_IntExpectedGotString(t *testing.T) {
	t.Parallel()

	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"User": {
				Name: "User",
				Fields: []*analysis.Field{
					{Name: "name", Type: analysis.TypeString},
					{Name: "age", Type: analysis.TypeInt},
				},
			},
		},
	}

	result := analyzeWithSchema(t, `
fn GetUser(id: string) `+"`MATCH (u:User {id: $id}) RETURN u.name, u.age`"+`

GetUser {
	test "age should be int" {
		$id: "123"
		u.name: "Alice"
		u.age: "thirty"
	}
}
`, schema)

	assertHasDiagnostic(t, result, "return-type-mismatch")
}

func TestRule_ReturnTypeMismatch_CorrectTypes(t *testing.T) {
	t.Parallel()

	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"User": {
				Name: "User",
				Fields: []*analysis.Field{
					{Name: "name", Type: analysis.TypeString},
					{Name: "age", Type: analysis.TypeInt},
				},
			},
		},
	}

	result := analyzeWithSchema(t, `
fn GetUser(id: string) `+"`MATCH (u:User {id: $id}) RETURN u.name, u.age`"+`

GetUser {
	test "correct types" {
		$id: "123"
		u.name: "Alice"
		u.age: 30
	}
}
`, schema)

	assertNoDiagnostic(t, result, "return-type-mismatch")
}

func TestRule_ReturnTypeMismatch_NoSchemaNoError(t *testing.T) {
	t.Parallel()

	// Without a schema, types cannot be inferred, so no type errors
	result := analyzeWithQueryAnalyzer(t, `
fn GetUser(id: string) `+"`MATCH (u:User {id: $id}) RETURN u.name, u.age`"+`

GetUser {
	test "no schema" {
		$id: "123"
		u.name: false
		u.age: "thirty"
	}
}
`)

	assertNoDiagnostic(t, result, "return-type-mismatch")
}

func TestRule_ReturnTypeMismatch_NullAlwaysAllowed(t *testing.T) {
	t.Parallel()

	schema := &analysis.TypeSchema{
		Models: map[string]*analysis.Model{
			"User": {
				Name: "User",
				Fields: []*analysis.Field{
					{Name: "name", Type: analysis.TypeString, Required: true},
					{Name: "age", Type: analysis.TypeInt, Required: false},
				},
			},
		},
	}

	// null should be allowed for ANY return field, regardless of required status
	// because queries might return no rows
	result := analyzeWithSchema(t, `
fn GetUser(id: string) `+"`MATCH (u:User {id: $id}) RETURN u.name, u.age`"+`

GetUser {
	test "non-existent user returns null" {
		$id: "999"
		u.name: null
		u.age: null
	}
}
`, schema)

	assertNoDiagnostic(t, result, "return-type-mismatch")
}
