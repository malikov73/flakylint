// Package sleepassert reports time.Sleep calls in test bodies.
//
// Synchronizing a test on real time races the goroutine scheduler and the
// CI machine's load; such tests pass locally and flake under CI pressure.
// Prefer testing/synctest (Go 1.25+) or explicit synchronization.
package sleepassert

import (
	"go/ast"
	"go/constant"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/malikov73/flakylint/internal/testfuncs"
)

var Analyzer = &analysis.Analyzer{
	Name:     "sleepassert",
	Doc:      "reports time.Sleep in test bodies; sleeping to wait for concurrent work is a common source of flakiness",
	URL:      "https://github.com/malikov73/flakylint",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

const msg = "time.Sleep synchronizes the test on real time and flakes under CI load; use testing/synctest (Go 1.25+) or explicit synchronization (channel, sync.WaitGroup)"

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.WithStack([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return false
		}
		call := n.(*ast.CallExpr)
		if !testfuncs.InTestFile(pass, call) {
			return false
		}
		if !testfuncs.IsPkgFunc(pass.TypesInfo, call, "time", "Sleep") {
			return true
		}
		if len(call.Args) == 1 && isZeroConst(pass, call.Args[0]) {
			return true
		}
		// Walk outwards from the sleep call to the innermost function.
		for i := len(stack) - 2; i >= 0; i-- {
			switch outer := stack[i].(type) {
			case *ast.ForStmt, *ast.RangeStmt:
				return true // polling/retry loop: stay silent
			case *ast.FuncDecl:
				if _, ok := testfuncs.TestFunc(pass.TypesInfo, outer); ok {
					pass.Reportf(call.Pos(), "%s", msg)
				}
				return true
			case *ast.FuncLit:
				if i > 0 {
					if parent, ok := stack[i-1].(*ast.CallExpr); ok {
						if isSynctestCall(pass, parent) {
							return true // synctest bubble uses a fake clock
						}
						if _, _, ok := testfuncs.SubtestLit(pass.TypesInfo, parent); ok {
							pass.Reportf(call.Pos(), "%s", msg)
						}
					}
				}
				return true // goroutine/callback literal: not the test body
			}
		}
		return true
	})
	return nil, nil
}

func isZeroConst(pass *analysis.Pass, arg ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[arg]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.Int {
		return false
	}
	v, ok := constant.Int64Val(tv.Value)
	return ok && v == 0
}

func isSynctestCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	return testfuncs.IsPkgFunc(pass.TypesInfo, call, "testing/synctest", "Run") ||
		testfuncs.IsPkgFunc(pass.TypesInfo, call, "testing/synctest", "Test")
}
