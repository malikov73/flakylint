package testfuncs_test

import (
	"go/ast"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/malikov73/flakylint/internal/testfuncs"
)

// probe reports what the helpers detect so analysistest can assert on it.
var probe = &analysis.Analyzer{
	Name: "probe",
	Doc:  "reports testfuncs helper detections, for testing only",
	Run: func(pass *analysis.Pass) (any, error) {
		for _, f := range pass.Files {
			if !testfuncs.InTestFile(pass, f) {
				continue
			}
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.FuncDecl:
					if param, ok := testfuncs.TestFunc(pass.TypesInfo, n); ok {
						pass.Reportf(n.Pos(), "testfunc %s", param.Name)
					}
					if testfuncs.IsTestMain(pass.TypesInfo, n) {
						pass.Reportf(n.Pos(), "testmain")
					}
				case *ast.CallExpr:
					if _, param, ok := testfuncs.SubtestLit(pass.TypesInfo, n); ok {
						pass.Reportf(n.Pos(), "subtest %s", param.Name)
					}
					if testfuncs.IsPkgFunc(pass.TypesInfo, n, "os", "Exit") {
						pass.Reportf(n.Pos(), "osexit")
					}
					if testfuncs.IsTestingMethod(pass.TypesInfo, n, "Parallel") {
						pass.Reportf(n.Pos(), "parallel")
					}
				}
				return true
			})
		}
		return nil, nil
	},
}

func TestHelpers(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), probe, "p")
}
