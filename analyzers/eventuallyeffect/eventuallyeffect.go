// Package eventuallyeffect reports side effects inside testify Eventually-style
// polling callbacks.
//
// testify's assert/require Eventually, Eventuallyf, Never, Neverf, and
// EventuallyWithT run their condition callback repeatedly until it passes (or
// the timeout elapses), so the callback executes a nondeterministic number of
// times that depends on machine speed and scheduling. A callback that mutates
// state shared with the test — writing a captured variable or sending on a
// channel — therefore makes the test's outcome depend on the poll count, which
// is exactly the kind of hidden timing dependency that flakes. The condition
// should be pure: perform the side effect once before polling, then poll on a
// read-only predicate, or assert on captured results after Eventually returns.
//
// This is a Go port of the idea behind testing-library's
// no-wait-for-side-effects rule.
//
// v1 is deliberately narrow. It flags only count-dependent effects inside the
// callback — those whose result changes with the number of poll ticks:
//
//   - an increment or decrement of a variable declared outside the callback (a
//     captured local or a package-level var): x++, x--;
//   - a compound assignment to such a variable: x += ..., x |= ..., etc.;
//   - a self-append that grows such a variable: x = append(x, ...);
//   - a channel send: ch <- v.
//
// A plain overwrite (x = v, including multi-assign a, b = f()) is silent. It is
// last-write-wins: re-running it each tick leaves the same final value, so it
// is the idiomatic way to capture the result of the final successful tick and
// does not make the outcome depend on the poll count. Flagging it produced
// false positives on real code (prometheus/prometheus), so plain overwrites are
// a deliberate boundary.
//
// Writes through a captured pointer, map, or slice element (p.f = v, m[k] = v,
// s[0] = v) stay silent: keyed or idempotent writes into a shared cache are a
// common and legitimate polling pattern, so flagging them would cost more false
// positives than it is worth. Method calls (HTTP requests, DB writes, mutex
// operations, t.Log) are out of scope in v1. Only package-level testify calls
// (assert.Eventually(t, ...)) in _test.go files are examined.
package eventuallyeffect

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
	Name:     "eventuallyeffect",
	Doc:      "reports count-dependent side effects (increments, compound assignments, self-appends, channel sends) inside testify Eventually-style polling callbacks; the callback runs an unpredictable number of times, so such effects make tests flake",
	URL:      "https://github.com/malikov73/flakylint",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

const (
	writeMsg = "the Eventually condition runs an unpredictable number of times; this write to %q makes test state depend on the poll count — capture results with a local variable inside the callback or assert after Eventually returns"
	sendMsg  = "the Eventually condition runs an unpredictable number of times; this channel send happens once per poll tick — move it out of the callback"
)

var testifyPkgs = []string{
	"github.com/stretchr/testify/assert",
	"github.com/stretchr/testify/require",
}

// pollFuncs are the testify functions whose second argument is a condition
// callback that runs an unpredictable number of times.
var pollFuncs = []string{
	"Eventually", "Eventuallyf",
	"Never", "Neverf",
	"EventuallyWithT", "EventuallyWithTf",
}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		if !testfuncs.InTestFile(pass, call) {
			return
		}
		lit, ok := conditionLit(pass.TypesInfo, call)
		if !ok {
			return
		}
		checkCondition(pass, lit)
	})
	return nil, nil
}

// conditionLit returns the callback literal of a testify poll call. It matches
// only the package-level testify form (assert.Eventually(t, func() ...)); the
// require.New(t) method form has a receiver and is skipped. A non-literal
// condition (a named function) cannot be analyzed and is skipped too.
func conditionLit(info *types.Info, call *ast.CallExpr) (*ast.FuncLit, bool) {
	if len(call.Args) < 2 {
		return nil, false
	}
	if !isPollCall(info, call) {
		return nil, false
	}
	lit, ok := call.Args[1].(*ast.FuncLit)
	return lit, ok
}

func isPollCall(info *types.Info, call *ast.CallExpr) bool {
	for _, pkg := range testifyPkgs {
		for _, name := range pollFuncs {
			if testfuncs.IsPkgFunc(info, call, pkg, name) {
				return true
			}
		}
	}
	return false
}

// checkCondition reports count-dependent writes and channel sends anywhere
// inside the callback body, including nested function literals (a goroutine
// spawned per tick is just as nondeterministic).
func checkCondition(pass *analysis.Pass, lit *ast.FuncLit) {
	ast.Inspect(lit.Body, func(n ast.Node) bool {
		switch st := n.(type) {
		case *ast.SendStmt:
			pass.Reportf(st.Pos(), sendMsg)
		case *ast.IncDecStmt:
			reportCapturedWrite(pass, lit, st.X)
		case *ast.AssignStmt:
			checkAssign(pass, lit, st)
		}
		return true
	})
}

// checkAssign reports only count-dependent assignments to captured variables.
// A compound assignment (x += ..., x |= ...) accumulates across ticks; a
// self-append (x = append(x, ...)) grows the target by one element per tick.
// A plain overwrite (x = v, a, b = f()) is last-write-wins and stays silent —
// the deliberate boundary documented on the package. A := declares a fresh
// local inside the callback and never writes captured state.
func checkAssign(pass *analysis.Pass, lit *ast.FuncLit, st *ast.AssignStmt) {
	switch {
	case st.Tok == token.DEFINE:
		return
	case st.Tok != token.ASSIGN:
		for _, lhs := range st.Lhs {
			reportCapturedWrite(pass, lit, lhs)
		}
	default:
		for i, lhs := range st.Lhs {
			if i < len(st.Rhs) && isSelfAppend(pass.TypesInfo, lhs, st.Rhs[i]) {
				reportCapturedWrite(pass, lit, lhs)
			}
		}
	}
}

// isSelfAppend reports whether rhs is a builtin append(...) that includes lhs
// among its arguments, i.e. the "grow lhs by appending to it" idiom. The lhs
// ident may appear as any argument, including a variadic spread (append(x, xs...)
// or append(prefix, x...)), so all arguments are checked.
func isSelfAppend(info *types.Info, lhs, rhs ast.Expr) bool {
	id, ok := lhs.(*ast.Ident)
	if !ok {
		return false
	}
	call, ok := rhs.(*ast.CallExpr)
	if !ok {
		return false
	}
	fn, ok := call.Fun.(*ast.Ident)
	if !ok || fn.Name != "append" {
		return false
	}
	if _, ok := info.ObjectOf(fn).(*types.Builtin); !ok {
		return false // append shadowed by a local of the same name
	}
	target := info.ObjectOf(id)
	if target == nil {
		return false
	}
	for _, arg := range call.Args {
		if argID, ok := arg.(*ast.Ident); ok && info.ObjectOf(argID) == target {
			return true
		}
	}
	return false
}

// reportCapturedWrite flags target when it is a direct write to a variable
// declared outside the callback. Indexed, field, and dereference writes, the
// blank identifier, and locals declared inside the callback are all silent.
func reportCapturedWrite(pass *analysis.Pass, lit *ast.FuncLit, target ast.Expr) {
	id, ok := target.(*ast.Ident)
	if !ok {
		return // indexed/field/deref write: documented v1 boundary
	}
	v, ok := pass.TypesInfo.ObjectOf(id).(*types.Var)
	if !ok {
		return // blank identifier or non-variable
	}
	if declaredInside(lit, v) {
		return
	}
	pass.Reportf(id.Pos(), writeMsg, v.Name())
}

// declaredInside reports whether v is declared within the callback literal,
// resolved by declaration position rather than syntax. Captured locals and
// package-level vars fall outside the literal's range and are flaggable.
func declaredInside(lit *ast.FuncLit, v *types.Var) bool {
	return v.Pos() >= lit.Pos() && v.Pos() < lit.End()
}
