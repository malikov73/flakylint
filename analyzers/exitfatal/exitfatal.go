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
		for i := len(stack) - 2; i >= 0; i-- {
			switch outer := stack[i].(type) {
			case *ast.FuncDecl:
				if param, ok := testfuncs.TestFunc(pass.TypesInfo, outer); ok {
					reportInTest(pass, call, isExit, logName, param.Name)
				} else if testfuncs.IsTestMain(pass.TypesInfo, outer) && hasDirectDefer(outer.Body) {
					callName := "os.Exit"
					if !isExit {
						callName = "log." + logName
					}
					pass.Reportf(call.Pos(),
						"%s in TestMain skips the function's pending defers; run cleanup before exiting or move it into m.Run setup/teardown", callName)
				}
				return true
			case *ast.FuncLit:
				if i > 0 {
					if parent, ok := stack[i-1].(*ast.CallExpr); ok {
						if _, param, ok := testfuncs.SubtestLit(pass.TypesInfo, parent); ok {
							reportInTest(pass, call, isExit, logName, param.Name)
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

func reportInTest(pass *analysis.Pass, call *ast.CallExpr, isExit bool, logName, tname string) {
	// tname is the receiver we point users at; "_" (blank param) has no name
	// to reference, so fall back to the conventional "t".
	recv := tname
	if recv == "_" {
		recv = "t"
	}
	if isExit {
		pass.Reportf(call.Pos(),
			"os.Exit inside a test terminates the whole test binary and skips cleanup; use %s.Fatal or %s.Skip", recv, recv)
		return
	}
	diag := analysis.Diagnostic{
		Pos: call.Pos(),
		End: call.End(),
		Message: fmt.Sprintf(
			"log.%s inside a test terminates the whole test binary and skips cleanup; use %s.%s",
			logName, recv, fatalFuncs[logName]),
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
