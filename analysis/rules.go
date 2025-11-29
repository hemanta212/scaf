package analysis

import (
	"errors"
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
		undefinedSetupQueryRule,    // Cross-file validation
		paramTypeMismatchRule,      // Type checking for function parameters
		undeclaredQueryParamRule,   // Parameters used in query body but not declared
		unknownParameterRule,       // Using a parameter that doesn't exist in the query
		duplicateTestRule,          // Duplicate test names cause conflicts
		duplicateGroupRule,         // Duplicate group names cause conflicts
		missingRequiredParamsRule,  // Missing params will cause runtime failures
		invalidExpressionRule,      // Expression syntax/type errors (compile-time)

		// Warning-level checks.
		unusedImportRule,
		unusedDeclaredParamRule, // Declared param not used in query body
		emptyGroupRule,

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
						f.Diagnostics = append(f.Diagnostics, Diagnostic{
							Span:     item.Test.Span(),
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
							f.Diagnostics = append(f.Diagnostics, Diagnostic{
								Span:     item.Test.Span(),
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
				f.Diagnostics = append(f.Diagnostics, Diagnostic{
					Span:     query.Span,
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
	Doc:      "Reports test parameters with values that don't match the function's type annotation.",
	Severity: SeverityError,
	Run:      checkParamTypeMismatch,
}

func checkParamTypeMismatch(f *AnalyzedFile) {
	if f.Suite == nil {
		return
	}

	for _, scope := range f.Suite.Scopes {
		query, ok := f.Symbols.Queries[scope.FunctionName]
		if !ok || query.TypedParams == nil {
			continue // No type annotations to check against
		}

		checkItemParamTypes(f, scope.Items, query.TypedParams, scope.FunctionName)
	}
}

func checkItemParamTypes(f *AnalyzedFile, items []*scaf.TestOrGroup, typedParams map[string]*scaf.TypeExpr, queryName string) {
	for _, item := range items {
		if item.Test != nil {
			for _, stmt := range item.Test.Statements {
				key := stmt.Key()
				paramName, isParam := strings.CutPrefix(key, "$")
				if !isParam {
					continue // Not an input parameter
				}

				expectedType, hasType := typedParams[paramName]
				if !hasType || expectedType == nil {
					continue // No type annotation for this parameter
				}

				if stmt.Value == nil || stmt.Value.Literal == nil {
					continue // No value to check (expressions are runtime-evaluated)
				}

				if err := checkValueMatchesType(stmt.Value.Literal, expectedType); err != nil {
					f.Diagnostics = append(f.Diagnostics, Diagnostic{
						Span:     stmt.Span(),
						Severity: SeverityError,
						Message:  "type mismatch for parameter $" + paramName + ": " + err.Error(),
						Code:     "param-type-mismatch",
						Source:   "scaf",
					})
				}
			}
		}

		if item.Group != nil {
			checkItemParamTypes(f, item.Group.Items, typedParams, queryName)
		}
	}
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

		// Check all conditions (handles both shorthand and block form)
		for _, cond := range assert.AllConditions() {
			if cond != nil {
				checkExpressionAtSpan(f, cond, cond.Span(), "assertion condition", boolOption(), env)
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
func buildExprEnvFromQuery(f *AnalyzedFile, queryName string) map[string]any {
	if f.QueryAnalyzer == nil {
		return nil
	}

	query, ok := f.Symbols.Queries[queryName]
	if !ok || query.Body == "" {
		return nil
	}

	metadata, err := f.QueryAnalyzer.AnalyzeQuery(query.Body)
	if err != nil || metadata == nil {
		return nil
	}

	// Build environment from return fields
	env := make(map[string]any)

	// Track entity prefixes for property access (e.g., "u" from "u.name")
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

	// Add entity prefixes to the environment
	for prefix, props := range entityPrefixes {
		// Only add if not already in env (don't override explicit returns)
		if _, exists := env[prefix]; !exists {
			env[prefix] = props
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

// typeToExprValue converts a Type to a representative value for expr.Env().
// expr-lang infers types from the values in the environment.
func typeToExprValue(t *scaf.Type) any {
	if t == nil {
		// For unknown types, use any (map) to allow property access
		return map[string]any{}
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
			Message:  exprErr.Message,
			Code:     "invalid-expression",
			Source:   "scaf",
		}
	}

	// Fallback: use the whole expression span
	return Diagnostic{
		Span:     span,
		Severity: SeverityError,
		Message:  context + ": " + err.Error(),
		Code:     "invalid-expression",
		Source:   "scaf",
	}
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