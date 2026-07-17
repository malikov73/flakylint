// Package parallelglobal reports writes to package-level variables from
// tests that call t.Parallel().
//
// Parallel tests sharing mutable package state race with each other; the
// race only manifests under CI parallelism and ordering, which makes such
// tests flaky rather than deterministically broken.
package parallelglobal

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/malikov73/flakylint/internal/testfuncs"
)

var Analyzer = &analysis.Analyzer{
	Name:     "parallelglobal",
	Doc:      "reports writes to package-level variables in tests marked t.Parallel(); shared state races between parallel tests",
	URL:      "https://github.com/malikov73/flakylint",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil), (*ast.CallExpr)(nil)}, func(n ast.Node) {
		if !testfuncs.InTestFile(pass, n) {
			return
		}
		var body *ast.BlockStmt
		switch n := n.(type) {
		case *ast.FuncDecl:
			if _, ok := testfuncs.TestFunc(pass.TypesInfo, n); !ok {
				return
			}
			body = n.Body
		case *ast.CallExpr:
			lit, _, ok := testfuncs.SubtestLit(pass.TypesInfo, n)
			if !ok {
				return
			}
			body = lit.Body
		}
		checkUnit(pass, body)
	})
	return nil, nil
}

// checkUnit analyzes one test unit (test function body or subtest literal
// body). Nested subtest literals are excluded from both parallel detection
// and write detection: each of them is analyzed as its own unit.
func checkUnit(pass *analysis.Pass, body *ast.BlockStmt) {
	info := pass.TypesInfo

	nested := map[*ast.FuncLit]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if lit, _, ok := testfuncs.SubtestLit(info, call); ok {
				nested[lit] = true
			}
		}
		return true
	})
	skipNested := func(n ast.Node) bool {
		lit, ok := n.(*ast.FuncLit)
		return ok && nested[lit]
	}

	parallel := false
	ast.Inspect(body, func(n ast.Node) bool {
		if skipNested(n) {
			return false
		}
		if call, ok := n.(*ast.CallExpr); ok && testfuncs.IsTestingMethod(info, call, "Parallel") {
			parallel = true
		}
		return true
	})
	if !parallel {
		return
	}

	// One diagnostic per variable: the first write in source order is
	// reported, later writes to the same var in this unit are collapsed.
	reported := map[types.Object]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		if skipNested(n) {
			return false
		}
		switch st := n.(type) {
		case *ast.AssignStmt:
			if st.Tok == token.DEFINE {
				return true // := always declares locals
			}
			for _, lhs := range st.Lhs {
				reportGlobalWrite(pass, lhs, reported)
			}
		case *ast.IncDecStmt:
			reportGlobalWrite(pass, st.X, reported)
		}
		return true
	})
}

func reportGlobalWrite(pass *analysis.Pass, target ast.Expr, reported map[types.Object]bool) {
	obj := targetObj(pass.TypesInfo, target)
	if obj == nil || !isGlobalVar(obj) || reported[obj] {
		return
	}
	reported[obj] = true
	pass.Reportf(target.Pos(),
		"parallel test writes package-level variable %q; parallel tests sharing state race with each other", obj.Name())
}

// targetObj resolves the root object of a write target: counter, state["k"],
// global.field, *ptr, and pkg.Var all resolve to the underlying variable.
func targetObj(info *types.Info, e ast.Expr) types.Object {
	for {
		switch x := e.(type) {
		case *ast.Ident:
			return info.ObjectOf(x)
		case *ast.SelectorExpr:
			if id, ok := x.X.(*ast.Ident); ok {
				if _, isPkg := info.ObjectOf(id).(*types.PkgName); isPkg {
					return info.ObjectOf(x.Sel) // qualified cross-package var
				}
			}
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.StarExpr:
			e = x.X
		case *ast.ParenExpr:
			e = x.X
		default:
			return nil
		}
	}
}

func isGlobalVar(obj types.Object) bool {
	v, ok := obj.(*types.Var)
	return ok && v.Pkg() != nil && v.Parent() == v.Pkg().Scope()
}
