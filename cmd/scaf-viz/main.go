package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
	"github.com/rlch/scaf"
)

//go:embed static/*
var staticFS embed.FS

func main() {
	mux := http.NewServeMux()

	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("/", http.FileServer(http.FS(static)))
	mux.HandleFunc("/api/analyze", handleAnalyze)

	fmt.Println("scaf-viz running at http://localhost:8765")
	log.Fatal(http.ListenAndServe(":8765", mux))
}

type AnalyzeRequest struct {
	Source string `json:"source"`
}

type TokenInfo struct {
	Type     string `json:"type"`
	TypeName string `json:"typeName"`
	Value    string `json:"value"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Offset   int    `json:"offset"`
	EndOff   int    `json:"endOffset"`
}

type ASTNode struct {
	Type      string     `json:"type"`
	Name      string     `json:"name,omitempty"`
	Value     string     `json:"value,omitempty"`
	Line      int        `json:"line"`
	Column    int        `json:"column"`
	EndLine   int        `json:"endLine"`
	EndCol    int        `json:"endColumn"`
	Children  []*ASTNode `json:"children,omitempty"`
	Recovered bool       `json:"recovered,omitempty"`
}

type AnalyzeResponse struct {
	Tokens   []TokenInfo    `json:"tokens"`
	AST      *ASTNode       `json:"ast"`
	Errors   []string       `json:"errors"`
	Recovery []RecoveryInfo `json:"recovery,omitempty"`
}

type RecoveryInfo struct {
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Skipped string `json:"skipped"`
}

var tokenNames = map[lexer.TokenType]string{
	scaf.TokenEOF:        "EOF",
	scaf.TokenComment:    "Comment",
	scaf.TokenRawString:  "RawString",
	scaf.TokenString:     "String",
	scaf.TokenNumber:     "Number",
	scaf.TokenIdent:      "Ident",
	scaf.TokenOp:         "Op",
	scaf.TokenDot:        "Dot",
	scaf.TokenColon:      "Colon",
	scaf.TokenComma:      "Comma",
	scaf.TokenSemi:       "Semi",
	scaf.TokenLParen:     "LParen",
	scaf.TokenRParen:     "RParen",
	scaf.TokenLBracket:   "LBracket",
	scaf.TokenRBracket:   "RBracket",
	scaf.TokenLBrace:     "LBrace",
	scaf.TokenRBrace:     "RBrace",
	scaf.TokenWhitespace: "Whitespace",
	scaf.TokenFn:         "fn",
	scaf.TokenImport:     "import",
	scaf.TokenSetup:      "setup",
	scaf.TokenTeardown:   "teardown",
	scaf.TokenTest:       "test",
	scaf.TokenGroup:      "group",
	scaf.TokenAssert:     "assert",
	scaf.TokenWhere:      "where",
	scaf.TokenQuestion:   "Question",
}

func handleAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var req AnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp := analyze(req.Source)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func analyze(source string) AnalyzeResponse {
	resp := AnalyzeResponse{
		Tokens: []TokenInfo{},
		Errors: []string{},
	}

	// Tokenize
	lex := scaf.ExportedLexer()
	l, err := lex.LexString("input.scaf", source)
	if err != nil {
		resp.Errors = append(resp.Errors, "Lexer error: "+err.Error())
		return resp
	}

	for {
		tok, err := l.Next()
		if err != nil {
			resp.Errors = append(resp.Errors, "Token error: "+err.Error())
			break
		}

		typeName := tokenNames[tok.Type]
		if typeName == "" {
			typeName = fmt.Sprintf("Unknown(%d)", tok.Type)
		}

		// Skip whitespace in display (but keep for position tracking)
		if tok.Type == scaf.TokenWhitespace {
			if tok.Type == lexer.EOF {
				break
			}
			continue
		}

		endOff := tok.Pos.Offset + len(tok.Value)

		resp.Tokens = append(resp.Tokens, TokenInfo{
			Type:     fmt.Sprintf("%d", tok.Type),
			TypeName: typeName,
			Value:    tok.Value,
			Line:     tok.Pos.Line,
			Column:   tok.Pos.Column,
			Offset:   tok.Pos.Offset,
			EndOff:   endOff,
		})

		if tok.Type == lexer.EOF {
			break
		}
	}

	// Parse with recovery
	file, parseErr := scaf.ParseWithRecovery([]byte(source), true)
	if parseErr != nil {
		resp.Errors = append(resp.Errors, parseErr.Error())
	}

	if file != nil {
		resp.AST = buildASTNode(file)
		resp.Recovery = extractRecovery(file)
	}

	return resp
}

func buildASTNode(file *scaf.File) *ASTNode {
	root := &ASTNode{
		Type:     "File",
		Line:     file.Pos.Line,
		Column:   file.Pos.Column,
		EndLine:  file.EndPos.Line,
		EndCol:   file.EndPos.Column,
		Children: []*ASTNode{},
	}

	for _, imp := range file.Imports {
		child := &ASTNode{
			Type:    "Import",
			Name:    deref(imp.Alias),
			Value:   imp.Path,
			Line:    imp.Pos.Line,
			Column:  imp.Pos.Column,
			EndLine: imp.EndPos.Line,
			EndCol:  imp.EndPos.Column,
		}
		root.Children = append(root.Children, child)
	}

	for _, fn := range file.Functions {
		child := &ASTNode{
			Type:     "Function",
			Name:     fn.Name,
			Value:    truncate(fn.Body, 50),
			Line:     fn.Pos.Line,
			Column:   fn.Pos.Column,
			EndLine:  fn.EndPos.Line,
			EndCol:   fn.EndPos.Column,
			Children: []*ASTNode{},
		}
		for _, p := range fn.Params {
			pNode := &ASTNode{
				Type:   "Param",
				Name:   p.Name,
				Value:  typeExprStr(p.Type),
				Line:   p.Pos.Line,
				Column: p.Pos.Column,
			}
			child.Children = append(child.Children, pNode)
		}
		root.Children = append(root.Children, child)
	}

	if file.Setup != nil {
		root.Children = append(root.Children, setupNode(file.Setup))
	}

	for _, scope := range file.Scopes {
		sNode := &ASTNode{
			Type:      "FunctionScope",
			Name:      scope.FunctionName,
			Line:      scope.Pos.Line,
			Column:    scope.Pos.Column,
			EndLine:   scope.EndPos.Line,
			EndCol:    scope.EndPos.Column,
			Children:  []*ASTNode{},
			Recovered: scope.WasRecovered(),
		}

		if scope.Setup != nil {
			sNode.Children = append(sNode.Children, setupNode(scope.Setup))
		}

		for _, item := range scope.Items {
			if item.Test != nil {
				sNode.Children = append(sNode.Children, testNode(item.Test))
			}
			if item.Group != nil {
				sNode.Children = append(sNode.Children, groupNode(item.Group))
			}
		}

		root.Children = append(root.Children, sNode)
	}

	return root
}

func setupNode(s *scaf.SetupClause) *ASTNode {
	n := &ASTNode{
		Type:   "Setup",
		Line:   s.Pos.Line,
		Column: s.Pos.Column,
	}
	if s.Inline != nil {
		n.Value = truncate(*s.Inline, 40)
	} else if s.Module != nil {
		n.Value = *s.Module
	} else if s.Call != nil {
		n.Value = s.Call.Module + "." + s.Call.Query + "()"
	}
	return n
}

func testNode(t *scaf.Test) *ASTNode {
	n := &ASTNode{
		Type:      "Test",
		Name:      t.Name,
		Line:      t.Pos.Line,
		Column:    t.Pos.Column,
		EndLine:   t.EndPos.Line,
		EndCol:    t.EndPos.Column,
		Children:  []*ASTNode{},
		Recovered: t.WasRecovered(),
	}

	for _, stmt := range t.Statements {
		sn := &ASTNode{
			Type:   "Statement",
			Name:   stmt.Key(),
			Value:  stmt.Value.String(),
			Line:   stmt.Pos.Line,
			Column: stmt.Pos.Column,
		}
		n.Children = append(n.Children, sn)
	}

	for _, a := range t.Asserts {
		an := &ASTNode{
			Type:     "Assert",
			Line:     a.Pos.Line,
			Column:   a.Pos.Column,
			Children: []*ASTNode{},
		}
		for _, cond := range a.AllConditions() {
			cn := &ASTNode{
				Type:  "Condition",
				Value: cond.String(),
				Line:  cond.Pos.Line,
			}
			an.Children = append(an.Children, cn)
		}
		n.Children = append(n.Children, an)
	}

	return n
}

func groupNode(g *scaf.Group) *ASTNode {
	n := &ASTNode{
		Type:      "Group",
		Name:      g.Name,
		Line:      g.Pos.Line,
		Column:    g.Pos.Column,
		EndLine:   g.EndPos.Line,
		EndCol:    g.EndPos.Column,
		Children:  []*ASTNode{},
		Recovered: g.WasRecovered(),
	}

	for _, item := range g.Items {
		if item.Test != nil {
			n.Children = append(n.Children, testNode(item.Test))
		}
		if item.Group != nil {
			n.Children = append(n.Children, groupNode(item.Group))
		}
	}

	return n
}

func extractRecovery(file *scaf.File) []RecoveryInfo {
	var recoveries []RecoveryInfo

	if file.WasRecovered() {
		recoveries = append(recoveries, RecoveryInfo{
			Line:    file.RecoveredSpan.Line,
			Column:  file.RecoveredSpan.Column,
			Skipped: file.RecoveredText(),
		})
	}

	for _, scope := range file.Scopes {
		if scope.WasRecovered() {
			recoveries = append(recoveries, RecoveryInfo{
				Line:    scope.RecoveredSpan.Line,
				Column:  scope.RecoveredSpan.Column,
				Skipped: scope.RecoveredText(),
			})
		}
		for _, item := range scope.Items {
			if item.Test != nil && item.Test.WasRecovered() {
				recoveries = append(recoveries, RecoveryInfo{
					Line:    item.Test.RecoveredSpan.Line,
					Column:  item.Test.RecoveredSpan.Column,
					Skipped: item.Test.RecoveredText(),
				})
			}
		}
	}

	return recoveries
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func typeExprStr(t *scaf.TypeExpr) string {
	if t == nil {
		return "any"
	}
	return t.ToGoType()
}
