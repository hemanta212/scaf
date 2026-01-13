package analysis

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
	"github.com/expr-lang/expr"
	exprfile "github.com/expr-lang/expr/file"
	"github.com/rlch/scaf"
)

// Rule represents a semantic analysis check.
// Inspired by go/analysis.Analyzer pattern.
type Rule struct {
	// Name is a short identifier for the rule (used in diagnostic codes).
	Name string

	// Doc is a brief description of what the rule checks.
	Doc string

	// Severity is the default severity for diagnostics from this rule.
	Severity DiagnosticSeverity

	// Run executes the rule and appends any diagnostics to the file.
	Run func(f *AnalyzedFile)
}

// DefaultRules returns all built-in semantic analysis rules.
func DefaultRules() []*Rule {
	return []*Rule{
		// Error-level checks.
		undefinedQueryRule,
		undefinedImportRule,
		duplicateQueryRule,
		duplicateImportRule,
		undefinedAssertQueryRule,
		undefinedSetupQueryRule,   // Cross-file validation
		paramTypeMismatchRule,     // Type checking for function parameters
		returnTypeMismatchRule,    // Type checking for return value assertions
		undeclaredQueryParamRule,  // Parameters used in query body but not declared
		unknownParameterRule,      // Using a parameter that doesn't exist in the query
		duplicateTestRule,         // Duplicate test names cause conflicts
		duplicateGroupRule,        // Duplicate group names cause conflicts
		missingRequiredParamsRule, // Missing params will cause runtime failures
		invalidExpressionRule,     // Expression syntax/type errors (compile-time)
		invalidTypeAnnotationRule, // Invalid type names in function signatures

		// Warning-level checks.
		unusedImportRule,
		unusedDeclaredParamRule, // Declared param not used in query body
		emptyGroupRule,
		samePackageImportRule,

		// Hint-level checks.
		emptyTestRule,
		unusedQueryParamRule,
	}
}

// ----------------------------------------------------------------------------
// Rule: undefined-query
// ----------------------------------------------------------------------------

var undefinedQueryRule = &Rule{
	Name:     "undefined-query",
	Doc:      "Reports query scopes that reference undefined queries.",
	Severity: SeverityError,
	Run:      checkUndefinedQueries,
}

func checkUndefinedQueries(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	for _, scope := range f.Suite.Scopes {
		if _, ok := f.Symbols.Queries[scope.FunctionName]; !ok {
			f.Diagnostics = append(f.Diagnostics, Diagnostic{
				Span:     scope.Span(),
				Severity: SeverityError,
				Message:  "undefined query: " + scope.FunctionName,
				Code:     "undefined-query",
				Source:   "scaf",
			})
		}
	}
}

// ----------------------------------------------------------------------------
// Rule: undefined-import
// ----------------------------------------------------------------------------

var undefinedImportRule = &Rule{
	Name:     "undefined-import",
	Doc:      "Reports setup calls that reference undefined imports.",
	Severity: SeverityError,
	Run:      checkUndefinedImports,
}

func checkUndefinedImports(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	checkSetup := func(setup *scaf.SetupClause) {
		if setup == nil {
			return
		}

		if setup.Module != nil {
			checkSetupModuleImport(f, *setup.Module, setup.Span())
		}

		if setup.Call != nil {
			checkSetupCallImport(f, setup.Call)
		}

		for _, item := range setup.Block {
			if item.Module != nil {
				checkSetupModuleImport(f, *item.Module, item.Span())
			}

			if item.Call != nil {
				checkSetupCallImport(f, item.Call)
			}
		}
	}

	var checkItems func([]*scaf.TestOrGroup)

	checkItems = func(items []*scaf.TestOrGroup) {
		for _, item := range items {
			if item.Test != nil {
				checkSetup(item.Test.Setup)
			}

			if item.Group != nil {
				checkSetup(item.Group.Setup)
				checkItems(item.Group.Items)
			}
		}
	}

	checkSetup(f.Suite.Setup)

	for _, scope := range f.Suite.Scopes {
		checkSetup(scope.Setup)
		checkItems(scope.Items)
	}
}

func checkSetupModuleImport(f *AnalyzedFile, moduleAlias string, span scaf.Span) {
	if imp, ok := f.Symbols.Imports[moduleAlias]; !ok {
		f.Diagnostics = append(f.Diagnostics, Diagnostic{
			Span:     span,
			Severity: SeverityError,
			Message:  "undefined import: " + moduleAlias,
			Code:     "undefined-import",
			Source:   "scaf",
		})
	} else {
		imp.Used = true
	}
}

func checkSetupCallImport(f *AnalyzedFile, call *scaf.SetupCall) {
	if imp, ok := f.Symbols.Imports[call.Module]; !ok {
		f.Diagnostics = append(f.Diagnostics, Diagnostic{
			Span:     call.Span(),
			Severity: SeverityError,
			Message:  "undefined import: " + call.Module,
			Code:     "undefined-import",
			Source:   "scaf",
		})
	} else {
		imp.Used = true
	}
}

// ----------------------------------------------------------------------------
// Rule: duplicate-query
// ----------------------------------------------------------------------------

var duplicateQueryRule = &Rule{
	Name:     "duplicate-query",
	Doc:      "Reports duplicate query name definitions.",
	Severity: SeverityError,
	Run:      checkDuplicateQueries,
}

func checkDuplicateQueries(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	seen := make(map[string]scaf.Span)

	for _, q := range f.Suite.Functions {
		if firstSpan, exists := seen[q.Name]; exists {
			f.Diagnostics = append(f.Diagnostics, Diagnostic{
				Span:     q.Span(),
				Severity: SeverityError,
				Message:  "duplicate query name: " + q.Name + " (first defined at line " + formatLine(firstSpan) + ")",
				Code:     "duplicate-query",
				Source:   "scaf",
			})
		} else {
			seen[q.Name] = q.Span()
		}
	}
}

// ----------------------------------------------------------------------------
// Rule: duplicate-import
// ----------------------------------------------------------------------------

var duplicateImportRule = &Rule{
	Name:     "duplicate-import",
	Doc:      "Reports duplicate import aliases.",
	Severity: SeverityError,
	Run:      checkDuplicateImports,
}

func checkDuplicateImports(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	seen := make(map[string]scaf.Span)

	for _, imp := range f.Suite.Imports {
		alias := baseNameFromPath(imp.Path)
		if imp.Alias != nil {
			alias = *imp.Alias
		}

		if firstSpan, exists := seen[alias]; exists {
			f.Diagnostics = append(f.Diagnostics, Diagnostic{
				Span:     imp.Span(),
				Severity: SeverityError,
				Message:  "duplicate import alias: " + alias + " (first defined at line " + formatLine(firstSpan) + ")",
				Code:     "duplicate-import",
				Source:   "scaf",
			})
		} else {
			seen[alias] = imp.Span()
		}
	}
}

// ----------------------------------------------------------------------------
// Rule: unused-import
// ----------------------------------------------------------------------------

var unusedImportRule = &Rule{
	Name:     "unused-import",
	Doc:      "Reports imports that are never referenced.",
	Severity: SeverityWarning,
	Run:      checkUnusedImports,
}

func checkUnusedImports(f *AnalyzedFile) {
	// Note: Must run after undefinedImportRule which marks imports as used.
	for alias, imp := range f.Symbols.Imports {
		if !imp.Used {
			f.Diagnostics = append(f.Diagnostics, Diagnostic{
				Span:     imp.Span,
				Severity: SeverityWarning,
				Message:  "unused import: " + alias,
				Code:     "unused-import",
				Source:   "scaf",
			})
		}
	}
}

// ----------------------------------------------------------------------------
// Rule: unknown-parameter
// ----------------------------------------------------------------------------

var unknownParameterRule = &Rule{
	Name:     "unknown-parameter",
	Doc:      "Reports test parameters that don't exist in the query.",
	Severity: SeverityError,
	Run:      checkUnknownParameters,
}

func checkUnknownParameters(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	for _, scope := range f.Suite.Scopes {
		query, ok := f.Symbols.Queries[scope.FunctionName]
		if !ok {
			continue // Already reported as undefined-query.
		}

		queryParams := make(map[string]bool)
		for _, p := range query.Params {
			queryParams[p] = true
		}

		checkItemParams(f, scope.Items, queryParams, scope.FunctionName)
	}
}

func checkItemParams(f *AnalyzedFile, items []*scaf.TestOrGroup, queryParams map[string]bool, queryName string) {
	for _, item := range items {
		if item.Test != nil {
			for _, stmt := range item.Test.Statements {
				key := stmt.Key()
				if paramName, ok := strings.CutPrefix(key, "$"); ok {
					if !queryParams[paramName] {
						// Use statement span for precise highlighting
						f.Diagnostics = append(f.Diagnostics, Diagnostic{
							Span:     stmt.Span(),
							Severity: SeverityError,
							Message:  "parameter $" + paramName + " not found in query " + queryName,
							Code:     "unknown-parameter",
							Source:   "scaf",
						})
					}
				}
			}
		}

		if item.Group != nil {
			checkItemParams(f, item.Group.Items, queryParams, queryName)
		}
	}
}

// ----------------------------------------------------------------------------
// Rule: empty-test
// ----------------------------------------------------------------------------

var emptyTestRule = &Rule{
	Name:     "empty-test",
	Doc:      "Reports tests with no statements or assertions.",
	Severity: SeverityHint,
	Run:      checkEmptyTests,
}

func checkEmptyTests(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	var checkItems func([]*scaf.TestOrGroup)

	checkItems = func(items []*scaf.TestOrGroup) {
		for _, item := range items {
			if item.Test != nil {
				if len(item.Test.Statements) == 0 && len(item.Test.Asserts) == 0 && item.Test.Setup == nil {
					f.Diagnostics = append(f.Diagnostics, Diagnostic{
						Span:     item.Test.Span(),
						Severity: SeverityHint,
						Message:  "empty test: " + item.Test.Name,
						Code:     "empty-test",
						Source:   "scaf",
					})
				}
			}

			if item.Group != nil {
				checkItems(item.Group.Items)
			}
		}
	}

	for _, scope := range f.Suite.Scopes {
		checkItems(scope.Items)
	}
}

// ----------------------------------------------------------------------------
// Rule: duplicate-test
// ----------------------------------------------------------------------------

var duplicateTestRule = &Rule{
	Name:     "duplicate-test",
	Doc:      "Reports duplicate test names within the same scope.",
	Severity: SeverityError,
	Run:      checkDuplicateTests,
}

func checkDuplicateTests(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	for _, scope := range f.Suite.Scopes {
		checkDuplicateTestNamesInItems(f, scope.Items)
	}
}

// ----------------------------------------------------------------------------
// Rule: duplicate-group
// ----------------------------------------------------------------------------

var duplicateGroupRule = &Rule{
	Name:     "duplicate-group",
	Doc:      "Reports duplicate group names within the same scope.",
	Severity: SeverityError,
	Run:      checkDuplicateGroups,
}

func checkDuplicateGroups(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	for _, scope := range f.Suite.Scopes {
		checkDuplicateGroupNamesInItems(f, scope.Items)
	}
}

func checkDuplicateTestNamesInItems(f *AnalyzedFile, items []*scaf.TestOrGroup) {
	testNames := make(map[string]scaf.Span)

	for _, item := range items {
		if item.Test != nil {
			if firstSpan, exists := testNames[item.Test.Name]; exists {
				f.Diagnostics = append(f.Diagnostics, Diagnostic{
					Span:     item.Test.Span(),
					Severity: SeverityError,
					Message: "duplicate test name in scope: " + item.Test.Name +
						" (first defined at line " + formatLine(firstSpan) + ")",
					Code:   "duplicate-test",
					Source: "scaf",
				})
			} else {
				testNames[item.Test.Name] = item.Test.Span()
			}
		}

		if item.Group != nil {
			// Recurse into group.
			checkDuplicateTestNamesInItems(f, item.Group.Items)
		}
	}
}

func checkDuplicateGroupNamesInItems(f *AnalyzedFile, items []*scaf.TestOrGroup) {
	groupNames := make(map[string]scaf.Span)

	for _, item := range items {
		if item.Group != nil {
			if firstSpan, exists := groupNames[item.Group.Name]; exists {
				f.Diagnostics = append(f.Diagnostics, Diagnostic{
					Span:     item.Group.Span(),
					Severity: SeverityError,
					Message: "duplicate group name in scope: " + item.Group.Name +
						" (first defined at line " + formatLine(firstSpan) + ")",
					Code:   "duplicate-group",
					Source: "scaf",
				})
			} else {
				groupNames[item.Group.Name] = item.Group.Span()
			}

			// Recurse into group.
			checkDuplicateGroupNamesInItems(f, item.Group.Items)
		}
	}
}

// ----------------------------------------------------------------------------
// Rule: undefined-assert-query
// ----------------------------------------------------------------------------

var undefinedAssertQueryRule = &Rule{
	Name:     "undefined-assert-query",
	Doc:      "Reports assert blocks that reference undefined queries.",
	Severity: SeverityError,
	Run:      checkUndefinedAssertQueries,
}

func checkUndefinedAssertQueries(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	var checkItems func([]*scaf.TestOrGroup)

	checkItems = func(items []*scaf.TestOrGroup) {
		for _, item := range items {
			if item.Test != nil {
				for _, assert := range item.Test.Asserts {
					if assert.Query != nil && assert.Query.QueryName != nil {
						queryName := *assert.Query.QueryName
						if _, ok := f.Symbols.Queries[queryName]; !ok {
							// Use assert span for precise highlighting
							f.Diagnostics = append(f.Diagnostics, Diagnostic{
								Span:     assert.Span(),
								Severity: SeverityError,
								Message:  "assert references undefined query: " + queryName,
								Code:     "undefined-assert-query",
								Source:   "scaf",
							})
						}
					}
				}
			}

			if item.Group != nil {
				checkItems(item.Group.Items)
			}
		}
	}

	for _, scope := range f.Suite.Scopes {
		checkItems(scope.Items)
	}
}

// ----------------------------------------------------------------------------
// Rule: missing-required-params
// ----------------------------------------------------------------------------

var missingRequiredParamsRule = &Rule{
	Name:     "missing-required-params",
	Doc:      "Reports tests that don't provide all required query parameters.",
	Severity: SeverityError,
	Run:      checkMissingRequiredParams,
}

func checkMissingRequiredParams(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	for _, scope := range f.Suite.Scopes {
		query, ok := f.Symbols.Queries[scope.FunctionName]
		if !ok {
			continue // Already reported as undefined-query.
		}

		checkItemMissingParams(f, scope.Items, query.Params, scope.FunctionName)
	}
}

func checkItemMissingParams(f *AnalyzedFile, items []*scaf.TestOrGroup, queryParams []string, queryName string) {
	for _, item := range items {
		if item.Test != nil {
			providedParams := make(map[string]bool)

			for _, stmt := range item.Test.Statements {
				key := stmt.Key()
				if paramName, ok := strings.CutPrefix(key, "$"); ok {
					providedParams[paramName] = true
				}
			}

			var missing []string

			for _, p := range queryParams {
				if !providedParams[p] {
					missing = append(missing, "$"+p)
				}
			}

			if len(missing) > 0 {
				f.Diagnostics = append(f.Diagnostics, Diagnostic{
					Span:     item.Test.Span(),
					Severity: SeverityError,
					Message:  "test is missing required parameters for " + queryName + ": " + strings.Join(missing, ", "),
					Code:     "missing-required-params",
					Source:   "scaf",
				})
			}
		}

		if item.Group != nil {
			checkItemMissingParams(f, item.Group.Items, queryParams, queryName)
		}
	}
}

// ----------------------------------------------------------------------------
// Rule: unused-declared-param
// ----------------------------------------------------------------------------

var unusedDeclaredParamRule = &Rule{
	Name:     "unused-declared-param",
	Doc:      "Reports function parameters that are declared but never used in the query body.",
	Severity: SeverityWarning,
	Run:      checkUnusedDeclaredParams,
}

func checkUnusedDeclaredParams(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	for _, query := range f.Symbols.Queries {
		// Skip if no declared params
		if len(query.DeclaredParams) == 0 {
			continue
		}

		// Get params used in query body
		// Prefer QueryBodyParams (from dialect analyzer) over Params (regex fallback)
		usedParams := make(map[string]bool)
		if len(query.QueryBodyParams) > 0 {
			for _, p := range query.QueryBodyParams {
				usedParams[p.Name] = true
			}
		} else {
			for _, p := range query.Params {
				usedParams[p] = true
			}
		}

		// Check each declared param is used in the body
		for paramName := range query.DeclaredParams {
			if !usedParams[paramName] {
				// Try to find the specific parameter span for precise highlighting
				paramSpan := query.Span // Fallback to function span
				if query.Node != nil {
					for _, p := range query.Node.Params {
						if p != nil && p.Name == paramName {
							paramSpan = p.Span()
							break
						}
					}
				}
				f.Diagnostics = append(f.Diagnostics, Diagnostic{
					Span:     paramSpan,
					Severity: SeverityWarning,
					Message:  "declared parameter " + paramName + " is not used in query body",
					Code:     "unused-declared-param",
					Source:   "scaf",
				})
			}
		}
	}
}

// ----------------------------------------------------------------------------
// Rule: empty-group
// ----------------------------------------------------------------------------

var emptyGroupRule = &Rule{
	Name:     "empty-group",
	Doc:      "Reports groups with no tests or nested groups.",
	Severity: SeverityWarning,
	Run:      checkEmptyGroups,
}

func checkEmptyGroups(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	var checkItems func([]*scaf.TestOrGroup)

	checkItems = func(items []*scaf.TestOrGroup) {
		for _, item := range items {
			if item.Group != nil {
				if len(item.Group.Items) == 0 {
					f.Diagnostics = append(f.Diagnostics, Diagnostic{
						Span:     item.Group.Span(),
						Severity: SeverityWarning,
						Message:  "empty group: " + item.Group.Name,
						Code:     "empty-group",
						Source:   "scaf",
					})
				}

				checkItems(item.Group.Items)
			}
		}
	}

	for _, scope := range f.Suite.Scopes {
		checkItems(scope.Items)
	}
}

// ----------------------------------------------------------------------------
// Rule: undefined-setup-query
// ----------------------------------------------------------------------------

var undefinedSetupQueryRule = &Rule{
	Name:     "undefined-setup-query",
	Doc:      "Reports setup calls that reference queries not found in the imported module.",
	Severity: SeverityError,
	Run:      checkUndefinedSetupQueries,
}

func checkUndefinedSetupQueries(f *AnalyzedFile) {
	if f.Suite == nil || f.Resolver == nil {
		return // Cross-file validation requires a resolver
	}

	// Helper to check a setup call
	checkSetupCall := func(call *scaf.SetupCall) {
		if call == nil {
			return
		}

		// Get the import for this module
		imp, ok := f.Symbols.Imports[call.Module]
		if !ok {
			// undefined-import rule handles this
			return
		}

		// Resolve the import path
		importedPath := f.Resolver.ResolveImportPath(f.Path, imp.Path)
		importedFile := f.Resolver.LoadAndAnalyze(importedPath)
		if importedFile == nil || importedFile.Symbols == nil {
			// Can't load/analyze the file - don't report error since file might just not exist yet
			return
		}

		// Check if the query exists in the imported module
		if _, ok := importedFile.Symbols.Queries[call.Query]; !ok {
			// Build list of available queries for better error message
			var available []string
			for name := range importedFile.Symbols.Queries {
				available = append(available, name)
			}

			msg := "undefined query in module " + call.Module + ": " + call.Query
			if len(available) > 0 {
				msg += " (available: " + strings.Join(available, ", ") + ")"
			}

			f.Diagnostics = append(f.Diagnostics, Diagnostic{
				Span:     call.Span(),
				Severity: SeverityError,
				Message:  msg,
				Code:     "undefined-setup-query",
				Source:   "scaf",
			})
		}
	}

	// Helper to check a setup clause
	checkSetup := func(setup *scaf.SetupClause) {
		if setup == nil {
			return
		}

		checkSetupCall(setup.Call)

		for _, item := range setup.Block {
			checkSetupCall(item.Call)
		}
	}

	// Check test/group items recursively
	var checkItems func([]*scaf.TestOrGroup)
	checkItems = func(items []*scaf.TestOrGroup) {
		for _, item := range items {
			if item.Test != nil {
				checkSetup(item.Test.Setup)
			}
			if item.Group != nil {
				checkSetup(item.Group.Setup)
				checkItems(item.Group.Items)
			}
		}
	}

	// Check global setup
	checkSetup(f.Suite.Setup)

	// Check all scopes
	for _, scope := range f.Suite.Scopes {
		checkSetup(scope.Setup)
		checkItems(scope.Items)
	}
}

// ----------------------------------------------------------------------------
// Rule: unused-query-param
// ----------------------------------------------------------------------------

var unusedQueryParamRule = &Rule{
	Name:     "unused-query-param",
	Doc:      "Reports query parameters that are never provided in any test.",
	Severity: SeverityHint,
	Run:      checkUnusedQueryParams,
}

func checkUnusedQueryParams(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	for _, scope := range f.Suite.Scopes {
		query, ok := f.Symbols.Queries[scope.FunctionName]
		if !ok {
			continue // Already reported as undefined-query.
		}

		if len(query.Params) == 0 {
			continue // No parameters to check.
		}

		// Collect all parameters provided across all tests in this scope.
		providedParams := make(map[string]bool)
		collectProvidedParams(scope.Items, providedParams)

		// Report parameters that exist in query but never appear in any test.
		for _, param := range query.Params {
			if !providedParams[param] {
				f.Diagnostics = append(f.Diagnostics, Diagnostic{
					Span:     scope.Span(),
					Severity: SeverityHint,
					Message:  "query parameter $" + param + " is never provided in any test within this scope",
					Code:     "unused-query-param",
					Source:   "scaf",
				})
			}
		}
	}
}

func collectProvidedParams(items []*scaf.TestOrGroup, provided map[string]bool) {
	for _, item := range items {
		if item.Test != nil {
			for _, stmt := range item.Test.Statements {
				key := stmt.Key()
				if paramName, ok := strings.CutPrefix(key, "$"); ok {
					provided[paramName] = true
				}
			}
		}

		if item.Group != nil {
			collectProvidedParams(item.Group.Items, provided)
		}
	}
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

func formatLine(span scaf.Span) string {
	return strconv.Itoa(span.Start.Line)
}

// ----------------------------------------------------------------------------
// Rule: undeclared-query-param
// ----------------------------------------------------------------------------

var undeclaredQueryParamRule = &Rule{
	Name:     "undeclared-query-param",
	Doc:      "Reports parameters used in query body that are not declared in the function signature.",
	Severity: SeverityError,
	Run:      checkUndeclaredQueryParams,
}

func checkUndeclaredQueryParams(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	for _, query := range f.Symbols.Queries {
		// Get params used in query body
		// Prefer QueryBodyParams (from dialect analyzer) over Params (regex fallback)
		var bodyParams []string
		if len(query.QueryBodyParams) > 0 {
			for _, p := range query.QueryBodyParams {
				bodyParams = append(bodyParams, p.Name)
			}
		} else {
			bodyParams = query.Params
		}

		// If there are no params in the body, nothing to check
		if len(bodyParams) == 0 {
			continue
		}

		// Build set of declared params (may be nil/empty if function has no params declared)
		// Use DeclaredParams which includes both typed and untyped parameters
		declaredParams := query.DeclaredParams
		if declaredParams == nil {
			declaredParams = make(map[string]bool)
		}

		// Check each param used in the body is declared in the function signature
		for _, paramName := range bodyParams {
			if !declaredParams[paramName] {
				f.Diagnostics = append(f.Diagnostics, Diagnostic{
					Span:     query.Span,
					Severity: SeverityError,
					Message:  "parameter $" + paramName + " used in query body but not declared in function signature",
					Code:     "undeclared-query-param",
					Source:   "scaf",
				})
			}
		}
	}
}

// ----------------------------------------------------------------------------
// Rule: param-type-mismatch
// ----------------------------------------------------------------------------

var paramTypeMismatchRule = &Rule{
	Name:     "param-type-mismatch",
	Doc:      "Reports test parameters with values that don't match the function's type annotation or schema-inferred type.",
	Severity: SeverityError,
	Run:      checkParamTypeMismatch,
}

func checkParamTypeMismatch(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	for _, scope := range f.Suite.Scopes {
		query, ok := f.Symbols.Queries[scope.FunctionName]
		if !ok {
			continue
		}

		// Build map of inferred types from query body params
		inferredTypes := make(map[string]*scaf.Type)
		for _, p := range query.QueryBodyParams {
			if p.Type != nil {
				inferredTypes[p.Name] = p.Type
			}
		}

		// Skip if no types to check (neither explicit nor inferred)
		if query.TypedParams == nil && len(inferredTypes) == 0 {
			continue
		}

		checkItemParamTypes(f, scope.Items, query.TypedParams, inferredTypes, scope.FunctionName)
	}
}

func checkItemParamTypes(f *AnalyzedFile, items []*scaf.TestOrGroup, typedParams map[string]*scaf.TypeExpr, inferredTypes map[string]*scaf.Type, queryName string) {
	for _, item := range items {
		if item.Test != nil {
			for _, stmt := range item.Test.Statements {
				key := stmt.Key()
				paramName, isParam := strings.CutPrefix(key, "$")
				if !isParam {
					continue // Not an input parameter
				}

				if stmt.Value == nil || stmt.Value.Literal == nil {
					continue // No value to check (expressions are runtime-evaluated)
				}

				// First check explicit type annotations (higher priority)
				if expectedType, hasType := typedParams[paramName]; hasType && expectedType != nil {
					if err := checkValueMatchesType(stmt.Value.Literal, expectedType); err != nil {
						f.Diagnostics = append(f.Diagnostics, Diagnostic{
							Span:     stmt.Span(),
							Severity: SeverityError,
							Message:  "type mismatch for parameter $" + paramName + ": " + err.Error(),
							Code:     "param-type-mismatch",
							Source:   "scaf",
						})
					}
					continue // Skip inferred type check if explicit annotation exists
				}

				// Then check inferred types from schema-aware analysis
				if inferredType, hasInferred := inferredTypes[paramName]; hasInferred && inferredType != nil {
					if err := checkValueMatchesInferredType(stmt.Value.Literal, inferredType); err != nil {
						f.Diagnostics = append(f.Diagnostics, Diagnostic{
							Span:     stmt.Span(),
							Severity: SeverityError,
							Message:  "type mismatch for parameter $" + paramName + " (inferred from schema): " + err.Error(),
							Code:     "param-type-mismatch",
							Source:   "scaf",
						})
					}
				}
			}
		}

		if item.Group != nil {
			checkItemParamTypes(f, item.Group.Items, typedParams, inferredTypes, queryName)
		}
	}
}

// checkValueMatchesInferredType checks if a value matches an inferred type (scaf.Type).
// Returns an error describing the mismatch, or nil if the value is valid.
func checkValueMatchesInferredType(v *scaf.Value, t *scaf.Type) error {
	if v == nil || t == nil {
		return nil
	}

	// Handle null values
	if v.Null {
		// Null might be valid for pointer types
		if t.Kind == scaf.TypeKindPointer {
			return nil
		}
		return errTypeMismatch(t.String(), "null")
	}

	switch t.Kind {
	case scaf.TypeKindPrimitive:
		return checkValueMatchesPrimitive(v, t.Name)
	case scaf.TypeKindSlice:
		if v.List == nil {
			return errTypeMismatch(t.String(), inferValueType(v))
		}
		// Check each element
		for i, elem := range v.List.Values {
			if err := checkValueMatchesInferredType(elem, t.Elem); err != nil {
				return errArrayElemMismatch(i, err)
			}
		}
		return nil
	case scaf.TypeKindPointer:
		// Pointer to a type - check the underlying type
		return checkValueMatchesInferredType(v, t.Elem)
	case scaf.TypeKindMap:
		if v.Map == nil {
			return errTypeMismatch(t.String(), inferValueType(v))
		}
		// Check each value (map value type is in Elem field)
		for _, entry := range v.Map.Entries {
			if err := checkValueMatchesInferredType(entry.Value, t.Elem); err != nil {
				return errMapValueMismatch(entry.Key, err)
			}
		}
		return nil
	case scaf.TypeKindNamed:
		// Named types (custom types) - allow any value for now
		return nil
	}

	return nil
}

// checkValueMatchesPrimitive checks if a value matches a primitive type name.
func checkValueMatchesPrimitive(v *scaf.Value, typeName string) error {
	switch typeName {
	case "string":
		if v.Str == nil {
			return errTypeMismatch("string", inferValueType(v))
		}
	case "int", "int64", "int32", "int16", "int8", "uint", "uint64", "uint32", "uint16", "uint8":
		if v.Number == nil {
			return errTypeMismatch(typeName, inferValueType(v))
		}
		// Check if it's actually an integer
		if *v.Number != float64(int64(*v.Number)) {
			return errTypeMismatch(typeName, "float")
		}
	case "float64", "float32":
		if v.Number == nil {
			return errTypeMismatch(typeName, inferValueType(v))
		}
	case "bool":
		if v.Boolean == nil {
			return errTypeMismatch("bool", inferValueType(v))
		}
	default:
		// Unknown primitive type - allow any value
		return nil
	}

	return nil
}

// checkValueMatchesType checks if a value matches the expected type.
// Returns an error describing the mismatch, or nil if the value is valid.
func checkValueMatchesType(v *scaf.Value, t *scaf.TypeExpr) error {
	if v == nil || t == nil {
		return nil
	}

	// Handle nullable types - null is always valid for nullable
	if t.Nullable && v.Null {
		return nil
	}

	// Handle null for non-nullable types
	if v.Null && !t.Nullable {
		expectedType := typeExprToString(t)
		return errTypeMismatch(expectedType, "null")
	}

	// Check based on the expected type
	switch {
	case t.Simple != nil:
		return checkSimpleType(v, *t.Simple, t.Nullable)
	case t.Array != nil:
		return checkArrayType(v, t.Array)
	case t.Map != nil:
		return checkMapType(v, t.Map)
	}

	// If we can't determine the expected type, allow any value
	return nil
}

func checkSimpleType(v *scaf.Value, typeName string, nullable bool) error {
	switch typeName {
	case "string":
		if v.Str == nil && !v.Null {
			return errTypeMismatch("string", inferValueType(v))
		}
	case "int", "int64", "int32":
		if v.Number == nil {
			return errTypeMismatch(typeName, inferValueType(v))
		}
		// Check if it's actually an integer
		if v.Number != nil && *v.Number != float64(int64(*v.Number)) {
			return errTypeMismatch(typeName, "float")
		}
	case "float", "float64", "float32":
		if v.Number == nil {
			return errTypeMismatch(typeName, inferValueType(v))
		}
	case "bool":
		if v.Boolean == nil && !v.Null {
			return errTypeMismatch("bool", inferValueType(v))
		}
	case "any":
		// Any type accepts any value
		return nil
	default:
		// Unknown type - allow any value (might be a custom type)
		return nil
	}

	return nil
}

func checkArrayType(v *scaf.Value, elemType *scaf.TypeExpr) error {
	if v.List == nil {
		return errTypeMismatch("array", inferValueType(v))
	}

	// Check each element matches the element type
	for i, elem := range v.List.Values {
		if err := checkValueMatchesType(elem, elemType); err != nil {
			return errArrayElemMismatch(i, err)
		}
	}

	return nil
}

func checkMapType(v *scaf.Value, mapType *scaf.MapTypeExpr) error {
	if v.Map == nil {
		return errTypeMismatch("map", inferValueType(v))
	}

	// Check each value matches the value type
	// (Keys are always strings in the DSL, so we only check values)
	for _, entry := range v.Map.Entries {
		if err := checkValueMatchesType(entry.Value, mapType.Value); err != nil {
			return errMapValueMismatch(entry.Key, err)
		}
	}

	return nil
}

// inferValueType returns a human-readable type name for a value.
func inferValueType(v *scaf.Value) string {
	switch {
	case v.Null:
		return "null"
	case v.Str != nil:
		return "string"
	case v.Number != nil:
		if *v.Number == float64(int64(*v.Number)) {
			return "int"
		}
		return "float"
	case v.Boolean != nil:
		return "bool"
	case v.Map != nil:
		return "map"
	case v.List != nil:
		return "array"
	default:
		return "unknown"
	}
}

// typeExprToString converts a TypeExpr to a human-readable string.
func typeExprToString(t *scaf.TypeExpr) string {
	if t == nil {
		return "unknown"
	}

	var base string
	switch {
	case t.Simple != nil:
		base = *t.Simple
	case t.Array != nil:
		base = "[" + typeExprToString(t.Array) + "]"
	case t.Map != nil:
		base = "{" + typeExprToString(t.Map.Key) + ": " + typeExprToString(t.Map.Value) + "}"
	default:
		base = "unknown"
	}

	if t.Nullable {
		base += "?"
	}

	return base
}

// Error helpers for type mismatch messages.

type typeMismatchError struct {
	expected string
	actual   string
}

func (e *typeMismatchError) Error() string {
	return "expected " + e.expected + ", got " + e.actual
}

func errTypeMismatch(expected, actual string) error {
	return &typeMismatchError{expected: expected, actual: actual}
}

type arrayElemError struct {
	index int
	err   error
}

func (e *arrayElemError) Error() string {
	return "element " + strconv.Itoa(e.index) + ": " + e.err.Error()
}

func errArrayElemMismatch(index int, err error) error {
	return &arrayElemError{index: index, err: err}
}

type mapValueError struct {
	key string
	err error
}

func (e *mapValueError) Error() string {
	return "key '" + e.key + "': " + e.err.Error()
}

func errMapValueMismatch(key string, err error) error {
	return &mapValueError{key: key, err: err}
}

// ----------------------------------------------------------------------------
// Rule: invalid-expression
// ----------------------------------------------------------------------------

var invalidExpressionRule = &Rule{
	Name:     "invalid-expression",
	Doc:      "Reports invalid expr-lang expressions in assertions and statement values.",
	Severity: SeverityError,
	Run:      checkInvalidExpressions,
}

func checkInvalidExpressions(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	// Check all scopes
	for _, scope := range f.Suite.Scopes {
		checkScopeExpressions(f, scope)
	}
}

// checkScopeExpressions checks expressions in a query scope.
func checkScopeExpressions(f *AnalyzedFile, scope *scaf.FunctionScope) {
	if scope == nil {
		return
	}

	// Recursively check items, passing the function name for type checking
	checkItemExpressions(f, scope.Items, scope.FunctionName)
}

// checkItemExpressions recursively checks expressions in test/group items.
// queryName is passed through to checkTestExpressions for type checking.
func checkItemExpressions(f *AnalyzedFile, items []*scaf.TestOrGroup, queryName string) {
	for _, item := range items {
		if item == nil {
			continue
		}

		if item.Test != nil {
			checkTestExpressions(f, item.Test, queryName)
		}

		if item.Group != nil {
			checkItemExpressions(f, item.Group.Items, queryName)
		}
	}
}

// checkTestExpressions checks all expressions in a test.
// queryName is used to look up typed parameters for type checking.
func checkTestExpressions(f *AnalyzedFile, test *scaf.Test, queryName string) {
	if test == nil {
		return
	}

	// Get typed params for this query (if any)
	var typedParams map[string]*scaf.TypeExpr
	if query, ok := f.Symbols.Queries[queryName]; ok {
		typedParams = query.TypedParams
	}

	// Build environment from query returns for expression validation
	env := buildExprEnvFromQuery(f, queryName)

	// Check statement expressions and where clauses
	for _, stmt := range test.Statements {
		if stmt == nil || stmt.Value == nil {
			continue
		}

		// Check expression value - type depends on the parameter
		if stmt.Value.Expr != nil {
			key := stmt.Key()
			paramName, isParam := strings.CutPrefix(key, "$")

			if isParam && typedParams != nil {
				if expectedType, hasType := typedParams[paramName]; hasType {
					// Parameter has a type annotation - expression must match
					checkTypedExpressionAtSpan(f, stmt.Value.Expr, stmt.Value.Expr.Span(), expectedType, "expression for $"+paramName, env)
				} else {
					// No type annotation - just check syntax with env
					checkExpressionAtSpan(f, stmt.Value.Expr, stmt.Value.Expr.Span(), "expression", nil, env)
				}
			} else {
				// Not a parameter or no query context - check with env
				checkExpressionAtSpan(f, stmt.Value.Expr, stmt.Value.Expr.Span(), "expression", nil, env)
			}
		}

		// Check where clause - must be bool
		if stmt.Value.Where != nil {
			checkExpressionAtSpan(f, stmt.Value.Where, stmt.Value.Where.Span(), "where constraint", boolOption(), env)
		}
	}

	// Check assert conditions - must be bool
	for _, assert := range test.Asserts {
		if assert == nil {
			continue
		}

		// Determine which environment to use for this assert block:
		// - If assert has a query (assert SomeQuery() { ... }), use that query's returns
		// - Otherwise use the parent query's returns
		assertEnv := env
		if assert.Query != nil {
			if assert.Query.QueryName != nil {
				// Named query reference - use its returns
				assertEnv = buildExprEnvFromQuery(f, *assert.Query.QueryName)
			} else if assert.Query.Inline != nil {
				// Inline query - analyze it directly
				assertEnv = buildExprEnvFromInlineQuery(f, *assert.Query.Inline)
			}
		}

		// Check all conditions (handles both shorthand and block form)
		for _, cond := range assert.AllConditions() {
			if cond != nil {
				checkExpressionAtSpan(f, cond, cond.Span(), "assertion condition", boolOption(), assertEnv)
			}
		}
	}
}

// boolOption returns expr.AsBool() as a compile option.
func boolOption() expr.Option {
	return expr.AsBool()
}

// buildExprEnvFromQuery builds an expr environment map from query return types.
// The environment tells expr-lang what variables exist and their types.
// Returns nil if no query analyzer is available or query not found.
//
// This function builds typed structs (via reflection) instead of map[string]any
// to enable proper type checking. For example, if a query returns `u.name` (string),
// accessing `p.name` in an assertion will be type-checked as string, so
// `assert (p.name)` will error with "expected bool, but got string".
func buildExprEnvFromQuery(f *AnalyzedFile, queryName string) map[string]any {
	if f.QueryAnalyzer == nil {
		return nil
	}

	query, ok := f.Symbols.Queries[queryName]
	if !ok || query.Body == "" {
		return nil
	}

	metadata := analyzeQueryWithSchemaIfAvailable(f, query.Body)
	if metadata == nil {
		return nil
	}

	return buildExprEnvFromMetadata(metadata)
}

// buildExprEnvFromInlineQuery builds an expr environment from an inline query string.
// Used for assert blocks with inline queries like: assert `MATCH (c:Comment) RETURN c` { (c.id > 0) }
func buildExprEnvFromInlineQuery(f *AnalyzedFile, queryBody string) map[string]any {
	if f.QueryAnalyzer == nil || queryBody == "" {
		return nil
	}

	metadata := analyzeQueryWithSchemaIfAvailable(f, queryBody)
	if metadata == nil {
		return nil
	}

	return buildExprEnvFromMetadata(metadata)
}

// analyzeQueryWithSchemaIfAvailable analyzes a query using the schema if available.
// Falls back to basic analysis without schema if schema-aware analysis is not supported.
func analyzeQueryWithSchemaIfAvailable(f *AnalyzedFile, queryBody string) *scaf.QueryMetadata {
	// Try schema-aware analysis first if schema is available
	if f.Schema != nil {
		if schemaAnalyzer, ok := f.QueryAnalyzer.(SchemaAwareAnalyzer); ok {
			metadata, err := schemaAnalyzer.AnalyzeQueryWithSchema(queryBody, f.Schema)
			if err == nil && metadata != nil {
				return metadata
			}
		}
	}

	// Fall back to basic analysis
	metadata, err := f.QueryAnalyzer.AnalyzeQuery(queryBody)
	if err != nil {
		return nil
	}
	return metadata
}

// buildExprEnvFromMetadata builds an expr environment from query metadata.
// This is the shared implementation used by both named and inline query environment builders.
func buildExprEnvFromMetadata(metadata *scaf.QueryMetadata) map[string]any {
	// Build environment from return fields
	env := make(map[string]any)

	// Track entity prefixes for property access (e.g., "u" from "u.name")
	// Maps prefix -> field name -> typed value
	entityPrefixes := make(map[string]map[string]any)

	for _, ret := range metadata.Returns {
		// Use the field name (or alias if present) as the variable name
		name := ret.Name
		if ret.Alias != "" {
			name = ret.Alias
		}

		// Map the return type to a Go type for expr-lang
		typeValue := typeToExprValue(ret.Type)
		env[name] = typeValue

		// If the expression is a property access (e.g., "u.name"), also add:
		// 1. The full expression as a variable (for direct access like "u.name")
		// 2. The entity prefix with nested properties (for access like "u.name" via u)
		if ret.Alias == "" && ret.Expression != "" && strings.Contains(ret.Expression, ".") {
			parts := strings.SplitN(ret.Expression, ".", 2)
			if len(parts) == 2 {
				prefix := parts[0]
				propPath := parts[1]

				// Initialize entity map if not exists
				if _, ok := entityPrefixes[prefix]; !ok {
					entityPrefixes[prefix] = make(map[string]any)
				}

				// Add the property to the entity map
				// Handle nested properties (e.g., "user.address.city")
				addNestedProperty(entityPrefixes[prefix], propPath, typeValue)
			}
		}
	}

	// Convert entity prefix maps to typed structs for proper type checking
	// This enables expr-lang to detect type mismatches like "assert (p.name)" where p.name is string
	for prefix, props := range entityPrefixes {
		// Only add if not already in env (don't override explicit returns)
		if _, exists := env[prefix]; !exists {
			env[prefix] = buildTypedStruct(props)
		}
	}

	// If no returns were found, return nil to skip validation
	if len(env) == 0 {
		return nil
	}

	return env
}

// addNestedProperty adds a property value to a nested map structure.
// For "address.city", it creates {"address": {"city": value}}.
func addNestedProperty(m map[string]any, path string, value any) {
	parts := strings.SplitN(path, ".", 2)
	if len(parts) == 1 {
		// Leaf property
		m[parts[0]] = value
	} else {
		// Nested property - ensure intermediate map exists
		if _, ok := m[parts[0]]; !ok {
			m[parts[0]] = make(map[string]any)
		}
		if nested, ok := m[parts[0]].(map[string]any); ok {
			addNestedProperty(nested, parts[1], value)
		}
	}
}

// buildTypedStruct dynamically creates a struct type with the given fields and their types.
// Each field gets an `expr:"fieldname"` tag to allow lowercase access in expressions.
// This enables expr-lang to perform proper type checking instead of treating everything as `any`.
func buildTypedStruct(fields map[string]any) any {
	if len(fields) == 0 {
		return struct{}{}
	}

	// Build struct fields
	structFields := make([]reflect.StructField, 0, len(fields))
	for name, value := range fields {
		// Capitalize first letter for Go export requirement
		exportedName := capitalizeFirst(name)

		var fieldType reflect.Type
		if value == nil {
			// Unknown type - use interface{} which expr-lang will check as "unknown"
			fieldType = reflect.TypeOf((*any)(nil)).Elem()
		} else if nestedMap, ok := value.(map[string]any); ok {
			// Recursive: build nested struct for nested properties
			nested := buildTypedStruct(nestedMap)
			fieldType = reflect.TypeOf(nested)
		} else {
			fieldType = reflect.TypeOf(value)
		}

		structFields = append(structFields, reflect.StructField{
			Name: exportedName,
			Type: fieldType,
			// Use expr tag for lowercase access (e.g., `expr:"name"` allows p.name)
			Tag: reflect.StructTag(fmt.Sprintf(`expr:"%s"`, name)),
		})
	}

	// Create the struct type and instance
	structType := reflect.StructOf(structFields)
	structValue := reflect.New(structType).Elem()

	// Set values (needed for expr-lang to infer types)
	for name, value := range fields {
		field := structValue.FieldByName(capitalizeFirst(name))
		if field.IsValid() && field.CanSet() {
			if nestedMap, ok := value.(map[string]any); ok {
				nested := buildTypedStruct(nestedMap)
				field.Set(reflect.ValueOf(nested))
			} else if value != nil {
				field.Set(reflect.ValueOf(value))
			}
		}
	}

	return structValue.Interface()
}

// capitalizeFirst returns s with the first character uppercased.
// Required for Go struct field names which must be exported.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	// Only capitalize if first char is lowercase letter
	if s[0] >= 'a' && s[0] <= 'z' {
		return string(s[0]-32) + s[1:]
	}
	return s
}

// unknownTypeMarker is a sentinel value for unknown/untyped fields.
// Using a non-nil empty interface value allows expr-lang to check as "unknown" type,
// which is stricter than map[string]any{} (which allows any property access).
var unknownTypeMarker = any("")

// typeToExprValue converts a Type to a representative value for expr.Env().
// expr-lang infers types from the values in the environment.
func typeToExprValue(t *scaf.Type) any {
	if t == nil {
		// For unknown types, return empty string as marker.
		// This is intentionally NOT map[string]any{} because that would allow
		// any property access. Using "" means the type is inferred as string,
		// which will give better error messages than "unknown" or "any".
		// Note: We use string because most database properties are strings.
		return unknownTypeMarker
	}

	switch t.Kind {
	case scaf.TypeKindPrimitive:
		switch t.Name {
		case "string":
			return ""
		case "int", "int8", "int16", "int32", "int64":
			return int64(0)
		case "uint", "uint8", "uint16", "uint32", "uint64":
			return uint64(0)
		case "float32", "float64":
			return float64(0)
		case "bool":
			return false
		default:
			return map[string]any{}
		}
	case scaf.TypeKindSlice:
		// Return an empty slice of the appropriate type
		if t.Elem != nil {
			switch t.Elem.Kind {
			case scaf.TypeKindPrimitive:
				switch t.Elem.Name {
				case "string":
					return []string{}
				case "int", "int8", "int16", "int32", "int64":
					return []int64{}
				case "float32", "float64":
					return []float64{}
				case "bool":
					return []bool{}
				}
			}
		}
		return []any{}
	case scaf.TypeKindMap:
		return map[string]any{}
	case scaf.TypeKindPointer:
		// For pointer types (like *User), return a map to allow property access
		return map[string]any{}
	case scaf.TypeKindNamed:
		// For named types, return a map to allow property access
		return map[string]any{}
	default:
		return map[string]any{}
	}
}

// checkExpressionAtSpan validates a ParenExpr and reports diagnostics at the given span.
// The context parameter describes where the expression appears (for error messages).
// If typeOpt is non-nil, the expression is expected to return that type.
// env provides type information for variables from query returns (required for validation).
func checkExpressionAtSpan(f *AnalyzedFile, parenExpr *scaf.ParenExpr, span scaf.Span, context string, typeOpt expr.Option, env map[string]any) {
	if parenExpr == nil {
		return
	}

	exprStr := parenExpr.String()
	if strings.TrimSpace(exprStr) == "" {
		return
	}

	// If no environment is available, we can't properly validate variable references
	// Skip validation rather than allowing undefined variables
	if env == nil {
		return
	}

	// Build compile options with strict environment checking
	opts := []expr.Option{expr.Env(env)}
	if typeOpt != nil {
		opts = append(opts, typeOpt)
	}

	// Try to compile the expression with expr-lang.
	_, err := expr.Compile(exprStr, opts...)
	if err == nil {
		return
	}

	// Extract location info from expr-lang error
	diag := exprErrorToDiagnostic(err, parenExpr, span, context)
	f.Diagnostics = append(f.Diagnostics, diag)
}

// checkTypedExpressionAtSpan validates an expression that should return a specific type.
// This is used for parameter expressions where the type is declared in the function signature.
// env provides type information for variables from query returns (required for validation).
func checkTypedExpressionAtSpan(f *AnalyzedFile, parenExpr *scaf.ParenExpr, span scaf.Span, expectedType *scaf.TypeExpr, context string, env map[string]any) {
	if parenExpr == nil || expectedType == nil {
		return
	}

	exprStr := parenExpr.String()
	if strings.TrimSpace(exprStr) == "" {
		return
	}

	// If no environment is available, we can't properly validate
	if env == nil {
		return
	}

	// Build compile options with strict environment checking
	opts := []expr.Option{expr.Env(env)}

	// Map scaf types to expr type options
	typeOpt := typeExprToExprOption(expectedType)
	if typeOpt != nil {
		opts = append(opts, typeOpt)
	}

	// Try to compile the expression with expr-lang.
	_, err := expr.Compile(exprStr, opts...)
	if err == nil {
		return
	}

	// Extract location info from expr-lang error
	diag := exprErrorToDiagnostic(err, parenExpr, span, context)
	f.Diagnostics = append(f.Diagnostics, diag)
}

// typeExprToExprOption converts a scaf TypeExpr to an expr.Option for type checking.
// Returns nil if the type cannot be mapped to an expr type check (e.g., complex types).
func typeExprToExprOption(t *scaf.TypeExpr) expr.Option {
	if t == nil {
		return nil
	}

	// For nullable types, we can't easily enforce the base type since nil is valid
	// Just check the base type - runtime will handle nil
	if t.Simple != nil {
		switch *t.Simple {
		case "bool":
			return expr.AsBool()
		case "int", "int64", "int32":
			return expr.AsInt64()
		case "float", "float64", "float32":
			return expr.AsFloat64()
		case "string":
			// expr doesn't have AsString, but type mismatches will still error
			return nil
		case "any":
			return expr.AsAny()
		}
	}

	// For arrays, maps, and complex types, we can't easily enforce at compile time
	// Runtime type checking will handle these
	return nil
}

// exprErrorToDiagnostic converts an expr-lang error to a scaf diagnostic.
// It extracts position information from file.Error if available.
func exprErrorToDiagnostic(err error, parenExpr *scaf.ParenExpr, span scaf.Span, context string) Diagnostic {
	var exprErr *exprfile.Error
	if errors.As(err, &exprErr) {
		// expr-lang provides detailed error info with line/column
		// The position is relative to the expression string, so we need to adjust
		adjustedSpan := adjustExprErrorSpan(exprErr, parenExpr, span)

		return Diagnostic{
			Span:     adjustedSpan,
			Severity: SeverityError,
			Message:  friendlyExprError(exprErr.Message),
			Code:     "invalid-expression",
			Source:   "scaf",
		}
	}

	// Fallback: use the whole expression span
	return Diagnostic{
		Span:     span,
		Severity: SeverityError,
		Message:  context + ": " + friendlyExprError(err.Error()),
		Code:     "invalid-expression",
		Source:   "scaf",
	}
}

// friendlyExprError transforms common expr-lang error patterns into friendlier messages.
// This makes error messages more actionable for scaf users.
func friendlyExprError(msg string) string {
	// Pattern: "undefined: foo" → "undefined variable 'foo' - check query RETURN clause"
	if strings.HasPrefix(msg, "undefined: ") {
		varName := strings.TrimPrefix(msg, "undefined: ")
		return fmt.Sprintf("undefined variable '%s' - check query RETURN clause", varName)
	}

	// Pattern: "unknown name foo" → "undefined variable 'foo' - check query RETURN clause"
	if strings.HasPrefix(msg, "unknown name ") {
		varName := strings.TrimPrefix(msg, "unknown name ")
		return fmt.Sprintf("undefined variable '%s' - check query RETURN clause", varName)
	}

	// Pattern: "invalid operation: <type> <op> <type>" → "cannot compare <type> with <type>"
	if strings.HasPrefix(msg, "invalid operation: ") {
		rest := strings.TrimPrefix(msg, "invalid operation: ")
		// Try to parse patterns like "string > int" or "int == string"
		for _, op := range []string{" > ", " < ", " >= ", " <= ", " == ", " != ", " + ", " - ", " * ", " / "} {
			if idx := strings.Index(rest, op); idx > 0 {
				leftType := rest[:idx]
				rightType := rest[idx+len(op):]
				// Check for "(1:1)" style position suffix and remove it
				if parenIdx := strings.Index(rightType, " ("); parenIdx > 0 {
					rightType = rightType[:parenIdx]
				}
				return fmt.Sprintf("cannot compare %s with %s", leftType, rightType)
			}
		}
		// Fallback if we can't parse
		return msg
	}

	// Pattern: "expected bool, but got <type>" → more helpful message
	if strings.HasPrefix(msg, "expected bool, but got ") {
		gotType := strings.TrimPrefix(msg, "expected bool, but got ")
		// Remove any position suffix
		if parenIdx := strings.Index(gotType, " ("); parenIdx > 0 {
			gotType = gotType[:parenIdx]
		}
		if gotType == "string" {
			return fmt.Sprintf("assertion must be boolean, got string (e.g., use 'name != \"\"' instead of 'name')")
		}
		if gotType == "int" || gotType == "int64" || gotType == "float64" {
			return fmt.Sprintf("assertion must be boolean, got %s (e.g., use 'count > 0' instead of 'count')", gotType)
		}
		return fmt.Sprintf("assertion must be boolean, got %s", gotType)
	}

	// Pattern: "expected int, but got <type>" for typed parameters
	if strings.HasPrefix(msg, "expected int, but got ") || strings.HasPrefix(msg, "expected int64, but got ") {
		return strings.Replace(msg, "expected ", "expression must return ", 1)
	}

	return msg
}

// adjustExprErrorSpan calculates the actual source span for an expr-lang error.
// The expr-lang error position is relative to the expression string, but we need
// the position in the source file. We adjust by adding the expression start position.
func adjustExprErrorSpan(exprErr *exprfile.Error, parenExpr *scaf.ParenExpr, span scaf.Span) scaf.Span {
	// If we have line/column from expr-lang, calculate adjusted position
	// Note: expr-lang's Line is 1-based, Column is 0-based
	if exprErr.Line > 0 {
		// For single-line expressions (most common case), adjust column
		if exprErr.Line == 1 {
			// Add 1 for the opening paren, then add the column offset
			adjustedCol := span.Start.Column + 1 + exprErr.Column
			return scaf.Span{
				Start: lexer.Position{
					Line:   span.Start.Line,
					Column: adjustedCol,
				},
				End: lexer.Position{
					Line:   span.Start.Line,
					Column: adjustedCol + 1, // Point to roughly 1 char
				},
			}
		}

		// Multi-line expression: adjust line, use column as-is
		adjustedLine := span.Start.Line + exprErr.Line - 1
		return scaf.Span{
			Start: lexer.Position{
				Line:   adjustedLine,
				Column: exprErr.Column,
			},
			End: lexer.Position{
				Line:   adjustedLine,
				Column: exprErr.Column + 1,
			},
		}
	}

	// No detailed position info, use the full span
	return span
}

// ----------------------------------------------------------------------------
// Rule: invalid-type-annotation
// ----------------------------------------------------------------------------

var invalidTypeAnnotationRule = &Rule{
	Name:     "invalid-type-annotation",
	Doc:      "Reports invalid type names in function parameter annotations.",
	Severity: SeverityError,
	Run:      checkInvalidTypeAnnotations,
}

func checkInvalidTypeAnnotations(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	for _, fn := range f.Suite.Functions {
		if fn == nil {
			continue
		}

		for _, param := range fn.Params {
			if param == nil || param.Type == nil {
				continue
			}

			validateTypeExpr(f, param.Type)
		}
	}
}

// validateTypeExpr validates a type expression recursively.
// Scaf only allows primitive types, not named types like Go does.
func validateTypeExpr(f *AnalyzedFile, t *scaf.TypeExpr) {
	if t == nil {
		return
	}

	switch {
	case t.Simple != nil:
		if !isValidScafType(*t.Simple) {
			f.Diagnostics = append(f.Diagnostics, Diagnostic{
				Span:     t.Span(),
				Severity: SeverityError,
				Message:  "unknown type: " + *t.Simple,
				Code:     "invalid-type-annotation",
				Source:   "scaf",
			})
		}

	case t.Array != nil:
		validateTypeExpr(f, t.Array)

	case t.Map != nil:
		validateTypeExpr(f, t.Map.Key)
		validateTypeExpr(f, t.Map.Value)
	}
}

// isValidScafType returns true if typeName is a valid scaf DSL type.
// Scaf uses a subset of Go primitive types.
func isValidScafType(typeName string) bool {
	switch typeName {
	case "string", "int", "int32", "int64", "float32", "float64", "bool", "any":
		return true
	}
	return false
}

// ----------------------------------------------------------------------------
// Rule: return-type-mismatch
// ----------------------------------------------------------------------------

var returnTypeMismatchRule = &Rule{
	Name:     "return-type-mismatch",
	Doc:      "Reports test statements with values that don't match the query's return type from schema inference.",
	Severity: SeverityError,
	Run:      checkReturnTypeMismatch,
}

func checkReturnTypeMismatch(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	for _, scope := range f.Suite.Scopes {
		query, ok := f.Symbols.Queries[scope.FunctionName]
		if !ok {
			continue
		}

		// Build map of return field types from query analysis
		// Key is "varName.propName" (e.g., "u.name") -> inferred type
		returnTypes := buildReturnTypeMap(query.QueryBodyReturns)
		if len(returnTypes) == 0 {
			continue
		}

		checkItemReturnTypes(f, scope.Items, returnTypes, scope.FunctionName)
	}
}

// buildReturnTypeMap builds a map from field path (e.g., "u.name") to inferred type.
// Handles both simple returns (u) and property access returns (u.name).
func buildReturnTypeMap(returns []scaf.ReturnInfo) map[string]*scaf.Type {
	result := make(map[string]*scaf.Type)

	for _, ret := range returns {
		if ret.Type == nil {
			continue
		}

		// Expression contains the original expression (e.g., "u.name")
		// Name contains the inferred column name (e.g., "name" for "u.name", or alias if AS used)
		// We primarily want to match by Expression since that's how test statements are written

		// Index by expression (e.g., "u.name") - primary key for test statements
		if ret.Expression != "" {
			result[ret.Expression] = ret.Type
		}

		// Also index by alias/name if present (e.g., "userName" for "u.name AS userName")
		if ret.Alias != "" {
			result[ret.Alias] = ret.Type
		}
	}

	return result
}

func checkItemReturnTypes(f *AnalyzedFile, items []*scaf.TestOrGroup, returnTypes map[string]*scaf.Type, queryName string) {
	for _, item := range items {
		if item.Test != nil {
			for _, stmt := range item.Test.Statements {
				key := stmt.Key()

				// Skip input parameters ($param)
				if strings.HasPrefix(key, "$") {
					continue
				}

				// Look up the expected type for this return field
				expectedType, hasType := returnTypes[key]
				if !hasType || expectedType == nil {
					continue // Unknown field, skip
				}

				if stmt.Value == nil || stmt.Value.Literal == nil {
					continue // No literal value to check (expressions are runtime-evaluated)
				}

				// Allow null for any return field - the query might not return rows,
				// or the field might be nullable in the database
				if stmt.Value.Literal.Null {
					continue
				}

				if err := checkValueMatchesInferredType(stmt.Value.Literal, expectedType); err != nil {
					f.Diagnostics = append(f.Diagnostics, Diagnostic{
						Span:     stmt.Span(),
						Severity: SeverityError,
						Message:  "type mismatch for " + key + ": " + err.Error(),
						Code:     "return-type-mismatch",
						Source:   "scaf",
					})
				}
			}
		}

		if item.Group != nil {
			checkItemReturnTypes(f, item.Group.Items, returnTypes, queryName)
		}
	}
}

// ----------------------------------------------------------------------------
// Rule: same-package-import
// ----------------------------------------------------------------------------

var samePackageImportRule = &Rule{
	Name:     "same-package-import",
	Doc:      "Warns when an import points to a sibling .scaf file in the same directory.",
	Severity: SeverityWarning,
	Run:      checkSamePackageImport,
}

func checkSamePackageImport(f *AnalyzedFile) {
	if f.Suite == nil || len(f.SiblingPaths) == 0 {
		return
	}

	// Build set of sibling absolute paths
	siblingSet := make(map[string]bool)
	for _, p := range f.SiblingPaths {
		abs, err := filepath.Abs(p)
		if err == nil {
			siblingSet[abs] = true
		}
	}

	fileDir := filepath.Dir(f.Path)

	for _, imp := range f.Suite.Imports {
		resolved := resolveImportPath(imp.Path, fileDir)
		if siblingSet[resolved] {
			f.Diagnostics = append(f.Diagnostics, Diagnostic{
				Span:     imp.Span(),
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("import %q resolves to sibling file in same package", imp.Path),
				Code:     "same-package-import",
				Source:   "scaf",
			})
		}
	}
}

// resolveImportPath resolves an import path relative to the file directory.
func resolveImportPath(importPath, fileDir string) string {
	if filepath.IsAbs(importPath) {
		return filepath.Clean(importPath)
	}
	if fileDir == "" {
		return importPath
	}
	resolved := filepath.Join(fileDir, importPath)
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return resolved
	}
	cleaned := filepath.Clean(abs)
	if filepath.Ext(cleaned) == "" {
		return cleaned + ".scaf"
	}
	return cleaned
}
