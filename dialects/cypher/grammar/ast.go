package cyphergrammar

import "github.com/alecthomas/participle/v2/lexer"

// ----------------------------------------------------------------------------
// Cypher AST
//
// This file defines the Abstract Syntax Tree for Cypher queries.
// The grammar follows the official Cypher specification:
// https://github.com/opencypher/openCypher
// ----------------------------------------------------------------------------

// Script is the root of a Cypher parse tree.
type Script struct {
	Pos   lexer.Position
	Query *Query `@@`
	Semi  string `@Semicolon?`
}

// Query represents a top-level query.
type Query struct {
	Pos            lexer.Position
	StandaloneCall *StandaloneCall `  @@`
	RegularQuery   *RegularQuery   `| @@`
}

// RegularQuery is a query with optional UNION clauses.
type RegularQuery struct {
	Pos         lexer.Position
	SingleQuery *SingleQuery   `@@`
	Unions      []*UnionClause `@@*`
}

// UnionClause represents a UNION clause.
type UnionClause struct {
	Pos   lexer.Position
	All   bool         `"UNION" @"ALL"?`
	Query *SingleQuery `@@`
}

// SingleQuery is a query consisting of clauses.
// We don't distinguish multi-part from single-part at the AST level
// since the presence of WITH clauses is what makes it multi-part.
type SingleQuery struct {
	Pos     lexer.Position
	Clauses []*Clause `@@+`
}

// Clause is any clause in a query (reading, updating, WITH, or RETURN).
type Clause struct {
	Pos      lexer.Position
	Reading  *ReadingClause  `  @@`
	Updating *UpdatingClause `| @@`
	With     *WithClause     `| @@`
	Return   *ReturnClause   `| @@`
}

// ----------------------------------------------------------------------------
// Clauses
// ----------------------------------------------------------------------------

// ReadingClause represents MATCH, UNWIND, or CALL.
type ReadingClause struct {
	Pos    lexer.Position
	Match  *MatchClause  `  @@`
	Unwind *UnwindClause `| @@`
	Call   *CallClause   `| @@`
}

// UpdatingClause represents CREATE, MERGE, DELETE, SET, or REMOVE.
type UpdatingClause struct {
	Pos    lexer.Position
	Create *CreateClause `  @@`
	Merge  *MergeClause  `| @@`
	Delete *DeleteClause `| @@`
	Set    *SetClause    `| @@`
	Remove *RemoveClause `| @@`
}

// MatchClause represents an OPTIONAL? MATCH pattern WHERE clause.
type MatchClause struct {
	Pos      lexer.Position
	Optional bool     `@"OPTIONAL"?`
	Pattern  *Pattern `"MATCH" @@`
	Where    *Where   `@@?`
}

// UnwindClause represents UNWIND expr AS symbol.
type UnwindClause struct {
	Pos    lexer.Position
	Expr   *Expression `"UNWIND" @@`
	Symbol string      `"AS" @Ident`
}

// CallClause represents CALL in a query (not standalone).
type CallClause struct {
	Pos       lexer.Position
	Procedure *InvocationName `"CALL" @@`
	Args      *ParenExprList  `@@?`
	Yield     *YieldClause    `( "YIELD" @@ )?`
}

// StandaloneCall represents a standalone CALL.
type StandaloneCall struct {
	Pos       lexer.Position
	Procedure *InvocationName  `"CALL" @@`
	Args      *ParenExprList   `@@?`
	Yield     *StandaloneYield `( "YIELD" @@ )?`
}

// StandaloneYield is YIELD for standalone CALL (can be * or items).
type StandaloneYield struct {
	Pos   lexer.Position
	Star  bool         `  @Star`
	Items *YieldClause `| @@`
}

// YieldClause represents YIELD items with optional WHERE.
type YieldClause struct {
	Pos   lexer.Position
	Items []*YieldItem `@@ ( Comma @@ )*`
	Where *Where       `@@?`
}

// YieldItem is a single yield item with optional alias.
type YieldItem struct {
	Pos    lexer.Position
	Source string `( @Ident "AS" )?`
	Target string `@Ident`
}

// ReturnClause represents RETURN projection.
type ReturnClause struct {
	Pos  lexer.Position
	Body *ProjectionBody `"RETURN" @@`
}

// WithClause represents WITH projection.
type WithClause struct {
	Pos   lexer.Position
	Body  *ProjectionBody `"WITH" @@`
	Where *Where          `@@?`
}

// ProjectionBody is the common body for RETURN and WITH.
type ProjectionBody struct {
	Pos      lexer.Position
	Distinct bool             `@"DISTINCT"?`
	Items    *ProjectionItems `@@`
	Order    *OrderBy         `@@?`
	Skip     *Skip            `@@?`
	Limit    *Limit           `@@?`
}

// ProjectionItems is * or a list of projection items.
type ProjectionItems struct {
	Pos   lexer.Position
	Star  bool              `  @Star`
	Items []*ProjectionItem `| @@ ( Comma @@ )*`
}

// ProjectionItem is an expression with optional alias.
type ProjectionItem struct {
	Pos   lexer.Position
	Expr  *Expression `@@`
	Alias string      `( "AS" @Ident )?`
}

// OrderBy represents ORDER BY clause.
type OrderBy struct {
	Pos   lexer.Position
	Items []*OrderItem `"ORDER" "BY" @@ ( Comma @@ )*`
}

// OrderItem is an expression with optional direction.
type OrderItem struct {
	Pos  lexer.Position
	Expr *Expression `@@`
	Desc bool        `( @( "DESC" | "DESCENDING" ) | "ASC" | "ASCENDING" )?`
}

// Skip represents SKIP clause.
type Skip struct {
	Pos  lexer.Position
	Expr *Expression `"SKIP" @@`
}

// Limit represents LIMIT clause.
type Limit struct {
	Pos  lexer.Position
	Expr *Expression `"LIMIT" @@`
}

// Where represents WHERE clause.
type Where struct {
	Pos  lexer.Position
	Expr *Expression `"WHERE" @@`
}

// CreateClause represents CREATE pattern.
type CreateClause struct {
	Pos     lexer.Position
	Pattern *Pattern `"CREATE" @@`
}

// MergeClause represents MERGE with optional ON MATCH/CREATE actions.
type MergeClause struct {
	Pos     lexer.Position
	Pattern *PatternPart   `"MERGE" @@`
	Actions []*MergeAction `@@*`
}

// MergeAction is ON MATCH or ON CREATE SET clause.
type MergeAction struct {
	Pos      lexer.Position
	OnMatch  bool       `"ON" ( @"MATCH"`
	OnCreate bool       `     | @"CREATE" )`
	Set      *SetClause `@@`
}

// DeleteClause represents DETACH? DELETE expression list.
type DeleteClause struct {
	Pos    lexer.Position
	Detach bool          `@"DETACH"?`
	Exprs  []*Expression `"DELETE" @@ ( Comma @@ )*`
}

// SetClause represents SET items.
type SetClause struct {
	Pos   lexer.Position
	Items []*SetItem `"SET" @@ ( Comma @@ )*`
}

// SetItem is a single SET operation.
// Cypher supports three forms:
// 1. property.path = expr
// 2. variable = expr (whole assignment) or variable += expr (merge)
// 3. variable:Label:Label... (label assignment)
type SetItem struct {
	Pos lexer.Position
	// Property assignment: prop.path = expr
	Property     *PropertyExpr `( @@ Eq`
	PropertyExpr *Expression   `  @@ )`
	// OR variable assignment/merge: var = expr or var += expr
	Variable  string      `| ( @Ident`
	AddAssign bool        `  ( @AddAssign`
	Assign    bool        `  | @Eq )`
	VarExpr   *Expression `  @@ )`
	// OR label assignment: var:Label
	LabelVar string      `| @Ident`
	Labels   *NodeLabels `  @@`
}

// RemoveClause represents REMOVE items.
type RemoveClause struct {
	Pos   lexer.Position
	Items []*RemoveItem `"REMOVE" @@ ( Comma @@ )*`
}

// RemoveItem is a label removal or property removal.
type RemoveItem struct {
	Pos      lexer.Position
	Variable string        `  @Ident`
	Labels   *NodeLabels   `( @@`
	Property *PropertyExpr `| @@ )`
}

// ----------------------------------------------------------------------------
// Patterns
// ----------------------------------------------------------------------------

// Pattern is a comma-separated list of pattern parts.
type Pattern struct {
	Pos   lexer.Position
	Parts []*PatternPart `@@ ( Comma @@ )*`
}

// PatternPart is an optional variable assignment to a pattern element.
type PatternPart struct {
	Pos     lexer.Position
	Var     string          `( @Ident Eq )?`
	Element *PatternElement `@@`
}

// PatternElement is a node pattern followed by relationship chains.
type PatternElement struct {
	Pos lexer.Position
	// Handle parenthesized pattern element
	Paren *PatternElement     `  LParen @@ RParen`
	Node  *NodePattern        `| @@`
	Chain []*PatternElemChain `@@*`
}

// PatternElemChain is a relationship pattern followed by a node pattern.
type PatternElemChain struct {
	Pos  lexer.Position
	Rel  *RelationshipPattern `@@`
	Node *NodePattern         `@@`
}

// NodePattern is (variable? labels? properties?).
type NodePattern struct {
	Pos        lexer.Position
	Variable   string      `LParen @Ident?`
	Labels     *NodeLabels `@@?`
	Properties *Properties `@@? RParen`
}

// NodeLabels is a sequence of :Label.
type NodeLabels struct {
	Pos    lexer.Position
	Labels []string `( Colon @Ident )+`
}

// Properties is a map literal or parameter.
type Properties struct {
	Pos   lexer.Position
	Map   *MapLiteral `  @@`
	Param *Parameter  `| @@`
}

// RelationshipPattern is -[...]-> or <-[...]- or -[...]-.
type RelationshipPattern struct {
	Pos        lexer.Position
	LeftArrow  bool                `@Less? Minus`
	Detail     *RelationshipDetail `( LBracket @@ RBracket )?`
	RightArrow bool                `Minus @Greater?`
}

// RelationshipDetail is the content inside relationship brackets.
type RelationshipDetail struct {
	Pos        lexer.Position
	Variable   string             `@Ident?`
	Types      *RelationshipTypes `@@?`
	Range      *RangeLiteral      `@@?`
	Properties *Properties        `@@?`
}

// RelationshipTypes is :TYPE|TYPE|...
type RelationshipTypes struct {
	Pos   lexer.Position
	Types []string `Colon @Ident ( Pipe Colon? @Ident )*`
}

// RangeLiteral is *min..max for variable-length relationships.
type RangeLiteral struct {
	Pos   lexer.Position
	Star  string `@Star`
	Min   *int   `@Int?`
	Range bool   `@Range?`
	Max   *int   `@Int?`
}

// ----------------------------------------------------------------------------
// Expressions
//
// Expression precedence (lowest to highest):
// 1. OR
// 2. XOR
// 3. AND
// 4. NOT
// 5. Comparison (=, <>, <, >, <=, >=)
// 6. Addition/Subtraction (+, -)
// 7. Multiplication/Division/Modulo (*, /, %)
// 8. Power (^)
// 9. Unary (-, +)
// 10. Postfix (property access, indexing, IS NULL, IN, STARTS WITH, etc.)
// 11. Atom (literals, variables, function calls, etc.)
// ----------------------------------------------------------------------------

// Expression is the top-level expression type (OR).
type Expression struct {
	Pos   lexer.Position
	Left  *XorExpr  `@@`
	Right []*OrTerm `@@*`
}

// OrTerm is an OR operand.
type OrTerm struct {
	Pos  lexer.Position
	Expr *XorExpr `"OR" @@`
}

// XorExpr handles XOR.
type XorExpr struct {
	Pos   lexer.Position
	Left  *AndExpr   `@@`
	Right []*XorTerm `@@*`
}

// XorTerm is an XOR operand.
type XorTerm struct {
	Pos  lexer.Position
	Expr *AndExpr `"XOR" @@`
}

// AndExpr handles AND.
type AndExpr struct {
	Pos   lexer.Position
	Left  *NotExpr   `@@`
	Right []*AndTerm `@@*`
}

// AndTerm is an AND operand.
type AndTerm struct {
	Pos  lexer.Position
	Expr *NotExpr `"AND" @@`
}

// NotExpr handles NOT.
type NotExpr struct {
	Pos  lexer.Position
	Not  bool            `@"NOT"?`
	Expr *ComparisonExpr `@@`
}

// ComparisonExpr handles comparisons.
type ComparisonExpr struct {
	Pos   lexer.Position
	Left  *AddSubExpr       `@@`
	Right []*ComparisonTerm `@@*`
}

// ComparisonTerm is a comparison operator and operand.
type ComparisonTerm struct {
	Pos  lexer.Position
	Op   string      `@( NotEqual | LessEqual | GreaterEqual | Eq | Less | Greater )`
	Expr *AddSubExpr `@@`
}

// AddSubExpr handles + and -.
type AddSubExpr struct {
	Pos   lexer.Position
	Left  *MultDivExpr  `@@`
	Right []*AddSubTerm `@@*`
}

// AddSubTerm is a + or - operand.
type AddSubTerm struct {
	Pos  lexer.Position
	Op   string       `@( Plus | Minus )`
	Expr *MultDivExpr `@@`
}

// MultDivExpr handles *, /, %.
type MultDivExpr struct {
	Pos   lexer.Position
	Left  *PowerExpr     `@@`
	Right []*MultDivTerm `@@*`
}

// MultDivTerm is a *, /, or % operand.
type MultDivTerm struct {
	Pos  lexer.Position
	Op   string     `@( Star | Slash | Percent )`
	Expr *PowerExpr `@@`
}

// PowerExpr handles ^.
type PowerExpr struct {
	Pos   lexer.Position
	Left  *UnaryExpr   `@@`
	Right []*PowerTerm `@@*`
}

// PowerTerm is a ^ operand.
type PowerTerm struct {
	Pos  lexer.Position
	Expr *UnaryExpr `Caret @@`
}

// UnaryExpr handles unary + and -.
type UnaryExpr struct {
	Pos  lexer.Position
	Op   string       `@( Plus | Minus )?`
	Expr *PostfixExpr `@@`
}

// PostfixExpr handles property access, indexing, and predicates.
type PostfixExpr struct {
	Pos      lexer.Position
	Atom     *Atom            `@@`
	Suffixes []*PostfixSuffix `@@*`
}

// PostfixSuffix is a property access, index, or predicate.
type PostfixSuffix struct {
	Pos        lexer.Position
	Property   string            `  Dot @Ident`
	Index      *IndexSuffix      `| @@`
	Labels     *NodeLabels       `| @@`
	IsNull     *IsNullSuffix     `| @@`
	In         *InSuffix         `| @@`
	StringPred *StringPredSuffix `| @@`
}

// IndexSuffix is [expr] or [start..end].
type IndexSuffix struct {
	Pos   lexer.Position
	Start *Expression `LBracket @@?`
	Range bool        `@Range?`
	End   *Expression `@@? RBracket`
}

// IsNullSuffix is IS NOT? NULL.
type IsNullSuffix struct {
	Pos  lexer.Position
	Not  bool `"IS" @"NOT"?`
	Null bool `@"NULL"`
}

// InSuffix is IN expression.
type InSuffix struct {
	Pos  lexer.Position
	Expr *AddSubExpr `"IN" @@` // Use AddSubExpr to avoid left recursion with full Expression
}

// StringPredSuffix is STARTS WITH, ENDS WITH, or CONTAINS.
type StringPredSuffix struct {
	Pos        lexer.Position
	StartsWith *AddSubExpr `  "STARTS" "WITH" @@`
	EndsWith   *AddSubExpr `| "ENDS" "WITH" @@`
	Contains   *AddSubExpr `| "CONTAINS" @@`
}

// ----------------------------------------------------------------------------
// Atoms
// ----------------------------------------------------------------------------

// Atom is the base expression type.
// Order matters for disambiguation:
// 1. List/pattern comprehension before list literal (both start with [)
// 2. COUNT(*) has special syntax
// 3. FunctionCall uses lookahead to require LParen, so safe to try first
// 4. Variable is a fallback Ident
type Atom struct {
	Pos lexer.Position
	// List comprehension MUST come before literal to handle [x IN list | expr]
	ListComprehension    *ListComprehension    `  @@`
	PatternComprehension *PatternComprehension `| @@`
	Parameter            *Parameter            `| @@`
	CaseExpr             *CaseExpression       `| @@`
	CountAll             bool                  `| @( "COUNT" LParen Star RParen )`
	FilterPredicate      *FilterPredicate      `| @@`
	ExistsSubquery       *ExistsSubquery       `| @@`
	Parenthesized        *Expression           `| LParen @@ RParen`
	// FunctionCall uses lookahead for LParen, so it only matches actual function calls
	FunctionCall *FunctionCall `| @@`
	Literal      *Literal      `| @@`
	Variable     string        `| @Ident`
}

// Literal is a constant value.
type Literal struct {
	Pos    lexer.Position
	Null   bool         `  @"NULL"`
	True   bool         `| @"TRUE"`
	False  bool         `| @"FALSE"`
	Float  *float64     `| @Float`
	HexInt *string      `| @HexInt`
	OctInt *string      `| @OctalInt`
	Int    *int64       `| @Int`
	String *string      `| @String`
	List   *ListLiteral `| @@`
	Map    *MapLiteral  `| @@`
}

// ListLiteral is [expr, expr, ...].
// We need to be careful to not conflict with list comprehension.
type ListLiteral struct {
	Pos lexer.Position
	// Use negative lookahead: not followed by Ident "IN"
	// Actually, participle handles this by trying alternatives in order
	// We'll put ListComprehension before this in Atom
	Items []*Expression `LBracket ( @@ ( Comma @@ )* )? RBracket`
}

// MapLiteral is {key: value, ...}.
type MapLiteral struct {
	Pos   lexer.Position
	Pairs []*MapPair `LBrace ( @@ ( Comma @@ )* )? RBrace`
}

// MapPair is key: value in a map literal.
type MapPair struct {
	Pos   lexer.Position
	Key   string      `@Ident Colon`
	Value *Expression `@@`
}

// Parameter is $name or $0.
type Parameter struct {
	Pos  lexer.Position
	Name string `Dollar ( @Ident | @Int )`
}

// ListComprehension is [variable IN list WHERE condition | mapping].
type ListComprehension struct {
	Pos      lexer.Position
	Variable string      `LBracket @Ident "IN"`
	Source   *Expression `@@`
	Where    *Where      `@@?`
	Mapping  *Expression `( Pipe @@ )? RBracket`
}

// PatternComprehension is [(var =)? pattern WHERE condition | mapping].
type PatternComprehension struct {
	Pos     lexer.Position
	Var     string                    `LBracket ( @Ident Eq )?`
	Pattern *RelationshipChainPattern `@@`
	Where   *Where                    `@@?`
	Mapping *Expression               `Pipe @@ RBracket`
}

// RelationshipChainPattern is a node pattern with at least one relationship chain.
type RelationshipChainPattern struct {
	Pos   lexer.Position
	Node  *NodePattern        `@@`
	Chain []*PatternElemChain `@@+`
}

// FilterPredicate is ALL/ANY/NONE/SINGLE(filterExpression).
type FilterPredicate struct {
	Pos      lexer.Position
	Type     string      `@( "ALL" | "ANY" | "NONE" | "SINGLE" )`
	Variable string      `LParen @Ident "IN"`
	Source   *Expression `@@`
	Where    *Where      `@@? RParen`
}

// ExistsSubquery is EXISTS { subquery }.
type ExistsSubquery struct {
	Pos     lexer.Position
	Query   *RegularQuery `"EXISTS" LBrace ( @@`
	Pattern *Pattern      `         | @@ ) RBrace`
}

// CaseExpression is CASE expr? (WHEN expr THEN expr)+ (ELSE expr)? END.
// Simple CASE: CASE expr WHEN val THEN result ... END
// Searched CASE: CASE WHEN cond THEN result ... END
type CaseExpression struct {
	Pos lexer.Position
	// Use negative lookahead: don't match Input if next token is WHEN
	Input *Expression `"CASE" ( (?! "WHEN" ) @@ )?`
	Whens []*CaseWhen `@@+`
	Else  *Expression `( "ELSE" @@ )?`
	End   bool        `@"END"`
}

// CaseWhen is WHEN condition THEN result.
type CaseWhen struct {
	Pos  lexer.Position
	When *Expression `"WHEN" @@`
	Then *Expression `"THEN" @@`
}

// FunctionCall is name(args).
// We use positive lookahead to ensure there's an LParen after the name,
// otherwise we'd greedily consume property access chains like u.address.city.
type FunctionCall struct {
	Pos      lexer.Position
	Name     *InvocationName `@@ (?= LParen )`
	Distinct bool            `LParen @"DISTINCT"?`
	Args     []*Expression   `( @@ ( Comma @@ )* )? RParen`
}

// InvocationName is a possibly namespaced identifier (e.g., apoc.text.join).
type InvocationName struct {
	Pos   lexer.Position
	Parts []string `@Ident ( Dot @Ident )*`
}

// ParenExprList is (expr, expr, ...).
type ParenExprList struct {
	Pos   lexer.Position
	Exprs []*Expression `LParen ( @@ ( Comma @@ )* )? RParen`
}

// PropertyExpr is a chain of property accesses: a.b.c
type PropertyExpr struct {
	Pos   lexer.Position
	Base  string   `@Ident`
	Props []string `( Dot @Ident )*`
}

// ----------------------------------------------------------------------------
// Helper types for SetItem parsing
// We need to restructure SetItem because the current definition is ambiguous.
// ----------------------------------------------------------------------------

// SetItemType1 is property = expr
type SetItemProperty struct {
	Pos      lexer.Position
	Property *PropertyExpr `@@`
	Expr     *Expression   `Eq @@`
}

// SetItemType2 is variable = expr or variable += expr
type SetItemVariable struct {
	Pos       lexer.Position
	Variable  string      `@Ident`
	AddAssign bool        `( @AddAssign`
	Assign    bool        `| @Eq )`
	Expr      *Expression `@@`
}

// SetItemType3 is variable:Label:Label
type SetItemLabel struct {
	Pos      lexer.Position
	Variable string      `@Ident`
	Labels   *NodeLabels `@@`
}
