// Package exitfatal reports os.Exit and log.Fatal calls in tests.
//
// Both terminate the whole test binary immediately: t.Cleanup callbacks and
// deferred teardown (containers, temp dirs, servers) never run, poisoning
// subsequent tests. In TestMain, os.Exit additionally skips the function's
// own pending defers.
package exitfatal

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/malikov73/flakylint/internal/testfuncs"
)

var Analyzer = &analysis.Analyzer{
	Name:     "exitfatal",
	Doc:      "reports os.Exit and log.Fatal in tests; they kill the test binary and skip cleanup, poisoning later tests",
	URL:      "https://github.com/malikov73/flakylint",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// fatalFuncs maps log package fatal functions to their testing.T replacement.
var fatalFuncs = map[string]string{
	"Fatal":   "Fatal",
	"Fatalf":  "Fatalf",
	"Fatalln": "Fatal",
}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// The pending-defer check is identical for every exit call inside a given
	// TestMain, so compute it once per function rather than on each call below.
	testMainDefer := map[*ast.FuncDecl]bool{}
	for _, f := range pass.Files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !testfuncs.IsTestMain(pass.TypesInfo, fn) {
				continue
			}
			testMainDefer[fn] = hasDirectDefer(fn.Body)
		}
	}

	insp.WithStack([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return false
		}
		call := n.(*ast.CallExpr)
		if !testfuncs.InTestFile(pass, call) {
			return false
		}
		isExit := testfuncs.IsPkgFunc(pass.TypesInfo, call, "os", "Exit")
		logName := ""
		if !isExit {
			for name := range fatalFuncs {
				if testfuncs.IsPkgFunc(pass.TypesInfo, call, "log", name) {
					logName = name
					break
				}
			}
			if logName == "" {
				return true
			}
		}
		// reportInTest flags a call inside a test body or subtest. When the
		// receiver name no longer resolves to the testing parameter at the call
		// site (e.g. a local `t := 1` shadows it), pointing the fix at t.Fatal
		// would rewrite the call against the wrong object, so drop the fix and
		// use wording that does not name the shadowed receiver.
		reportInTest := func(param *ast.Ident) {
			tname := param.Name
			shadowed := tname != "_" &&
				!resolvesToParam(pass, tname, pass.TypesInfo.Defs[param], call.Pos())
			if isExit {
				reportExit(pass, call, tname, shadowed)
			} else {
				reportLogFatal(pass, call, logName, tname, shadowed)
			}
		}
		for i := len(stack) - 2; i >= 0; i-- {
			switch outer := stack[i].(type) {
			case *ast.FuncDecl:
				if param, ok := testfuncs.TestFunc(pass.TypesInfo, outer); ok {
					reportInTest(param)
				} else if testMainDefer[outer] {
					reportTestMain(pass, call, isExit, logName)
				}
				return true
			case *ast.FuncLit:
				if i > 0 {
					if parent, ok := stack[i-1].(*ast.CallExpr); ok {
						if _, param, ok := testfuncs.SubtestLit(pass.TypesInfo, parent); ok {
							reportInTest(param)
						}
					}
				}
				return true // goroutine/callback literal: conservative, silent
			}
		}
		return true
	})
	return nil, nil
}

// receiver is the method receiver to point users at; the blank param "_" has
// no name to reference, so fall back to the conventional "t".
func receiver(tname string) string {
	if tname == "_" {
		return "t"
	}
	return tname
}

// resolvesToParam reports whether name still resolves to the testing parameter
// param at pos. It returns false when a nearer declaration shadows the
// parameter, so callers can suppress a fix that would target the wrong object.
func resolvesToParam(pass *analysis.Pass, name string, param types.Object, pos token.Pos) bool {
	if param == nil {
		return false
	}
	inner := pass.Pkg.Scope().Innermost(pos)
	if inner == nil {
		return false
	}
	_, obj := inner.LookupParent(name, pos)
	return obj == param
}

// reportExit flags an os.Exit call inside a test body or subtest.
func reportExit(pass *analysis.Pass, call *ast.CallExpr, tname string, shadowed bool) {
	if shadowed {
		pass.Reportf(call.Pos(),
			"os.Exit inside a test terminates the whole test binary and skips cleanup; route the failure through the test's *testing.T")
		return
	}
	recv := receiver(tname)
	pass.Reportf(call.Pos(),
		"os.Exit inside a test terminates the whole test binary and skips cleanup; use %s.Fatal or %s.Skip", recv, recv)
}

// reportLogFatal flags a log.Fatal* call inside a test body or subtest and,
// when the receiver has a usable name and is not shadowed, offers to rewrite
// it to the t.Fatal* form.
func reportLogFatal(pass *analysis.Pass, call *ast.CallExpr, logName, tname string, shadowed bool) {
	if shadowed {
		pass.Reportf(call.Pos(),
			"log.%s inside a test terminates the whole test binary and skips cleanup; route the failure through the test's *testing.T",
			logName)
		return
	}
	diag := analysis.Diagnostic{
		Pos: call.Pos(),
		End: call.End(),
		Message: fmt.Sprintf(
			"log.%s inside a test terminates the whole test binary and skips cleanup; use %s.%s",
			logName, receiver(tname), fatalFuncs[logName]),
	}
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok && tname != "_" {
		diag.SuggestedFixes = []analysis.SuggestedFix{{
			Message: fmt.Sprintf("replace log.%s with %s.%s", logName, tname, fatalFuncs[logName]),
			TextEdits: []analysis.TextEdit{{
				Pos:     sel.Pos(),
				End:     sel.End(),
				NewText: []byte(tname + "." + fatalFuncs[logName]),
			}},
		}}
	}
	pass.Report(diag)
}

// reportTestMain flags an exit call that skips TestMain's own pending defers.
func reportTestMain(pass *analysis.Pass, call *ast.CallExpr, isExit bool, logName string) {
	callName := "os.Exit"
	if !isExit {
		callName = "log." + logName
	}
	pass.Reportf(call.Pos(),
		"%s in TestMain skips the function's pending defers; run cleanup before exiting or move it into m.Run setup/teardown", callName)
}

// hasDirectDefer reports whether body contains a defer statement outside
// nested function literals (those defers are the ones os.Exit skips).
func hasDirectDefer(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.DeferStmt:
			found = true
		}
		return true
	})
	return found
}
