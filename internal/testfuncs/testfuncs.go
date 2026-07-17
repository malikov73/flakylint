// Package testfuncs provides shared helpers for recognizing Go test
// functions, subtests, and calls in _test.go files.
package testfuncs

import (
	"go/ast"
	"go/types"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"
)

// InTestFile reports whether the node belongs to a _test.go file.
func InTestFile(pass *analysis.Pass, n ast.Node) bool {
	f := pass.Fset.File(n.Pos())
	return f != nil && strings.HasSuffix(f.Name(), "_test.go")
}

// TestFunc reports whether fn is a Test, Benchmark, or Fuzz function and
// returns the identifier of its *testing.T/B/F parameter.
// TestMain is not a test function.
func TestFunc(info *types.Info, fn *ast.FuncDecl) (*ast.Ident, bool) {
	if fn.Recv != nil || fn.Name == nil || fn.Body == nil || fn.Name.Name == "TestMain" {
		return nil, false
	}
	if !matchesPrefix(fn.Name.Name, "Test") &&
		!matchesPrefix(fn.Name.Name, "Benchmark") &&
		!matchesPrefix(fn.Name.Name, "Fuzz") {
		return nil, false
	}
	params := fn.Type.Params
	if params == nil || len(params.List) != 1 || len(params.List[0].Names) != 1 {
		return nil, false
	}
	if !isTestingPtr(info, params.List[0].Type, "T", "B", "F") {
		return nil, false
	}
	return params.List[0].Names[0], true
}

// IsTestMain reports whether fn is func TestMain(m *testing.M).
func IsTestMain(info *types.Info, fn *ast.FuncDecl) bool {
	if fn.Recv != nil || fn.Name == nil || fn.Name.Name != "TestMain" {
		return false
	}
	params := fn.Type.Params
	if params == nil || len(params.List) != 1 {
		return false
	}
	return isTestingPtr(info, params.List[0].Type, "M")
}

// SubtestLit returns the function literal and its testing parameter for
// calls of the form t.Run("name", func(t *testing.T) { ... }).
func SubtestLit(info *types.Info, call *ast.CallExpr) (*ast.FuncLit, *ast.Ident, bool) {
	if !IsTestingMethod(info, call, "Run") || len(call.Args) != 2 {
		return nil, nil, false
	}
	lit, ok := call.Args[1].(*ast.FuncLit)
	if !ok {
		return nil, nil, false
	}
	params := lit.Type.Params
	if params == nil || len(params.List) != 1 || len(params.List[0].Names) != 1 {
		return nil, nil, false
	}
	if !isTestingPtr(info, params.List[0].Type, "T", "B", "F") {
		return nil, nil, false
	}
	return lit, params.List[0].Names[0], true
}

// calleeFunc returns the function that call statically resolves to, or nil
// for dynamic/interface calls and conversions.
func calleeFunc(info *types.Info, call *ast.CallExpr) *types.Func {
	fn, _ := typeutil.Callee(info, call).(*types.Func)
	return fn
}

// IsPkgFunc reports whether call invokes the package-level function
// pkgPath.name (e.g. "os".Exit).
func IsPkgFunc(info *types.Info, call *ast.CallExpr, pkgPath, name string) bool {
	fn := calleeFunc(info, call)
	if fn == nil || fn.Name() != name || fn.Pkg() == nil || fn.Pkg().Path() != pkgPath {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	return ok && sig.Recv() == nil
}

// IsTestingMethod reports whether call invokes a method named name whose
// receiver type is declared in package "testing" (covers *testing.T,
// *testing.B, and methods promoted from the embedded testing.common).
func IsTestingMethod(info *types.Info, call *ast.CallExpr, name string) bool {
	fn := calleeFunc(info, call)
	if fn == nil || fn.Name() != name || fn.Pkg() == nil || fn.Pkg().Path() != "testing" {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	return ok && sig.Recv() != nil
}

func isTestingPtr(info *types.Info, expr ast.Expr, names ...string) bool {
	ptr, ok := info.TypeOf(expr).(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := ptr.Elem().(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	if obj.Pkg() == nil || obj.Pkg().Path() != "testing" {
		return false
	}
	for _, n := range names {
		if obj.Name() == n {
			return true
		}
	}
	return false
}

func matchesPrefix(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	rest := name[len(prefix):]
	if rest == "" {
		return true
	}
	r, _ := utf8.DecodeRuneInString(rest)
	return !unicode.IsLower(r)
}
