// Package httptestclose reports httptest servers that are never closed.
//
// A leaked httptest.Server keeps a port bound and goroutines running after
// the test finishes; the GC finalizer then races with the HTTP client's
// connection pool, which is a documented source of test flakiness
// (see google/go-github#4210).
package httptestclose

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/malikov73/flakylint/internal/testfuncs"
)

var Analyzer = &analysis.Analyzer{
	Name:     "httptestclose",
	Doc:      "reports httptest servers that are never closed; the leaked port and goroutines can make later tests flaky",
	URL:      "https://github.com/malikov73/flakylint",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

var constructors = map[string]bool{
	"NewServer":          true,
	"NewTLSServer":       true,
	"NewUnstartedServer": true,
}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.WithStack([]ast.Node{(*ast.AssignStmt)(nil)}, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return false
		}
		assign := n.(*ast.AssignStmt)
		if !testfuncs.InTestFile(pass, assign) {
			return false
		}
		if assign.Tok != token.DEFINE || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok || !isConstructor(pass.TypesInfo, call) {
			return true
		}
		id, ok := assign.Lhs[0].(*ast.Ident)
		if !ok || id.Name == "_" {
			return true
		}
		obj := pass.TypesInfo.Defs[id]
		if obj == nil {
			return true
		}
		body := enclosingFuncBody(stack)
		if body == nil {
			return true
		}
		hasClose, escapes := analyzeUses(body, obj, id, pass.TypesInfo)
		if hasClose || escapes {
			return true
		}
		report(pass, assign, id, stack)
		return true
	})
	return nil, nil
}

func isConstructor(info *types.Info, call *ast.CallExpr) bool {
	for name := range constructors {
		if testfuncs.IsPkgFunc(info, call, "net/http/httptest", name) {
			return true
		}
	}
	return false
}

// analyzeUses scans body for uses of obj. A use as the base of a selector
// (srv.URL, srv.Close, ...) is benign; srv.Close sets hasClose. Any other
// use (call argument, return value, assignment RHS, &srv, ...) means the
// server escapes and we stay silent.
func analyzeUses(body ast.Node, obj types.Object, def *ast.Ident, info *types.Info) (hasClose, escapes bool) {
	benign := map[*ast.Ident]bool{def: true}
	ast.Inspect(body, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok || info.Uses[id] != obj {
			return true
		}
		benign[id] = true
		if sel.Sel.Name == "Close" {
			hasClose = true
		}
		return true
	})
	ast.Inspect(body, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || benign[id] {
			return true
		}
		if info.Uses[id] == obj {
			escapes = true
		}
		return true
	})
	return hasClose, escapes
}

// enclosingFuncBody returns the body of the innermost function in the stack.
func enclosingFuncBody(stack []ast.Node) *ast.BlockStmt {
	for i := len(stack) - 1; i >= 0; i-- {
		switch fn := stack[i].(type) {
		case *ast.FuncLit:
			return fn.Body
		case *ast.FuncDecl:
			return fn.Body
		}
	}
	return nil
}

// enclosingTestParam returns the name of the *testing.T/B/F parameter of the
// innermost enclosing test function or subtest literal, or "".
func enclosingTestParam(info *types.Info, stack []ast.Node) string {
	for i := len(stack) - 1; i >= 0; i-- {
		switch fn := stack[i].(type) {
		case *ast.FuncLit:
			if i > 0 {
				if call, ok := stack[i-1].(*ast.CallExpr); ok {
					if _, param, ok := testfuncs.SubtestLit(info, call); ok {
						return param.Name
					}
				}
			}
			return ""
		case *ast.FuncDecl:
			if param, ok := testfuncs.TestFunc(info, fn); ok {
				return param.Name
			}
			return ""
		}
	}
	return ""
}

func report(pass *analysis.Pass, assign *ast.AssignStmt, id *ast.Ident, stack []ast.Node) {
	indent := strings.Repeat("\t", max(0, pass.Fset.Position(assign.Pos()).Column-1))
	var fixText, fixMsg string
	if tname := enclosingTestParam(pass.TypesInfo, stack); tname != "" && tname != "_" {
		fixText = "\n" + indent + tname + ".Cleanup(" + id.Name + ".Close)"
		fixMsg = "register " + id.Name + ".Close with " + tname + ".Cleanup"
	} else {
		fixText = "\n" + indent + "defer " + id.Name + ".Close()"
		fixMsg = "defer " + id.Name + ".Close()"
	}
	pass.Report(analysis.Diagnostic{
		Pos:     assign.Pos(),
		End:     assign.End(),
		Message: "httptest server is never closed; the leaked port and goroutines can make later tests flaky",
		SuggestedFixes: []analysis.SuggestedFix{{
			Message: fixMsg,
			TextEdits: []analysis.TextEdit{{
				Pos:     assign.End(),
				End:     assign.End(),
				NewText: []byte(fixText),
			}},
		}},
	})
}
