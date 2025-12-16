package scaf_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/rlch/scaf"
)

// inlineSetup creates a SetupClause with an inline query.
func inlineSetup(body string) *scaf.SetupClause {
	return &scaf.SetupClause{Inline: ptr(body)}
}

func TestFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		suite    *scaf.Suite
		expected string
	}{
		{
			name: "single query",
			suite: &scaf.Suite{
				Functions: []*scaf.Query{
					{Name: "GetUser", Body: "MATCH (u:User) RETURN u"},
				},
			},
			expected: "fn GetUser() `MATCH (u:User) RETURN u`\n",
		},
		{
			name: "multiple queries",
			suite: &scaf.Suite{
				Functions: []*scaf.Query{
					{Name: "A", Body: "A"},
					{Name: "B", Body: "B"},
				},
			},
			expected: `fn A() ` + "`A`" + `

fn B() ` + "`B`" + `
`,
		},
		{
			name: "query with global setup",
			suite: &scaf.Suite{
				Functions: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Setup:   inlineSetup("CREATE (:User)"),
			},
			expected: `fn Q() ` + "`Q`" + `

setup ` + "`CREATE (:User)`" + `
`,
		},
		{
			name: "basic scope with test",
			suite: &scaf.Suite{
				Functions: []*scaf.Query{{Name: "GetUser", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						FunctionName: "GetUser",
						Items: []*scaf.TestOrGroup{
							{Test: &scaf.Test{Name: "finds user"}},
						},
					},
				},
			},
			expected: `fn GetUser() ` + "`Q`" + `

GetUser {
	test "finds user" {
	}
}
`,
		},
		{
			name: "scope with setup",
			suite: &scaf.Suite{
				Functions: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						FunctionName: "Q",
						Setup:     inlineSetup("SCOPE SETUP"),
						Items: []*scaf.TestOrGroup{
							{Test: &scaf.Test{Name: "t"}},
						},
					},
				},
			},
			expected: `fn Q() ` + "`Q`" + `

Q {
	setup ` + "`SCOPE SETUP`" + `

	test "t" {
	}
}
`,
		},
		{
			name: "test with inputs and outputs",
			suite: &scaf.Suite{
				Functions: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						FunctionName: "Q",
						Items: []*scaf.TestOrGroup{
							{
								Test: &scaf.Test{
									Name: "test",
									Statements: []*scaf.Statement{
										scaf.NewStatement("$id", &scaf.Value{Number: ptr(1.0)}),
										scaf.NewStatement("$name", &scaf.Value{Str: ptr("alice")}),
										scaf.NewStatement("u.name", &scaf.Value{Str: ptr("Alice")}),
										scaf.NewStatement("u.age", &scaf.Value{Number: ptr(30.0)}),
									},
								},
							},
						},
					},
				},
			},
			expected: `fn Q() ` + "`Q`" + `

Q {
	test "test" {
		$id: 1
		$name: "alice"

		u.name: "Alice"
		u.age: 30
	}
}
`,
		},
		{
			name: "test with setup",
			suite: &scaf.Suite{
				Functions: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						FunctionName: "Q",
						Items: []*scaf.TestOrGroup{
							{
								Test: &scaf.Test{
									Name:  "t",
									Setup: inlineSetup("TEST SETUP"),
									Statements: []*scaf.Statement{
										scaf.NewStatement("$id", &scaf.Value{Number: ptr(1.0)}),
									},
								},
							},
						},
					},
				},
			},
			expected: `fn Q() ` + "`Q`" + `

Q {
	test "t" {
		setup ` + "`TEST SETUP`" + `

		$id: 1
	}
}
`,
		},
		{
			name: "test with assertion",
			suite: &scaf.Suite{
				Functions: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						FunctionName: "Q",
						Items: []*scaf.TestOrGroup{
							{
								Test: &scaf.Test{
									Name: "t",
									Statements: []*scaf.Statement{
										scaf.NewStatement("$id", &scaf.Value{Number: ptr(1.0)}),
									},
									Asserts: []*scaf.Assert{
										{
											Query: &scaf.AssertQuery{
												Inline: ptr("MATCH (n) RETURN count(n) as c"),
											},
											Conditions: makeConditions(&scaf.Expr{ExprTokens: []*scaf.ExprToken{
												{Ident: ptr("c")},
												{Op: ptr("==")},
												{Number: ptr("1")},
											}}),
										},
									},
								},
							},
						},
					},
				},
			},
			expected: `fn Q() ` + "`Q`" + `

Q {
	test "t" {
		$id: 1

		assert ` + "`MATCH (n) RETURN count(n) as c`" + ` { (c == 1) }
	}
}
`,
		},
		{
			name: "group with tests",
			suite: &scaf.Suite{
				Functions: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						FunctionName: "Q",
						Items: []*scaf.TestOrGroup{
							{
								Group: &scaf.Group{
									Name: "users",
									Items: []*scaf.TestOrGroup{
										{Test: &scaf.Test{Name: "a"}},
										{Test: &scaf.Test{Name: "b"}},
									},
								},
							},
						},
					},
				},
			},
			expected: `fn Q() ` + "`Q`" + `

Q {
	group "users" {
		test "a" {
		}

		test "b" {
		}
	}
}
`,
		},
		{
			name: "group with setup",
			suite: &scaf.Suite{
				Functions: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						FunctionName: "Q",
						Items: []*scaf.TestOrGroup{
							{
								Group: &scaf.Group{
									Name:  "users",
									Setup: inlineSetup("GROUP SETUP"),
									Items: []*scaf.TestOrGroup{
										{Test: &scaf.Test{Name: "a"}},
									},
								},
							},
						},
					},
				},
			},
			expected: `fn Q() ` + "`Q`" + `

Q {
	group "users" {
		setup ` + "`GROUP SETUP`" + `

		test "a" {
		}
	}
}
`,
		},
		{
			name: "nested groups",
			suite: &scaf.Suite{
				Functions: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						FunctionName: "Q",
						Items: []*scaf.TestOrGroup{
							{
								Group: &scaf.Group{
									Name: "level1",
									Items: []*scaf.TestOrGroup{
										{
											Group: &scaf.Group{
												Name:  "level2",
												Items: []*scaf.TestOrGroup{{Test: &scaf.Test{Name: "deep"}}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: `fn Q() ` + "`Q`" + `

Q {
	group "level1" {
		group "level2" {
			test "deep" {
			}
		}
	}
}
`,
		},
		{
			name: "multiple scopes",
			suite: &scaf.Suite{
				Functions: []*scaf.Query{
					{Name: "A", Body: "A"},
					{Name: "B", Body: "B"},
				},
				Scopes: []*scaf.QueryScope{
					{FunctionName: "A", Items: []*scaf.TestOrGroup{{Test: &scaf.Test{Name: "a"}}}},
					{FunctionName: "B", Items: []*scaf.TestOrGroup{{Test: &scaf.Test{Name: "b"}}}},
				},
			},
			expected: `fn A() ` + "`A`" + `

fn B() ` + "`B`" + `

A {
	test "a" {
	}
}

B {
	test "b" {
	}
}
`,
		},
		{
			name: "empty assertion",
			suite: &scaf.Suite{
				Functions: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						FunctionName: "Q",
						Items: []*scaf.TestOrGroup{
							{
								Test: &scaf.Test{
									Name: "t",
									Asserts: []*scaf.Assert{
										{
											Query: &scaf.AssertQuery{
												Inline: ptr("MATCH (n) RETURN n"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expected: `fn Q() ` + "`Q`" + `

Q {
	test "t" {
		assert ` + "`MATCH (n) RETURN n`" + ` {}
	}
}
`,
		},
		{
			name: "shorthand assertion",
			suite: &scaf.Suite{
				Functions: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						FunctionName: "Q",
						Items: []*scaf.TestOrGroup{
							{
								Test: &scaf.Test{
									Name: "t",
									Asserts: []*scaf.Assert{
										{
											Shorthand: makeParenExpr([]*scaf.ExprToken{
												{Ident: ptr("u")},
												{Dot: true},
												{Ident: ptr("age")},
												{Op: ptr(">=")},
												{Number: ptr("18")},
											}),
										},
									},
								},
							},
						},
					},
				},
			},
			expected: `fn Q() ` + "`Q`" + `

Q {
	test "t" {
		assert (u.age >= 18)
	}
}
`,
		},
		{
			name: "scope only with global setup",
			suite: &scaf.Suite{
				Setup: inlineSetup("GLOBAL SETUP"),
				Scopes: []*scaf.QueryScope{
					{
						FunctionName: "Q",
						Items:     []*scaf.TestOrGroup{{Test: &scaf.Test{Name: "t"}}},
					},
				},
			},
			expected: `setup ` + "`GLOBAL SETUP`" + `

Q {
	test "t" {
	}
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := scaf.Format(tt.suite)
			if diff := cmp.Diff(tt.expected, got); diff != "" {
				t.Errorf("Format() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFormatValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    *scaf.Value
		expected string
	}{
		{name: "null", value: &scaf.Value{Null: true}, expected: "null"},
		{name: "string", value: &scaf.Value{Str: ptr("hello")}, expected: `"hello"`},
		{name: "integer", value: &scaf.Value{Number: ptr(42.0)}, expected: "42"},
		{name: "float", value: &scaf.Value{Number: ptr(3.14)}, expected: "3.14"},
		{name: "negative int", value: &scaf.Value{Number: ptr(-5.0)}, expected: "-5"},
		{name: "negative float", value: &scaf.Value{Number: ptr(-2.5)}, expected: "-2.5"},
		{name: "zero", value: &scaf.Value{Number: ptr(0.0)}, expected: "0"},
		{name: "bool true", value: &scaf.Value{Boolean: boolPtr(true)}, expected: "true"},
		{name: "bool false", value: &scaf.Value{Boolean: boolPtr(false)}, expected: "false"},
		{name: "empty list", value: &scaf.Value{List: &scaf.List{}}, expected: "[]"},
		{
			name: "list with values",
			value: &scaf.Value{List: &scaf.List{Values: []*scaf.Value{
				{Number: ptr(1.0)},
				{Str: ptr("two")},
				{Boolean: boolPtr(true)},
			}}},
			expected: `[1, "two", true]`,
		},
		{name: "empty map", value: &scaf.Value{Map: &scaf.Map{}}, expected: "{}"},
		{
			name: "map with values",
			value: &scaf.Value{Map: &scaf.Map{Entries: []*scaf.MapEntry{
				{Key: "a", Value: &scaf.Value{Number: ptr(1.0)}},
				{Key: "b", Value: &scaf.Value{Str: ptr("two")}},
			}}},
			expected: `{a: 1, b: "two"}`,
		},
		{
			name: "nested map in list",
			value: &scaf.Value{List: &scaf.List{Values: []*scaf.Value{
				{Map: &scaf.Map{Entries: []*scaf.MapEntry{
					{Key: "x", Value: &scaf.Value{Number: ptr(1.0)}},
				}}},
			}}},
			expected: `[{x: 1}]`,
		},
		{
			name: "nested list in map",
			value: &scaf.Value{Map: &scaf.Map{Entries: []*scaf.MapEntry{
				{Key: "arr", Value: &scaf.Value{List: &scaf.List{Values: []*scaf.Value{
					{Number: ptr(1.0)},
					{Number: ptr(2.0)},
				}}}},
			}}},
			expected: `{arr: [1, 2]}`,
		},
		{name: "empty value defaults to null", value: &scaf.Value{}, expected: "null"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create a minimal suite with the value
			suite := &scaf.Suite{
				Functions: []*scaf.Query{{Name: "Q", Body: "Q"}},
				Scopes: []*scaf.QueryScope{
					{
						FunctionName: "Q",
						Items: []*scaf.TestOrGroup{
							{
								Test: &scaf.Test{
									Name: "t",
									Statements: []*scaf.Statement{
										scaf.NewStatement("v", tt.value),
									},
								},
							},
						},
					},
				},
			}

			expected := `fn Q() ` + "`Q`" + `

Q {
	test "t" {
		v: ` + tt.expected + `
	}
}
`

			got := scaf.Format(suite)
			if diff := cmp.Diff(expected, got); diff != "" {
				t.Errorf("Format() value mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFormatRoundTrip(t *testing.T) {
	t.Parallel()

	// Test that parsing and then formatting produces parseable output
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "basic query and test",
			input: `fn GetUser() ` + "`MATCH (u:User) RETURN u`" + `

GetUser {
	test "finds user" {
		$id: 1

		u.name: "alice"
	}
}
`,
		},
		{
			name: "with global setup",
			input: `fn Q() ` + "`Q`" + `

setup ` + "`CREATE (:User)`" + `

Q {
	test "t" {
	}
}
`,
		},
		{
			name: "nested groups",
			input: `fn Q() ` + "`Q`" + `

Q {
	group "level1" {
		group "level2" {
			test "deep" {
				$x: 1
			}
		}
	}
}
`,
		},
		{
			name: "complex values",
			input: `fn Q() ` + "`Q`" + `

Q {
	test "complex" {
		list: [1, "two", true, null]
		map: {a: 1, b: "two"}
		nested: {arr: [1, {x: true}]}
	}
}
`,
		},
		{
			name: "assertion",
			input: `fn Q() ` + "`Q`" + `

Q {
	test "t" {
		$id: 1

		assert ` + "`MATCH (n) RETURN count(n) as c`" + ` {
			(c == 1)
		}
	}
}
`,
		},
		{
			name: "shorthand assertion",
			input: `fn Q() ` + "`Q`" + `

Q {
	test "t" {
		assert (u.age >= 18)
	}
}
`,
		},
		{
			name: "multiple shorthand assertions",
			input: `fn Q() ` + "`Q`" + `

Q {
	test "t" {
		assert (x > 0)
		assert (y < 10)
		assert (len(items) == 3)
	}
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Parse
			suite, err := scaf.Parse([]byte(tt.input))
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}

			// Format
			formatted := scaf.Format(suite)

			// Parse again
			suite2, err := scaf.Parse([]byte(formatted))
			if err != nil {
				t.Fatalf("Parse() of formatted output error: %v\nFormatted:\n%s", err, formatted)
			}

			// Format again
			formatted2 := scaf.Format(suite2)

			// The two formatted outputs should be identical (idempotent)
			if diff := cmp.Diff(formatted, formatted2); diff != "" {
				t.Errorf("Format() not idempotent (-first +second):\n%s", diff)
			}
		})
	}
}

func TestFormatPreservesSemantics(t *testing.T) {
	t.Parallel()

	// Test that formatting preserves the AST structure
	suite := &scaf.Suite{
		Functions: []*scaf.Query{
			{Name: "GetUser", Body: "MATCH (u:User {id: $id}) RETURN u"},
		},
		Setup: inlineSetup("CREATE (:User {id: 1, name: \"Alice\"})"),
		Scopes: []*scaf.QueryScope{
			{
				FunctionName: "GetUser",
				Setup:     inlineSetup("MATCH (u:User) SET u.active = true"),
				Items: []*scaf.TestOrGroup{
					{
						Group: &scaf.Group{
							Name:  "active users",
							Setup: inlineSetup("CREATE (:Session)"),
							Items: []*scaf.TestOrGroup{
								{
									Test: &scaf.Test{
										Name:  "finds user",
										Setup: inlineSetup("SET u.verified = true"),
										Statements: []*scaf.Statement{
											scaf.NewStatement("$id", &scaf.Value{Number: ptr(1.0)}),
											scaf.NewStatement("u.name", &scaf.Value{Str: ptr("Alice")}),
											scaf.NewStatement("u.active", &scaf.Value{Boolean: boolPtr(true)}),
										},
										Asserts: []*scaf.Assert{
											{
												Query: &scaf.AssertQuery{
													Inline: ptr("MATCH (s:Session) RETURN count(s) as c"),
												},
												Conditions: makeConditions(&scaf.Expr{ExprTokens: []*scaf.ExprToken{
													{Ident: ptr("c")},
													{Op: ptr("==")},
													{Number: ptr("1")},
												}}),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	formatted := scaf.Format(suite)

	// Parse the formatted output
	parsed, err := scaf.Parse([]byte(formatted))
	if err != nil {
		t.Fatalf("Parse() error: %v\nFormatted:\n%s", err, formatted)
	}

	// Compare ASTs (ignoring position info since it won't match)
	if diff := cmp.Diff(suite, parsed, cmpIgnoreAST); diff != "" {
		t.Errorf("AST mismatch after format+parse (-original +parsed):\n%s", diff)
	}
}

func TestFormatOnlyOutputs(t *testing.T) {
	t.Parallel()

	// Test that outputs without inputs are formatted correctly (no blank line)
	suite := &scaf.Suite{
		Functions: []*scaf.Query{{Name: "Q", Body: "Q"}},
		Scopes: []*scaf.QueryScope{
			{
				FunctionName: "Q",
				Items: []*scaf.TestOrGroup{
					{
						Test: &scaf.Test{
							Name: "outputs only",
							Statements: []*scaf.Statement{
								scaf.NewStatement("name", &scaf.Value{Str: ptr("Alice")}),
								scaf.NewStatement("age", &scaf.Value{Number: ptr(30.0)}),
							},
						},
					},
				},
			},
		},
	}

	expected := `fn Q() ` + "`Q`" + `

Q {
	test "outputs only" {
		name: "Alice"
		age: 30
	}
}
`

	got := scaf.Format(suite)
	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Format() mismatch (-want +got):\n%s", diff)
	}
}

func TestFormatOnlyInputs(t *testing.T) {
	t.Parallel()

	// Test that inputs without outputs are formatted correctly (no trailing blank line)
	suite := &scaf.Suite{
		Functions: []*scaf.Query{{Name: "Q", Body: "Q"}},
		Scopes: []*scaf.QueryScope{
			{
				FunctionName: "Q",
				Items: []*scaf.TestOrGroup{
					{
						Test: &scaf.Test{
							Name: "inputs only",
							Statements: []*scaf.Statement{
								scaf.NewStatement("$id", &scaf.Value{Number: ptr(1.0)}),
								scaf.NewStatement("$name", &scaf.Value{Str: ptr("alice")}),
							},
						},
					},
				},
			},
		},
	}

	expected := `fn Q() ` + "`Q`" + `

Q {
	test "inputs only" {
		$id: 1
		$name: "alice"
	}
}
`

	got := scaf.Format(suite)
	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Format() mismatch (-want +got):\n%s", diff)
	}
}

func TestFormatMultipleTestsInGroup(t *testing.T) {
	t.Parallel()

	suite := &scaf.Suite{
		Functions: []*scaf.Query{{Name: "Q", Body: "Q"}},
		Scopes: []*scaf.QueryScope{
			{
				FunctionName: "Q",
				Items: []*scaf.TestOrGroup{
					{
						Group: &scaf.Group{
							Name: "group",
							Items: []*scaf.TestOrGroup{
								{
									Test: &scaf.Test{
										Name: "first",
										Statements: []*scaf.Statement{
											scaf.NewStatement("$x", &scaf.Value{Number: ptr(1.0)}),
										},
									},
								},
								{
									Test: &scaf.Test{
										Name: "second",
										Statements: []*scaf.Statement{
											scaf.NewStatement("$y", &scaf.Value{Number: ptr(2.0)}),
										},
									},
								},
								{
									Test: &scaf.Test{
										Name: "third",
										Statements: []*scaf.Statement{
											scaf.NewStatement("$z", &scaf.Value{Number: ptr(3.0)}),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	expected := `fn Q() ` + "`Q`" + `

Q {
	group "group" {
		test "first" {
			$x: 1
		}

		test "second" {
			$y: 2
		}

		test "third" {
			$z: 3
		}
	}
}
`

	got := scaf.Format(suite)
	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Format() mismatch (-want +got):\n%s", diff)
	}
}

func TestFormatMixedGroupsAndTests(t *testing.T) {
	t.Parallel()

	suite := &scaf.Suite{
		Functions: []*scaf.Query{{Name: "Q", Body: "Q"}},
		Scopes: []*scaf.QueryScope{
			{
				FunctionName: "Q",
				Items: []*scaf.TestOrGroup{
					{Test: &scaf.Test{Name: "standalone"}},
					{
						Group: &scaf.Group{
							Name:  "group",
							Items: []*scaf.TestOrGroup{{Test: &scaf.Test{Name: "in group"}}},
						},
					},
					{Test: &scaf.Test{Name: "another standalone"}},
				},
			},
		},
	}

	expected := `fn Q() ` + "`Q`" + `

Q {
	test "standalone" {
	}

	group "group" {
		test "in group" {
		}
	}

	test "another standalone" {
	}
}
`

	got := scaf.Format(suite)
	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Format() mismatch (-want +got):\n%s", diff)
	}
}

func TestFormatEmptySuite(t *testing.T) {
	t.Parallel()

	suite := &scaf.Suite{}
	got := scaf.Format(suite)

	if got != "\n" {
		t.Errorf("Format() empty suite = %q, want %q", got, "\n")
	}
}

func TestFormatQueryOnly(t *testing.T) {
	t.Parallel()

	suite := &scaf.Suite{
		Functions: []*scaf.Query{
			{Name: "GetUser", Body: "MATCH (u:User) RETURN u"},
		},
	}

	expected := "fn GetUser() `MATCH (u:User) RETURN u`\n"
	got := scaf.Format(suite)

	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Format() mismatch (-want +got):\n%s", diff)
	}
}

func TestFormatSetupOnly(t *testing.T) {
	t.Parallel()

	suite := &scaf.Suite{
		Setup: inlineSetup("CREATE (:Node)"),
	}

	expected := "setup `CREATE (:Node)`\n"
	got := scaf.Format(suite)

	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Format() mismatch (-want +got):\n%s", diff)
	}
}

func TestFormatLargeNumbers(t *testing.T) {
	t.Parallel()

	suite := &scaf.Suite{
		Functions: []*scaf.Query{{Name: "Q", Body: "Q"}},
		Scopes: []*scaf.QueryScope{
			{
				FunctionName: "Q",
				Items: []*scaf.TestOrGroup{
					{
						Test: &scaf.Test{
							Name: "t",
							Statements: []*scaf.Statement{
								scaf.NewStatement("big", &scaf.Value{Number: ptr(1000000.0)}),
								scaf.NewStatement("precise", &scaf.Value{Number: ptr(123.456789)}),
							},
						},
					},
				},
			},
		},
	}

	expected := `fn Q() ` + "`Q`" + `

Q {
	test "t" {
		big: 1000000
		precise: 123.456789
	}
}
`

	got := scaf.Format(suite)
	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Format() mismatch (-want +got):\n%s", diff)
	}
}

func TestFormatWithComments(t *testing.T) {
	// Not parallel - trivia state requires serialized access
	input := "// File-level comment\nfn GetUser() `MATCH (u:User) RETURN u`\n\n// Scope comment\nGetUser {\n\t// Group comment\n\tgroup \"tests\" {\n\t\t// Test comment\n\t\ttest \"finds user\" {\n\t\t\t$id: 1\n\t\t}\n\t}\n}\n"

	result, err := scaf.Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	got := scaf.Format(result)

	// The formatter should preserve comments
	if !strings.Contains(got, "// File-level comment") {
		t.Errorf("Missing file-level comment in output:\n%s", got)
	}

	if !strings.Contains(got, "// Scope comment") {
		t.Errorf("Missing scope comment in output:\n%s", got)
	}

	if !strings.Contains(got, "// Group comment") {
		t.Errorf("Missing group comment in output:\n%s", got)
	}

	if !strings.Contains(got, "// Test comment") {
		t.Errorf("Missing test comment in output:\n%s", got)
	}
}

func TestFormatWithTrailingComments(t *testing.T) {
	// Not parallel - trivia state requires serialized access
	input := "fn GetUser() `MATCH (u:User) RETURN u` // query comment\n\nGetUser {\n\ttest \"finds user\" {\n\t\t$id: 1\n\t}\n}\n"

	result, err := scaf.Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	got := scaf.Format(result)

	// The formatter should preserve trailing comments
	if !strings.Contains(got, "// query comment") {
		t.Errorf("Missing trailing comment in output:\n%s", got)
	}
}

func TestFormatWithParameterComments(t *testing.T) {
	// Not parallel - trivia state requires serialized access
	input := `fn CreateUser(
	// The user's unique identifier
	id: string,
	// The user's display name
	name: string,
) ` + "`CREATE (u:User {id: $id, name: $name}) RETURN u`" + `
`
	result, err := scaf.Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	got := scaf.Format(result)

	// The formatter should preserve parameter comments
	if !strings.Contains(got, "// The user's unique identifier") {
		t.Errorf("Missing first param comment in output:\n%s", got)
	}
	if !strings.Contains(got, "// The user's display name") {
		t.Errorf("Missing second param comment in output:\n%s", got)
	}
}

func TestFormatWithStatementComments(t *testing.T) {
	// Not parallel - trivia state requires serialized access
	input := `fn Q() ` + "`Q`" + `

Q {
	test "example" {
		// Comment for input
		$id: 1
		// Comment for output
		u.name: "Alice"
	}
}
`
	result, err := scaf.Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	got := scaf.Format(result)

	// The formatter should preserve statement comments
	if !strings.Contains(got, "// Comment for input") {
		t.Errorf("Missing input comment in output:\n%s", got)
	}
	if !strings.Contains(got, "// Comment for output") {
		t.Errorf("Missing output comment in output:\n%s", got)
	}
}

func TestFormatWithAssertComments(t *testing.T) {
	// Not parallel - trivia state requires serialized access
	input := `fn Q() ` + "`Q`" + `

Q {
	test "example" {
		// Verify the result
		assert (u != null)
	}
}
`
	result, err := scaf.Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	got := scaf.Format(result)

	// The formatter should preserve assert comments
	if !strings.Contains(got, "// Verify the result") {
		t.Errorf("Missing assert comment in output:\n%s", got)
	}
}

func TestFormatUntypedParams(t *testing.T) {
	t.Parallel()

	suite := &scaf.Suite{
		Functions: []*scaf.Query{
			{
				Name: "CreatePost",
				Params: []*scaf.FnParam{
					{Name: "title"},
					{Name: "authorId"},
				},
				Body: "CREATE (p:Post {title: $title, authorId: $authorId}) RETURN p",
			},
		},
	}

	expected := "fn CreatePost(title, authorId) `CREATE (p:Post {title: $title, authorId: $authorId}) RETURN p`\n"
	got := scaf.Format(suite)

	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Format() mismatch (-want +got):\n%s", diff)
	}
}

func TestFormatMixedTypedAndUntypedParams(t *testing.T) {
	t.Parallel()

	suite := &scaf.Suite{
		Functions: []*scaf.Query{
			{
				Name: "CreateUser",
				Params: []*scaf.FnParam{
					{Name: "id"},
					{Name: "name", Type: &scaf.TypeExpr{Simple: ptr("string")}},
					{Name: "data"},
				},
				Body: "CREATE (u:User) RETURN u",
			},
		},
	}

	expected := "fn CreateUser(id, name: string, data) `CREATE (u:User) RETURN u`\n"
	got := scaf.Format(suite)

	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Format() mismatch (-want +got):\n%s", diff)
	}
}

func TestFormatLongParamsStaySingleLineWithoutTrailingComma(t *testing.T) {
	t.Parallel()

	// Without trailing comma, long parameter lists stay on single line
	suite := &scaf.Suite{
		Functions: []*scaf.Query{
			{
				Name: "CreateUserWithManyParams",
				Params: []*scaf.FnParam{
					{Name: "firstName", Type: &scaf.TypeExpr{Simple: ptr("string")}},
					{Name: "lastName", Type: &scaf.TypeExpr{Simple: ptr("string")}},
					{Name: "emailAddress", Type: &scaf.TypeExpr{Simple: ptr("string")}},
					{Name: "phoneNumber", Type: &scaf.TypeExpr{Simple: ptr("string")}},
					{Name: "dateOfBirth", Type: &scaf.TypeExpr{Simple: ptr("string")}},
				},
				TrailingComma: false, // No trailing comma = single line
				Body:          "CREATE (u:User) RETURN u",
			},
		},
	}

	got := scaf.Format(suite)

	// Should be single line (no auto-splitting without trailing comma)
	expected := "fn CreateUserWithManyParams(firstName: string, lastName: string, emailAddress: string, phoneNumber: string, dateOfBirth: string) `CREATE (u:User) RETURN u`\n"
	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Format() mismatch (-want +got):\n%s", diff)
	}
}

func TestFormatSmartSplitPreservesShort(t *testing.T) {
	t.Parallel()

	// Short params should stay on one line
	suite := &scaf.Suite{
		Functions: []*scaf.Query{
			{
				Name: "GetUser",
				Params: []*scaf.FnParam{
					{Name: "id", Type: &scaf.TypeExpr{Simple: ptr("string")}},
				},
				Body: "Q",
			},
		},
	}

	expected := "fn GetUser(id: string) `Q`\n"
	got := scaf.Format(suite)

	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Format() mismatch (-want +got):\n%s", diff)
	}
}

func TestFormatLongListStaySingleLineWithoutTrailingComma(t *testing.T) {
	t.Parallel()

	// Create a long list - without trailing comma should stay on single line
	values := make([]*scaf.Value, 20)
	for i := range values {
		values[i] = &scaf.Value{Str: ptr("item" + strings.Repeat("x", 10))}
	}

	suite := &scaf.Suite{
		Functions: []*scaf.Query{{Name: "Q", Body: "Q"}},
		Scopes: []*scaf.QueryScope{
			{
				FunctionName: "Q",
				Items: []*scaf.TestOrGroup{
					{
						Test: &scaf.Test{
							Name: "test",
							Statements: []*scaf.Statement{
								scaf.NewStatement("longList", &scaf.Value{List: &scaf.List{
									Values:        values,
									TrailingComma: false, // No trailing comma = single line
								}}),
							},
						},
					},
				},
			},
		},
	}

	got := scaf.Format(suite)

	// Should be single line (no auto-splitting without trailing comma)
	if strings.Contains(got, "longList: [\n") {
		t.Errorf("Expected single-line list (no trailing comma), got:\n%s", got)
	}

	// Verify it contains the list inline
	if !strings.Contains(got, "longList: [") || !strings.Contains(got, "]") {
		t.Errorf("Expected list to be present, got:\n%s", got)
	}
}

func TestFormatSmartSplitShortListStaysInline(t *testing.T) {
	t.Parallel()

	suite := &scaf.Suite{
		Functions: []*scaf.Query{{Name: "Q", Body: "Q"}},
		Scopes: []*scaf.QueryScope{
			{
				FunctionName: "Q",
				Items: []*scaf.TestOrGroup{
					{
						Test: &scaf.Test{
							Name: "test",
							Statements: []*scaf.Statement{
								scaf.NewStatement("ids", &scaf.Value{List: &scaf.List{Values: []*scaf.Value{
									{Number: ptr(1.0)},
									{Number: ptr(2.0)},
									{Number: ptr(3.0)},
								}}}),
							},
						},
					},
				},
			},
		},
	}

	got := scaf.Format(suite)

	// Short list should stay on one line
	if !strings.Contains(got, "ids: [1, 2, 3]") {
		t.Errorf("Expected short list to stay inline, got:\n%s", got)
	}
}

func TestFormatRoundTripUntypedParams(t *testing.T) {
	t.Parallel()

	input := `fn CreatePost(title, authorId) ` + "`CREATE (p:Post) RETURN p`" + `

CreatePost {
	test "creates post" {
		$title: "Hello"
		$authorId: "user-1"
	}
}
`

	// Parse
	suite, err := scaf.Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	// Format
	formatted := scaf.Format(suite)

	// Parse again
	suite2, err := scaf.Parse([]byte(formatted))
	if err != nil {
		t.Fatalf("Parse() of formatted output error: %v\nFormatted:\n%s", err, formatted)
	}

	// Format again
	formatted2 := scaf.Format(suite2)

	// Should be idempotent
	if diff := cmp.Diff(formatted, formatted2); diff != "" {
		t.Errorf("Format() not idempotent (-first +second):\n%s", diff)
	}

	// Should contain untyped params without ':'
	if strings.Contains(formatted, "title:") && !strings.Contains(formatted, "$title:") {
		t.Errorf("Expected untyped params without ':', got:\n%s", formatted)
	}
}

func TestFormatStringEscaping(t *testing.T) {
	t.Parallel()

	// Test that strings with special characters are properly escaped
	suite := &scaf.Suite{
		Functions: []*scaf.Query{{Name: "Q", Body: "Q"}},
		Scopes: []*scaf.QueryScope{
			{
				FunctionName: "Q",
				Items: []*scaf.TestOrGroup{
					{
						Test: &scaf.Test{
							Name: "special chars",
							Statements: []*scaf.Statement{
								scaf.NewStatement("$multiline", &scaf.Value{Str: ptr("Line 1\nLine 2\nLine 3")}),
								scaf.NewStatement("$withQuotes", &scaf.Value{Str: ptr(`He said "hello"`)}),
								scaf.NewStatement("$withTab", &scaf.Value{Str: ptr("col1\tcol2")}),
								scaf.NewStatement("$withBackslash", &scaf.Value{Str: ptr(`path\to\file`)}),
								scaf.NewStatement("$withEmoji", &scaf.Value{Str: ptr("Hello 🎉 World")}),
							},
						},
					},
				},
			},
		},
	}

	formatted := scaf.Format(suite)

	// Should be parseable
	parsed, err := scaf.Parse([]byte(formatted))
	if err != nil {
		t.Fatalf("Parse() of formatted output error: %v\nFormatted:\n%s", err, formatted)
	}

	// Check roundtrip preserves values
	test := parsed.Scopes[0].Items[0].Test
	for _, stmt := range test.Statements {
		key := stmt.Key()
		var expected string
		switch key {
		case "$multiline":
			expected = "Line 1\nLine 2\nLine 3"
		case "$withQuotes":
			expected = `He said "hello"`
		case "$withTab":
			expected = "col1\tcol2"
		case "$withBackslash":
			expected = `path\to\file`
		case "$withEmoji":
			expected = "Hello 🎉 World"
		}

		if stmt.Value == nil || stmt.Value.Literal == nil || stmt.Value.Literal.Str == nil {
			t.Errorf("Statement %s: expected string value, got nil", key)

			continue
		}

		if *stmt.Value.Literal.Str != expected {
			t.Errorf("Statement %s: got %q, want %q", key, *stmt.Value.Literal.Str, expected)
		}
	}
}

func TestFormatStringEscapingIdempotent(t *testing.T) {
	t.Parallel()

	// Test that formatting is idempotent for strings with special characters
	input := `fn Q() ` + "`Q`" + `

Q {
	test "special" {
		$bio: "Line 1\nLine 2\nEmoji: 🎉"
	}
}
`

	// Parse
	suite, err := scaf.Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	// Format
	formatted := scaf.Format(suite)

	// Parse again
	suite2, err := scaf.Parse([]byte(formatted))
	if err != nil {
		t.Fatalf("Parse() of formatted output error: %v\nFormatted:\n%s", err, formatted)
	}

	// Format again
	formatted2 := scaf.Format(suite2)

	// Should be idempotent
	if diff := cmp.Diff(formatted, formatted2); diff != "" {
		t.Errorf("Format() not idempotent (-first +second):\n%s", diff)
	}
}

// =============================================================================
// Trailing comma tests (Dart-style formatting)
// =============================================================================

func TestFormatTrailingCommaFunctionParams(t *testing.T) {
	// Trailing comma forces multi-line formatting
	input := `fn CreateUser(
	id: string,
	name: string,
) ` + "`CREATE (u:User) RETURN u`" + `
`

	// Parse
	suite, err := scaf.Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	// Format should preserve multi-line because of trailing comma
	formatted := scaf.Format(suite)

	// Should contain trailing comma and be multi-line
	expected := `fn CreateUser(
	id: string,
	name: string,
) ` + "`CREATE (u:User) RETURN u`" + `
`

	if diff := cmp.Diff(expected, formatted); diff != "" {
		t.Errorf("Format() mismatch (-want +got):\n%s", diff)
	}

	// Verify idempotence
	suite2, err := scaf.Parse([]byte(formatted))
	if err != nil {
		t.Fatalf("Parse() of formatted output error: %v", err)
	}
	formatted2 := scaf.Format(suite2)

	if diff := cmp.Diff(formatted, formatted2); diff != "" {
		t.Errorf("Format() not idempotent (-first +second):\n%s", diff)
	}
}

func TestFormatNoTrailingCommaFunctionParams(t *testing.T) {
	t.Parallel()

	// Without trailing comma, short params stay on one line
	suite := &scaf.Suite{
		Functions: []*scaf.Query{
			{
				Name: "CreateUser",
				Params: []*scaf.FnParam{
					{Name: "id", Type: &scaf.TypeExpr{Simple: ptr("string")}},
					{Name: "name", Type: &scaf.TypeExpr{Simple: ptr("string")}},
				},
				TrailingComma: false,
				Body:          "CREATE (u:User) RETURN u",
			},
		},
	}

	expected := "fn CreateUser(id: string, name: string) `CREATE (u:User) RETURN u`\n"
	got := scaf.Format(suite)

	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Format() mismatch (-want +got):\n%s", diff)
	}
}

func TestFormatTrailingCommaList(t *testing.T) {
	// Trailing comma in list forces multi-line formatting
	input := `fn Q() ` + "`Q`" + `

Q {
	test "test" {
		$ids: [
			1,
			2,
			3,
		]
	}
}
`

	// Parse
	suite, err := scaf.Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	// Format should preserve multi-line because of trailing comma
	formatted := scaf.Format(suite)

	// Should be multi-line with trailing comma
	if !strings.Contains(formatted, "[\n") {
		t.Errorf("Expected multi-line list formatting, got:\n%s", formatted)
	}

	// Each element should have trailing comma
	if !strings.Contains(formatted, "1,\n") || !strings.Contains(formatted, "3,\n") {
		t.Errorf("Expected trailing commas on all elements, got:\n%s", formatted)
	}

	// Verify idempotence
	suite2, err := scaf.Parse([]byte(formatted))
	if err != nil {
		t.Fatalf("Parse() of formatted output error: %v", err)
	}
	formatted2 := scaf.Format(suite2)

	if diff := cmp.Diff(formatted, formatted2); diff != "" {
		t.Errorf("Format() not idempotent (-first +second):\n%s", diff)
	}
}

func TestFormatTrailingCommaMap(t *testing.T) {
	// Trailing comma in map forces multi-line formatting
	input := `fn Q() ` + "`Q`" + `

Q {
	test "test" {
		$data: {
			a: 1,
			b: 2,
		}
	}
}
`

	// Parse
	suite, err := scaf.Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	// Format should preserve multi-line because of trailing comma
	formatted := scaf.Format(suite)

	// Should be multi-line with trailing comma
	if !strings.Contains(formatted, "{\n") {
		t.Errorf("Expected multi-line map formatting, got:\n%s", formatted)
	}

	// Each entry should have trailing comma
	if !strings.Contains(formatted, "a: 1,\n") || !strings.Contains(formatted, "b: 2,\n") {
		t.Errorf("Expected trailing commas on all entries, got:\n%s", formatted)
	}

	// Verify idempotence
	suite2, err := scaf.Parse([]byte(formatted))
	if err != nil {
		t.Fatalf("Parse() of formatted output error: %v", err)
	}
	formatted2 := scaf.Format(suite2)

	if diff := cmp.Diff(formatted, formatted2); diff != "" {
		t.Errorf("Format() not idempotent (-first +second):\n%s", diff)
	}
}

func TestFormatNoTrailingCommaListInline(t *testing.T) {
	t.Parallel()

	// Without trailing comma, short lists stay on one line
	suite := &scaf.Suite{
		Functions: []*scaf.Query{{Name: "Q", Body: "Q"}},
		Scopes: []*scaf.QueryScope{
			{
				FunctionName: "Q",
				Items: []*scaf.TestOrGroup{
					{
						Test: &scaf.Test{
							Name: "test",
							Statements: []*scaf.Statement{
								scaf.NewStatement("$ids", &scaf.Value{List: &scaf.List{
									Values:        []*scaf.Value{{Number: ptr(1.0)}, {Number: ptr(2.0)}, {Number: ptr(3.0)}},
									TrailingComma: false,
								}}),
							},
						},
					},
				},
			},
		},
	}

	got := scaf.Format(suite)

	// Short list without trailing comma should be inline
	if !strings.Contains(got, "$ids: [1, 2, 3]") {
		t.Errorf("Expected inline list, got:\n%s", got)
	}
}

func TestFormatTrailingCommaSetupCall(t *testing.T) {
	// Trailing comma in setup call forces multi-line formatting
	input := `fn Q() ` + "`Q`" + `

setup fixtures.CreateUser(
	id: "user-1",
	name: "Alice",
)

Q {
	test "test" {
	}
}
`

	// Parse
	suite, err := scaf.Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	// Format should preserve multi-line because of trailing comma
	formatted := scaf.Format(suite)

	// Should be multi-line
	if !strings.Contains(formatted, "setup fixtures.CreateUser(\n") {
		t.Errorf("Expected multi-line setup call, got:\n%s", formatted)
	}

	// Each param should have trailing comma
	if !strings.Contains(formatted, `id: "user-1",`) || !strings.Contains(formatted, `name: "Alice",`) {
		t.Errorf("Expected trailing commas on params, got:\n%s", formatted)
	}

	// Verify idempotence
	suite2, err := scaf.Parse([]byte(formatted))
	if err != nil {
		t.Fatalf("Parse() of formatted output error: %v", err)
	}
	formatted2 := scaf.Format(suite2)

	if diff := cmp.Diff(formatted, formatted2); diff != "" {
		t.Errorf("Format() not idempotent (-first +second):\n%s", diff)
	}
}

func TestFormatTrailingCommaNestedStructures(t *testing.T) {
	// Nested structures with trailing commas
	input := `fn Q() ` + "`Q`" + `

Q {
	test "test" {
		$data: {
			users: [
				{
					id: 1,
					name: "Alice",
				},
			],
		}
	}
}
`

	// Parse
	suite, err := scaf.Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	// Format should preserve multi-line structure
	formatted := scaf.Format(suite)

	// Verify idempotence - the key property of Dart-style trailing commas
	suite2, err := scaf.Parse([]byte(formatted))
	if err != nil {
		t.Fatalf("Parse() of formatted output error: %v", err)
	}
	formatted2 := scaf.Format(suite2)

	if diff := cmp.Diff(formatted, formatted2); diff != "" {
		t.Errorf("Format() not idempotent (-first +second):\n%s", diff)
	}
}
