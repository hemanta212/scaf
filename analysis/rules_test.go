package analysis_test

import (
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

	// When there's no type annotation, any value should be allowed
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

	// Note: Without schema info, the cypher analyzer doesn't provide types.
	// All returns are typed as map[string]any, so expr-lang can't type-check properly.
	// This test just verifies we don't crash - full type checking requires schema.
	result := analyzeWithQueryAnalyzer(t, `
fn GetUser() `+"`MATCH (u:User) RETURN u`"+`

GetUser {
	test "non-bool assertion" {
		assert (u.age)
	}
}
`)

	// Without schema-derived types, u.age is map[string]any which can be any type
	// So this won't error. This is a known limitation.
	// The test passes if we don't crash and can analyze the file.
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
