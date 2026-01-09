package scaf

import "github.com/alecthomas/participle/v2/lexer"

// Span represents a range in source code.
type Span struct {
	Start lexer.Position
	End   lexer.Position
}

// Trivia represents non-semantic tokens like comments and whitespace.
type Trivia struct {
	Type TriviaType
	Text string
	Span Span
	// HasNewlineBefore is true if there was a blank line before this trivia.
	// Useful for distinguishing "detached" comments.
	HasNewlineBefore bool
}

// TriviaType distinguishes different kinds of trivia.
type TriviaType int

// TriviaType constants define the types of trivia (comments, whitespace).
const (
	// TriviaComment represents a comment trivia.
	TriviaComment TriviaType = iota
	// TriviaWhitespace represents whitespace trivia.
	TriviaWhitespace
)

// TriviaList holds all trivia collected during lexing.
// It's associated with tokens by position - trivia "attaches" to the next
// real token as leading trivia, except trailing comments (same line) attach
// to the previous token.
type TriviaList struct {
	items []Trivia
}

// Add appends trivia to the list.
func (t *TriviaList) Add(trivia Trivia) {
	t.items = append(t.items, trivia)
}

// All returns all collected trivia.
func (t *TriviaList) All() []Trivia {
	return t.items
}

// Reset clears the trivia list.
func (t *TriviaList) Reset() {
	t.items = t.items[:0]
}

// commentableNode represents an AST node that can have comments attached.
// We store both the span (for position matching) and a pointer to the CommentMeta.
type commentableNode struct {
	span    Span
	comment *CommentMeta
}

// attachComments associates collected trivia with AST nodes based on positions,
// applying the comments directly to the node fields.
//
// Comment attachment rules:
//   - Comments on the same line after a node are trailing comments for that node
//   - Comments before a node attach to the closest following node
//   - Comments at the very top of the file (before any declarations) attach to Suite
//     only if separated from the first declaration by a blank line
func attachComments(suite *Suite, trivia *TriviaList) {
	if trivia == nil || len(trivia.items) == 0 || suite == nil {
		return
	}

	allTrivia := trivia.All()

	// Collect all commentable nodes (excluding Suite - handled separately)
	var nodes []commentableNode
	collectCommentableNodes(suite, &nodes)

	// Find the first declaration's line number for Suite comment detection
	firstDeclLine := findFirstDeclarationLine(suite)

	// Extract just the comments for easier processing
	var comments []Trivia
	for _, t := range allTrivia {
		if t.Type == TriviaComment {
			comments = append(comments, t)
		}
	}

	// For each comment, find which node it belongs to
	for _, t := range comments {
		commentText := t.Text
		attached := false

		// Check if it's a trailing comment (same line as end of some node)
		for j := range nodes {
			node := &nodes[j]
			// Trailing: comment starts on same line as node ends, after the node
			if t.Span.Start.Line == node.span.End.Line && t.Span.Start.Offset > node.span.End.Offset {
				node.comment.TrailingComment = commentText
				attached = true

				break
			}
		}

		if attached {
			continue
		}

		// Leading: find the closest node that starts after this comment
		var closestNode *commentableNode
		for j := range nodes {
			node := &nodes[j]
			// Comment ends before node starts (on previous line or same line before)
			if t.Span.End.Line < node.span.Start.Line ||
				(t.Span.End.Line == node.span.Start.Line && t.Span.End.Offset < node.span.Start.Offset) {
				// Check if this is the closest node
				if closestNode == nil || node.span.Start.Line < closestNode.span.Start.Line ||
					(node.span.Start.Line == closestNode.span.Start.Line && node.span.Start.Offset < closestNode.span.Start.Offset) {
					closestNode = node
				}
			}
		}

		if closestNode != nil {
			// Check if this comment should be a Suite comment.
			// Suite comments are before the first declaration AND part of a comment block
			// that is separated by a blank line from the first declaration (or its doc comments).
			isFirstDecl := closestNode.span.Start.Line == firstDeclLine

			shouldAttachToSuite := false
			if isFirstDecl {
				// Find where the "doc comment block" for the first declaration starts.
				// Walk backwards from the declaration to find consecutive comments (no blank lines).
				docBlockStartLine := closestNode.span.Start.Line

				// Find the last comment before the declaration and trace back through
				// consecutive comments to find where the doc block starts
				for j := len(comments) - 1; j >= 0; j-- {
					c := comments[j]
					if c.Span.End.Line >= closestNode.span.Start.Line {
						continue // Comment is on or after the declaration
					}
					// Check if this comment is consecutive with docBlockStartLine
					if c.Span.End.Line == docBlockStartLine-1 {
						docBlockStartLine = c.Span.Start.Line
					} else if c.Span.End.Line < docBlockStartLine-1 {
						// There's a gap - this comment is before the doc block
						break
					}
				}

				// This comment should go to Suite if it ends before the doc block starts
				// (i.e., there's a blank line between this comment and the doc block)
				shouldAttachToSuite = t.Span.End.Line < docBlockStartLine-1
			}

			if shouldAttachToSuite {
				// Comment separated by blank line -> Suite comment
				suite.LeadingComments = append(suite.LeadingComments, commentText)
			} else {
				// Comment directly before node -> leading comment for that node
				closestNode.comment.LeadingComments = append(closestNode.comment.LeadingComments, commentText)
			}
			attached = true
		}

		// If comment wasn't attached to any declaration (e.g., file with no declarations),
		// attach to Suite
		if !attached {
			suite.LeadingComments = append(suite.LeadingComments, commentText)
		}
	}
}

// findFirstDeclarationLine returns the line number of the first declaration
// (import, query, setup, teardown, or scope). Returns 0 if no declarations.
func findFirstDeclarationLine(suite *Suite) int {
	if suite == nil {
		return 0
	}

	firstLine := 0

	// Check imports
	for _, imp := range suite.Imports {
		if firstLine == 0 || imp.Pos.Line < firstLine {
			firstLine = imp.Pos.Line
		}
	}

	// Check queries
	for _, q := range suite.Functions {
		if firstLine == 0 || q.Pos.Line < firstLine {
			firstLine = q.Pos.Line
		}
	}

	// Check scopes
	for _, scope := range suite.Scopes {
		if firstLine == 0 || scope.Pos.Line < firstLine {
			firstLine = scope.Pos.Line
		}
	}

	return firstLine
}

// collectCommentableNodes gathers all nodes that can have comments attached.
// Suite is excluded - it's handled specially for file-level comments.
func collectCommentableNodes(suite *Suite, nodes *[]commentableNode) {
	if suite == nil {
		return
	}

	// NOTE: We intentionally do NOT include Suite here.
	// Suite only gets comments that are separated by a blank line from
	// the first declaration.

	// Helper to add a commentable node
	add := func(c Commentable) {
		*nodes = append(*nodes, commentableNode{
			span:    c.Span(),
			comment: c.Comments(),
		})
	}

	// Collect imports
	for _, imp := range suite.Imports {
		add(imp)
	}

	// Collect functions and their parameters
	for _, fn := range suite.Functions {
		add(fn)
		for _, p := range fn.Params {
			if p != nil {
				add(p)
			}
		}
	}

	// Collect scopes and their contents
	for _, scope := range suite.Scopes {
		collectScopeNodes(scope, add)
	}
}

// collectScopeNodes collects all commentable nodes within a scope.
func collectScopeNodes(scope *QueryScope, add func(Commentable)) {
	if scope == nil {
		return
	}

	add(scope)
	collectItemNodes(scope.Items, add)
}

// collectItemNodes collects all commentable nodes within test/group items.
func collectItemNodes(items []*TestOrGroup, add func(Commentable)) {
	for _, item := range items {
		if item.Test != nil {
			collectTestNodes(item.Test, add)
		}
		if item.Group != nil {
			collectGroupNodes(item.Group, add)
		}
	}
}

// collectGroupNodes collects all commentable nodes within a group.
func collectGroupNodes(group *Group, add func(Commentable)) {
	if group == nil {
		return
	}

	add(group)
	collectItemNodes(group.Items, add)
}

// collectTestNodes collects all commentable nodes within a test.
func collectTestNodes(test *Test, add func(Commentable)) {
	if test == nil {
		return
	}

	add(test)

	// Collect statements
	for _, stmt := range test.Statements {
		if stmt != nil {
			add(stmt)
		}
	}

	// Collect asserts
	for _, assert := range test.Asserts {
		if assert != nil {
			add(assert)
		}
	}
}
