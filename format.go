package scaf

import (
	"strconv"
	"strings"
)

const (
	// DefaultMaxLineWidth is the target line width for smart splitting.
	DefaultMaxLineWidth = 100
	// MinWidthForSplit is the minimum width before considering splitting.
	MinWidthForSplit = 40
)

// Format formats a Suite AST back into scaf DSL source code, preserving comments.
func Format(s *Suite) string {
	return FormatWithWidth(s, DefaultMaxLineWidth)
}

// FormatWithWidth formats a Suite AST with a specific target line width.
func FormatWithWidth(s *Suite, maxWidth int) string {
	var b strings.Builder

	f := &formatter{b: &b, indent: 0, maxWidth: maxWidth}
	f.formatSuite(s)

	return strings.TrimSpace(b.String()) + "\n"
}

type formatter struct {
	b        *strings.Builder
	indent   int
	maxWidth int
}

func (f *formatter) write(s string) {
	f.b.WriteString(s)
}

func (f *formatter) writeLine(s string) {
	f.writeIndent()
	f.write(s)
	f.write("\n")
}

func (f *formatter) writeIndent() {
	for range f.indent {
		f.write("\t")
	}
}

func (f *formatter) blankLine() {
	f.write("\n")
}

// currentLineWidth returns the approximate current line width including indent.
func (f *formatter) currentLineWidth() int {
	return f.indent * 4 // Approximate tab width
}

// wouldExceedWidth checks if adding content would exceed max width.
func (f *formatter) wouldExceedWidth(content string) bool {
	return f.currentLineWidth()+len(content) > f.maxWidth
}

// writeLeadingComments writes any leading comments.
func (f *formatter) writeLeadingComments(leading []string) {
	for _, comment := range leading {
		f.writeLine(comment)
	}
}

// writeTrailingComment appends a trailing comment to the current line if one exists.
func (f *formatter) writeTrailingComment(trailing string) {
	if trailing != "" {
		f.write(" " + trailing)
	}
}

func (f *formatter) formatSuite(s *Suite) {
	// Leading comments for the whole file
	f.writeLeadingComments(s.LeadingComments)

	// Imports
	for _, imp := range s.Imports {
		f.formatImport(imp)
	}

	// Queries
	for i, q := range s.Functions {
		if i > 0 || len(s.Imports) > 0 {
			f.blankLine()
		}

		f.formatQuery(q)
	}

	// Global setup
	if s.Setup != nil {
		if len(s.Functions) > 0 || len(s.Imports) > 0 {
			f.blankLine()
		}

		f.formatSetupClause(s.Setup)
	}

	// Global teardown
	if s.Teardown != nil {
		f.formatTeardown(*s.Teardown)
	}

	// Scopes
	for i, scope := range s.Scopes {
		if i > 0 || len(s.Functions) > 0 || len(s.Imports) > 0 || s.Setup != nil || s.Teardown != nil {
			f.blankLine()
		}

		f.formatScope(scope)
	}
}

func (f *formatter) formatImport(imp *Import) {
	f.writeLeadingComments(imp.LeadingComments)

	if imp.Alias != nil {
		f.writeIndent()
		f.write("import " + *imp.Alias + " " + f.quotedString(imp.Path))
	} else {
		f.writeIndent()
		f.write("import " + f.quotedString(imp.Path))
	}

	f.writeTrailingComment(imp.TrailingComment)
	f.write("\n")
}

func (f *formatter) formatQuery(q *Function) {
	f.writeLeadingComments(q.LeadingComments)
	f.writeIndent()

	// Trailing comma controls formatting: present = multi-line, absent = single-line
	if q.TrailingComma {
		f.formatQueryMultiLine(q)
	} else {
		f.write(f.formatQuerySingleLine(q))
	}

	f.writeTrailingComment(q.TrailingComment)
	f.write("\n")
}

func (f *formatter) formatQuerySingleLine(q *Function) string {
	var b strings.Builder
	b.WriteString("fn ")
	b.WriteString(q.Name)
	b.WriteString("(")

	for i, p := range q.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(f.formatParam(p))
	}

	b.WriteString(") ")
	b.WriteString(f.rawString(q.Body))

	return b.String()
}

func (f *formatter) formatQueryMultiLine(q *Function) {
	f.write("fn ")
	f.write(q.Name)
	f.write("(")

	if len(q.Params) > 0 {
		f.write("\n")
		f.indent++

		for _, p := range q.Params {
			f.writeIndent()
			f.write(f.formatParam(p))
			f.write(",")
			f.write("\n")
		}

		f.indent--
		f.writeIndent()
	}

	f.write(") ")
	f.write(f.rawString(q.Body))
}

func (f *formatter) formatParam(p *FnParam) string {
	if p.Type == nil {
		return p.Name
	}
	return p.Name + ": " + f.formatTypeExpr(p.Type)
}

func (f *formatter) formatTypeExpr(t *TypeExpr) string {
	if t == nil {
		return "any"
	}

	var base string
	switch {
	case t.Simple != nil:
		base = *t.Simple
	case t.Array != nil:
		base = "[" + f.formatTypeExpr(t.Array) + "]"
	case t.Map != nil:
		base = "{" + f.formatTypeExpr(t.Map.Key) + ": " + f.formatTypeExpr(t.Map.Value) + "}"
	default:
		base = "any"
	}

	if t.Nullable {
		base += "?"
	}
	return base
}

func (f *formatter) formatSetupClause(s *SetupClause) {
	switch {
	case s.Inline != nil:
		f.writeLine("setup " + f.rawString(*s.Inline))
	case s.Module != nil:
		f.writeLine("setup " + *s.Module)
	case s.Call != nil:
		f.formatSetupCallLine(s.Call)
	case len(s.Block) > 0:
		f.formatSetupBlock(s.Block)
	}
}

func (f *formatter) formatSetupCallLine(c *SetupCall) {
	// Trailing comma controls formatting: present = multi-line, absent = single-line
	if c.TrailingComma {
		f.formatSetupCallMultiLine(c)
	} else {
		f.writeLine("setup " + f.formatSetupCallSingleLine(c))
	}
}

func (f *formatter) formatSetupBlock(items []*SetupItem) {
	if len(items) == 1 {
		// Try single line format
		singleLine := "setup { " + f.formatSetupItem(items[0]) + " }"
		if !f.wouldExceedWidth(singleLine) {
			f.writeLine(singleLine)
			return
		}
	}

	// Multiple items or too long - block format
	f.writeLine("setup {")
	f.indent++

	for _, item := range items {
		f.writeLine(f.formatSetupItem(item))
	}

	f.indent--
	f.writeLine("}")
}

func (f *formatter) formatSetupItem(item *SetupItem) string {
	if item.Inline != nil {
		return f.rawString(*item.Inline)
	}

	if item.Module != nil {
		return *item.Module
	}

	if item.Call != nil {
		return f.formatSetupCallSingleLine(item.Call)
	}

	return ""
}

func (f *formatter) formatSetupCallSingleLine(c *SetupCall) string {
	var b strings.Builder

	b.WriteString(c.Module)
	b.WriteString(".")
	b.WriteString(c.Query)
	b.WriteString("(")

	for i, p := range c.Params {
		if i > 0 {
			b.WriteString(", ")
		}

		b.WriteString(p.Name)
		b.WriteString(": ")
		b.WriteString(f.formatParamValue(p.Value))
	}

	b.WriteString(")")

	return b.String()
}

func (f *formatter) formatSetupCallMultiLine(c *SetupCall) {
	f.writeIndent()
	f.write("setup ")
	f.write(c.Module)
	f.write(".")
	f.write(c.Query)
	f.write("(")

	if len(c.Params) > 0 {
		f.write("\n")
		f.indent++

		for _, p := range c.Params {
			f.writeIndent()
			f.write(p.Name)
			f.write(": ")
			f.write(f.formatParamValue(p.Value))
			f.write(",")
			f.write("\n")
		}

		f.indent--
		f.writeIndent()
	}

	f.write(")\n")
}

func (f *formatter) formatTeardown(body string) {
	f.writeLine("teardown " + f.rawString(body))
}

func (f *formatter) formatScope(s *QueryScope) {
	f.writeLeadingComments(s.LeadingComments)
	f.writeLine(s.FunctionName + " {")
	f.indent++

	if s.Setup != nil {
		f.formatSetupClause(s.Setup)
	}

	if s.Teardown != nil {
		f.formatTeardown(*s.Teardown)
	}

	f.formatItems(s.Items, s.Setup != nil || s.Teardown != nil)

	f.indent--
	f.writeLine("}")
}

func (f *formatter) formatItems(items []*TestOrGroup, hasSetupOrTeardown bool) {
	for i, item := range items {
		needsBlank := i > 0 || hasSetupOrTeardown

		if item.Test != nil {
			if needsBlank {
				f.blankLine()
			}

			f.formatTest(item.Test)
		} else if item.Group != nil {
			if needsBlank {
				f.blankLine()
			}

			f.formatGroup(item.Group)
		}
	}
}

func (f *formatter) formatGroup(g *Group) {
	f.writeLeadingComments(g.LeadingComments)
	f.writeLine("group " + f.quotedString(g.Name) + " {")
	f.indent++

	if g.Setup != nil {
		f.formatSetupClause(g.Setup)
	}

	if g.Teardown != nil {
		f.formatTeardown(*g.Teardown)
	}

	f.formatItems(g.Items, g.Setup != nil || g.Teardown != nil)

	f.indent--
	f.writeLine("}")
}

func (f *formatter) formatTest(t *Test) {
	f.writeLeadingComments(t.LeadingComments)
	f.writeLine("test " + f.quotedString(t.Name) + " {")
	f.indent++

	if t.Setup != nil {
		f.formatSetupClause(t.Setup)
	}

	// Separate inputs from outputs
	var inputs, outputs []*Statement

	for _, stmt := range t.Statements {
		if strings.HasPrefix(stmt.Key(), "$") {
			inputs = append(inputs, stmt)
		} else {
			outputs = append(outputs, stmt)
		}
	}

	// Format inputs
	for i, stmt := range inputs {
		if i == 0 && t.Setup != nil {
			f.blankLine()
		}

		f.formatStatement(stmt)
	}

	// Format outputs with blank line separator from inputs
	for i, stmt := range outputs {
		if i == 0 && len(inputs) > 0 {
			f.blankLine()
		}

		f.formatStatement(stmt)
	}

	// Assertions
	for i, a := range t.Asserts {
		if i == 0 && (len(t.Statements) > 0 || t.Setup != nil) {
			f.blankLine()
		}

		f.formatAssert(a)
	}

	f.indent--
	f.writeLine("}")
}

func (f *formatter) formatStatement(s *Statement) {
	// Check if value needs multi-line (trailing comma present)
	valueSingle := f.formatStatementValueSingleLine(s.Value)

	// Empty string means trailing comma forces multi-line
	if valueSingle == "" {
		f.formatStatementMultiLine(s)
	} else {
		f.writeLine(s.Key() + ": " + valueSingle)
	}
}

func (f *formatter) formatStatementMultiLine(s *Statement) {
	f.writeIndent()
	f.write(s.Key())
	f.write(": ")

	sv := s.Value
	if sv == nil {
		f.write("null\n")
		return
	}

	// Handle expressions
	if sv.IsExpr() {
		f.write("(" + sv.Expr.String() + ")")
		if sv.HasWhere() {
			f.write(" where (" + sv.Where.String() + ")")
		}
		f.write("\n")
		return
	}

	// Handle literals
	lit := sv.Literal
	if lit == nil {
		f.write("null")
		if sv.HasWhere() {
			f.write(" where (" + sv.Where.String() + ")")
		}
		f.write("\n")
		return
	}

	// Check if value is a collection that can be split
	switch {
	case lit.Map != nil && len(lit.Map.Entries) > 0:
		f.formatMapMultiLine(lit.Map)
		if sv.HasWhere() {
			f.write(" where (" + sv.Where.String() + ")")
		}
		f.write("\n")
	case lit.List != nil && len(lit.List.Values) > 0:
		f.formatListMultiLine(lit.List)
		if sv.HasWhere() {
			f.write(" where (" + sv.Where.String() + ")")
		}
		f.write("\n")
	default:
		// Fall back to single line for primitives
		f.write(f.formatValueSingleLine(lit))
		if sv.HasWhere() {
			f.write(" where (" + sv.Where.String() + ")")
		}
		f.write("\n")
	}
}

// formatStatementValueSingleLine formats a StatementValue on a single line.
// Returns empty string if value has trailing comma (needs multi-line).
func (f *formatter) formatStatementValueSingleLine(sv *StatementValue) string {
	if sv == nil {
		return "null"
	}

	var b strings.Builder

	// Value part: literal or expression
	switch {
	case sv.IsExpr():
		b.WriteString("(")
		b.WriteString(sv.Expr.String())
		b.WriteString(")")
	case sv.Literal != nil:
		valueSingle := f.formatValueSingleLine(sv.Literal)
		if valueSingle == "" {
			return "" // Trailing comma forces multi-line
		}
		b.WriteString(valueSingle)
	default:
		b.WriteString("null")
	}

	// Where clause
	if sv.HasWhere() {
		b.WriteString(" where (")
		b.WriteString(sv.Where.String())
		b.WriteString(")")
	}

	return b.String()
}

func (f *formatter) formatAssert(a *Assert) {
	// Handle shorthand form: assert (expr)
	if a.IsShorthand() {
		f.writeLine("assert (" + a.Shorthand.String() + ")")
		return
	}

	var queryPart string
	if a.Query != nil {
		if a.Query.Inline != nil {
			queryPart = f.rawString(*a.Query.Inline) + " "
		} else if a.Query.QueryName != nil {
			queryPart = *a.Query.QueryName
			// If trailing comma, format params multi-line
			if a.Query.TrailingComma && len(a.Query.Params) > 0 {
				f.writeIndent()
				f.write("assert " + queryPart + "(\n")
				f.indent++
				for _, p := range a.Query.Params {
					f.writeIndent()
					f.write(p.Name + ": " + f.formatParamValue(p.Value) + ",\n")
				}
				f.indent--
				f.writeIndent()
				f.write(") ")
				f.formatAssertConditions(a.Conditions)
				return
			}
			if len(a.Query.Params) > 0 {
				var params []string
				for _, p := range a.Query.Params {
					params = append(params, p.Name+": "+f.formatParamValue(p.Value))
				}

				queryPart += "(" + strings.Join(params, ", ") + ") "
			} else {
				queryPart += "() "
			}
		}
	}

	if len(a.Conditions) == 0 {
		f.writeLine("assert " + queryPart + "{}")
		return
	}

	// For single condition without query, always use shorthand form
	if len(a.Conditions) == 1 && queryPart == "" {
		f.writeLine("assert (" + a.Conditions[0].String() + ")")
		return
	}

	// Try single line format with braces - conditions wrapped in parens (for assertions with query)
	if len(a.Conditions) == 1 {
		singleLine := "assert " + queryPart + "{ (" + a.Conditions[0].String() + ") }"
		if !f.wouldExceedWidth(singleLine) {
			f.writeLine(singleLine)
			return
		}
	}

	// Multi-line format - each condition on its own line, wrapped in parens
	f.writeLine("assert " + queryPart + "{")
	f.indent++

	for _, cond := range a.Conditions {
		f.writeLine("(" + cond.String() + ")")
	}

	f.indent--
	f.writeLine("}")
}

func (f *formatter) formatAssertConditions(conditions []*ParenExpr) {
	if len(conditions) == 0 {
		f.write("{}\n")
		return
	}

	// Try single line format - conditions wrapped in parens
	if len(conditions) == 1 {
		singleLine := "{ (" + conditions[0].String() + ") }"
		if !f.wouldExceedWidth(singleLine) {
			f.write(singleLine + "\n")
			return
		}
	}

	// Multi-line format
	f.write("{\n")
	f.indent++

	for _, cond := range conditions {
		f.writeLine("(" + cond.String() + ")")
	}

	f.indent--
	f.writeLine("}")
}

func (f *formatter) formatValue(v *Value) string {
	return f.formatValueSingleLine(v)
}

func (f *formatter) formatValueSingleLine(v *Value) string {
	switch {
	case v.Null:
		return "null"
	case v.Str != nil:
		return f.quotedString(*v.Str)
	case v.Number != nil:
		return f.formatNumber(*v.Number)
	case v.Boolean != nil:
		return strconv.FormatBool(bool(*v.Boolean))
	case v.Map != nil:
		result := f.formatMapSingleLine(v.Map)
		if result == "" {
			return "" // Trailing comma forces multi-line
		}
		return result
	case v.List != nil:
		result := f.formatListSingleLine(v.List)
		if result == "" {
			return "" // Trailing comma forces multi-line
		}
		return result
	default:
		return "null"
	}
}

func (f *formatter) formatParamValue(v *ParamValue) string {
	if v.IsFieldRef() {
		return v.FieldRefString()
	}

	if v.Literal != nil {
		return f.formatValue(v.Literal)
	}

	return "null"
}

func (f *formatter) formatNumber(n float64) string {
	if n == float64(int64(n)) {
		return strconv.FormatInt(int64(n), 10)
	}

	return strconv.FormatFloat(n, 'f', -1, 64)
}

func (f *formatter) formatMapSingleLine(m *Map) string {
	if len(m.Entries) == 0 {
		return "{}"
	}

	// If trailing comma is set, cannot use single-line format
	if m.TrailingComma {
		return "" // Signal to use multi-line
	}

	parts := make([]string, len(m.Entries))
	for i, e := range m.Entries {
		parts[i] = e.Key + ": " + f.formatValue(e.Value)
	}

	return "{" + strings.Join(parts, ", ") + "}"
}

func (f *formatter) formatMapMultiLine(m *Map) {
	if len(m.Entries) == 0 {
		f.write("{}")
		return
	}

	f.write("{\n")
	f.indent++

	for _, e := range m.Entries {
		f.writeIndent()
		f.write(e.Key)
		f.write(": ")

		// Check if nested value needs multi-line (trailing comma)
		valueSingle := f.formatValueSingleLine(e.Value)
		if valueSingle == "" && (e.Value.Map != nil || e.Value.List != nil) {
			if e.Value.Map != nil {
				f.formatMapMultiLine(e.Value.Map)
			} else {
				f.formatListMultiLine(e.Value.List)
			}
		} else {
			f.write(valueSingle)
		}

		f.write(",")
		f.write("\n")
	}

	f.indent--
	f.writeIndent()
	f.write("}")
}

func (f *formatter) formatListSingleLine(l *List) string {
	if len(l.Values) == 0 {
		return "[]"
	}

	// If trailing comma is set, cannot use single-line format
	if l.TrailingComma {
		return "" // Signal to use multi-line
	}

	parts := make([]string, len(l.Values))
	for i, v := range l.Values {
		parts[i] = f.formatValue(v)
	}

	return "[" + strings.Join(parts, ", ") + "]"
}

func (f *formatter) formatListMultiLine(l *List) {
	if len(l.Values) == 0 {
		f.write("[]")
		return
	}

	f.write("[\n")
	f.indent++

	for _, v := range l.Values {
		f.writeIndent()

		// Check if nested value needs multi-line (trailing comma)
		valueSingle := f.formatValueSingleLine(v)
		if valueSingle == "" && (v.Map != nil || v.List != nil) {
			if v.Map != nil {
				f.formatMapMultiLine(v.Map)
			} else {
				f.formatListMultiLine(v.List)
			}
		} else {
			f.write(valueSingle)
		}

		f.write(",")
		f.write("\n")
	}

	f.indent--
	f.writeIndent()
	f.write("]")
}

func (f *formatter) rawString(s string) string {
	return "`" + s + "`"
}

func (f *formatter) quotedString(s string) string {
	// Use Go's %q formatting which properly escapes special characters
	return strconv.Quote(s)
}

func (f *formatter) formatMap(m *Map) string {
	return f.formatMapSingleLine(m)
}

func (f *formatter) formatList(l *List) string {
	return f.formatListSingleLine(l)
}
