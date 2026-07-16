# flakylint MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship flakylint v0.1.0 — a `go/analysis` multichecker with 4 conservative analyzers that statically detect flaky-test patterns in Go `_test.go` files.

**Architecture:** Each analyzer is a standalone package exporting `var Analyzer *analysis.Analyzer` (usable via `singlechecker`/`go vet -vettool`, required for future golangci-lint integration). Shared AST/type helpers live in `internal/testfuncs`. `cmd/flakylint` wires everything through `multichecker.Main`. All traversal uses the standard `passes/inspect` inspector.

**Tech Stack:** Go 1.25, `golang.org/x/tools` (only dependency), `analysistest` for tests, GitHub Actions CI, goreleaser.

## Global Constraints

- Module path: `github.com/malikov73/flakylint`. License: MIT.
- `go.mod` says `go 1.25` (spec said 1.24+; tightened to 1.25 because sleepassert testdata uses the stable `synctest.Test` API).
- The only third-party dependency is `golang.org/x/tools`. Adding anything else is a plan violation.
- All analyzers report **only** in `_test.go` files (filename check via `testfuncs.InTestFile`).
- Conservative philosophy: when in doubt, stay silent. Every anti-false-positive heuristic in the spec must have a "no diagnostic" test case.
- Diagnostics, code comments, README: English. Commit messages: English, conventional commits, **no AI attribution / Co-Authored-By lines**.
- Repo root: `/Users/asman/Developer/personal/contributions/flakylint` (git repo already initialized, spec committed).

---

### Task 1: Scaffolding + `internal/testfuncs` helpers

**Files:**
- Create: `go.mod` (via `go mod init`), `LICENSE`, `.gitignore`
- Create: `internal/testfuncs/testfuncs.go`
- Test: `internal/testfuncs/testfuncs_test.go`, `internal/testfuncs/testdata/src/p/p_test.go`

**Interfaces:**
- Consumes: nothing (first task).
- Produces (used by every analyzer task):
  - `func InTestFile(pass *analysis.Pass, n ast.Node) bool`
  - `func TestFunc(info *types.Info, fn *ast.FuncDecl) (*ast.Ident, bool)` — testing param ident + ok; matches `TestXxx(*testing.T)`, `BenchmarkXxx(*testing.B)`, `FuzzXxx(*testing.F)`; returns false for `TestMain` and lowercase-after-prefix names.
  - `func IsTestMain(info *types.Info, fn *ast.FuncDecl) bool`
  - `func SubtestLit(info *types.Info, call *ast.CallExpr) (*ast.FuncLit, *ast.Ident, bool)` — detects `t.Run("...", func(t *testing.T) {...})`.
  - `func IsPkgFunc(info *types.Info, call *ast.CallExpr, pkgPath, name string) bool`
  - `func IsTestingMethod(info *types.Info, call *ast.CallExpr, name string) bool` — method `name` whose receiver is declared in package `testing`.

- [ ] **Step 1: Initialize module and repo files**

```bash
cd /Users/asman/Developer/personal/contributions/flakylint
go mod init github.com/malikov73/flakylint
go get golang.org/x/tools@latest
```

Create `.gitignore`:

```
dist/
bin/
```

Create `LICENSE` with the standard MIT license text, copyright line: `Copyright (c) 2026 Asman Malikov`.

- [ ] **Step 2: Write the failing test (probe analyzer + testdata)**

`internal/testfuncs/testfuncs_test.go`:

```go
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
```

`internal/testfuncs/testdata/src/p/p_test.go`:

```go
package p

import (
	"os"
	"testing"
)

func TestSimple(t *testing.T) { // want `testfunc t`
	t.Parallel() // want `parallel`
	t.Run("sub", func(t *testing.T) { // want `subtest t`
	})
}

func BenchmarkB(b *testing.B) { // want `testfunc b`
}

func FuzzF(f *testing.F) { // want `testfunc f`
}

func TestMain(m *testing.M) { // want `testmain`
	os.Exit(m.Run()) // want `osexit`
}

// Not tests: lowercase after prefix, helper signature, no param.
func Testhelper(t *testing.T) {
	_ = t
}

func helper(t *testing.T) {
	_ = t
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/testfuncs/`
Expected: FAIL — compile error, package `testfuncs` does not exist.

- [ ] **Step 4: Implement `internal/testfuncs/testfuncs.go`**

```go
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

// IsPkgFunc reports whether call invokes the package-level function
// pkgPath.name (e.g. "os".Exit).
func IsPkgFunc(info *types.Info, call *ast.CallExpr, pkgPath, name string) bool {
	fn, _ := typeutil.Callee(info, call).(*types.Func)
	if fn == nil {
		return false
	}
	if fn.Name() != name || fn.Pkg() == nil || fn.Pkg().Path() != pkgPath {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	return ok && sig.Recv() == nil
}

// IsTestingMethod reports whether call invokes a method named name whose
// receiver type is declared in package "testing" (covers *testing.T,
// *testing.B, and methods promoted from the embedded testing.common).
func IsTestingMethod(info *types.Info, call *ast.CallExpr, name string) bool {
	fn, _ := typeutil.Callee(info, call).(*types.Func)
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
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/testfuncs/`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum LICENSE .gitignore internal/
git commit -m "feat: scaffold module and shared test-function helpers"
```

---

### Task 2: `httptestclose` analyzer

**Files:**
- Create: `analyzers/httptestclose/httptestclose.go`
- Test: `analyzers/httptestclose/httptestclose_test.go`, `analyzers/httptestclose/testdata/src/a/a_test.go`, `analyzers/httptestclose/testdata/src/fix/fix_test.go`, `analyzers/httptestclose/testdata/src/fix/fix_test.go.golden`

**Interfaces:**
- Consumes: everything listed in Task 1 "Produces".
- Produces: `httptestclose.Analyzer *analysis.Analyzer` (name `"httptestclose"`), consumed by Task 6.

Detection rule (from spec): a `srv := httptest.NewServer/NewTLSServer/NewUnstartedServer(...)` define in a `_test.go` file is reported when, within the enclosing function body, (a) there is no `srv.Close` selector reference (covers `srv.Close()`, `defer srv.Close()`, `t.Cleanup(srv.Close)`), and (b) the variable never escapes — every use of `srv` is a selector base (`srv.URL`, `srv.Client()`, ...). Any bare identifier use (call argument, return, RHS of assignment, composite literal, `&srv`) counts as escape → silent.

- [ ] **Step 1: Write the failing test**

`analyzers/httptestclose/httptestclose_test.go`:

```go
package httptestclose_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/malikov73/flakylint/analyzers/httptestclose"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), httptestclose.Analyzer, "a")
}

func TestSuggestedFix(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), httptestclose.Analyzer, "fix")
}
```

`analyzers/httptestclose/testdata/src/a/a_test.go`:

```go
package a

import (
	"net/http/httptest"
	"testing"
)

func use(*httptest.Server) {}

func TestLeak(t *testing.T) {
	srv := httptest.NewServer(nil) // want `httptest server is never closed`
	_ = srv.URL
}

func TestTLSLeak(t *testing.T) {
	srv := httptest.NewTLSServer(nil) // want `httptest server is never closed`
	_ = srv.URL
}

func TestUnstartedLeak(t *testing.T) {
	srv := httptest.NewUnstartedServer(nil) // want `httptest server is never closed`
	srv.Start()
}

func TestCleanup(t *testing.T) {
	srv := httptest.NewServer(nil)
	t.Cleanup(srv.Close)
	_ = srv.URL
}

func TestDefer(t *testing.T) {
	srv := httptest.NewServer(nil)
	defer srv.Close()
	_ = srv.URL
}

func TestDirectClose(t *testing.T) {
	srv := httptest.NewServer(nil)
	srv.Close()
}

func TestEscapeArg(t *testing.T) {
	srv := httptest.NewServer(nil) // escapes as argument: silent
	use(srv)
}

func newServer(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(nil) // escapes via return: silent
	return srv
}

func TestEscapeAssign(t *testing.T) {
	var keep *httptest.Server
	srv := httptest.NewServer(nil) // escapes via reassignment: silent
	keep = srv
	_ = keep
}

func TestSubtestLeak(t *testing.T) {
	t.Run("sub", func(t *testing.T) {
		srv := httptest.NewServer(nil) // want `httptest server is never closed`
		_ = srv.URL
	})
}
```

`analyzers/httptestclose/testdata/src/fix/fix_test.go`:

```go
package fix

import (
	"net/http/httptest"
	"testing"
)

func TestFix(t *testing.T) {
	srv := httptest.NewServer(nil) // want `httptest server is never closed`
	_ = srv.URL
}
```

`analyzers/httptestclose/testdata/src/fix/fix_test.go.golden` (the fix inserts a new line at the end of the assignment expression, so the trailing comment moves to the inserted line):

```go
package fix

import (
	"net/http/httptest"
	"testing"
)

func TestFix(t *testing.T) {
	srv := httptest.NewServer(nil)
	t.Cleanup(srv.Close) // want `httptest server is never closed`
	_ = srv.URL
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./analyzers/httptestclose/`
Expected: FAIL — package `httptestclose` does not exist.

- [ ] **Step 3: Implement the analyzer**

`analyzers/httptestclose/httptestclose.go`:

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./analyzers/httptestclose/`
Expected: PASS (both `TestAnalyzer` and `TestSuggestedFix`). If the golden file mismatches, inspect the reported diff and align the golden file with the actual edit — the intent is `t.Cleanup(srv.Close)` inserted directly after the assignment expression.

- [ ] **Step 5: Commit**

```bash
git add analyzers/httptestclose/
git commit -m "feat: add httptestclose analyzer"
```

---

### Task 3: `sleepassert` analyzer

**Files:**
- Create: `analyzers/sleepassert/sleepassert.go`
- Test: `analyzers/sleepassert/sleepassert_test.go`, `analyzers/sleepassert/testdata/src/a/a_test.go`

**Interfaces:**
- Consumes: Task 1 helpers.
- Produces: `sleepassert.Analyzer *analysis.Analyzer` (name `"sleepassert"`), consumed by Task 6.

Detection rule (from spec): report `time.Sleep(d)` whose innermost enclosing function is a test function or a subtest literal. Stay silent when: the sleep is inside a `for`/`range` statement between it and the enclosing function (polling/retry); the enclosing literal is an argument to `synctest.Run`/`synctest.Test`; `d` is the constant 0; the sleep is in any other function literal (goroutine, helper) or in a non-test function.

- [ ] **Step 1: Write the failing test**

`analyzers/sleepassert/sleepassert_test.go`:

```go
package sleepassert_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/malikov73/flakylint/analyzers/sleepassert"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), sleepassert.Analyzer, "a")
}
```

`analyzers/sleepassert/testdata/src/a/a_test.go`:

```go
package a

import (
	"testing"
	"testing/synctest"
	"time"
)

func work()                {}
func check(t *testing.T)   { t.Helper() }

func TestSleep(t *testing.T) {
	go work()
	time.Sleep(50 * time.Millisecond) // want `time.Sleep synchronizes the test on real time`
	check(t)
}

func TestSubtestSleep(t *testing.T) {
	t.Run("sub", func(t *testing.T) {
		time.Sleep(time.Second) // want `time.Sleep synchronizes the test on real time`
	})
}

func TestPollingLoop(t *testing.T) {
	for i := 0; i < 10; i++ {
		time.Sleep(10 * time.Millisecond) // polling loop: silent
	}
}

func TestRangeLoop(t *testing.T) {
	for range 3 {
		time.Sleep(time.Millisecond) // polling loop: silent
	}
}

func TestZeroSleep(t *testing.T) {
	time.Sleep(0) // zero duration: silent
}

func TestInsideSynctest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		time.Sleep(time.Second) // inside synctest bubble: silent
	})
}

func TestGoroutineSleep(t *testing.T) {
	go func() {
		time.Sleep(time.Millisecond) // helper literal, not the test body: silent
	}()
}

func helperSleep() {
	time.Sleep(time.Millisecond) // not a test function: silent
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./analyzers/sleepassert/`
Expected: FAIL — package `sleepassert` does not exist.

- [ ] **Step 3: Implement the analyzer**

`analyzers/sleepassert/sleepassert.go`:

```go
// Package sleepassert reports time.Sleep calls in test bodies.
//
// Synchronizing a test on real time races the goroutine scheduler and the
// CI machine's load; such tests pass locally and flake under CI pressure.
// Prefer testing/synctest (Go 1.24+) or explicit synchronization.
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

const msg = "time.Sleep synchronizes the test on real time and flakes under CI load; use testing/synctest (Go 1.24+) or explicit synchronization (channel, sync.WaitGroup)"

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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./analyzers/sleepassert/`
Expected: PASS. (If the toolchain rejects `testing/synctest`, the local Go is older than 1.25 — install Go 1.25+; this is the version floor from Global Constraints.)

- [ ] **Step 5: Commit**

```bash
git add analyzers/sleepassert/
git commit -m "feat: add sleepassert analyzer"
```

---

### Task 4: `parallelglobal` analyzer

**Files:**
- Create: `analyzers/parallelglobal/parallelglobal.go`
- Test: `analyzers/parallelglobal/parallelglobal_test.go`, `analyzers/parallelglobal/testdata/src/a/a_test.go`, `analyzers/parallelglobal/testdata/src/b/b.go`

**Interfaces:**
- Consumes: Task 1 helpers.
- Produces: `parallelglobal.Analyzer *analysis.Analyzer` (name `"parallelglobal"`), consumed by Task 6.

Detection rule (from spec): a **unit** is a top-level test function or a subtest literal. A unit is *parallel* if its body contains a `Parallel()` call on a testing type, excluding nested subtest literals (each subtest is its own unit). In a parallel unit, report every write to a package-level variable: plain/compound assignment, `x.f = ...`, `x[i] = ...`, `x++/x--` — again excluding nested subtest literals. Known false negative (documented): a write inside a non-parallel subtest of a parallel parent is not reported. Mutex protection is deliberately ignored.

- [ ] **Step 1: Write the failing test**

`analyzers/parallelglobal/parallelglobal_test.go`:

```go
package parallelglobal_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/malikov73/flakylint/analyzers/parallelglobal"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), parallelglobal.Analyzer, "a")
}
```

`analyzers/parallelglobal/testdata/src/b/b.go`:

```go
package b

var Counter int
```

`analyzers/parallelglobal/testdata/src/a/a_test.go`:

```go
package a

import (
	"sync"
	"testing"

	"b"
)

var (
	counter int
	state   = map[string]int{}
	mu      sync.Mutex
)

func TestParallelWrite(t *testing.T) {
	t.Parallel()
	counter = 1 // want `parallel test writes package-level variable "counter"`
}

func TestParallelCompound(t *testing.T) {
	t.Parallel()
	counter += 1   // want `parallel test writes package-level variable "counter"`
	counter++      // want `parallel test writes package-level variable "counter"`
	state["k"] = 1 // want `parallel test writes package-level variable "state"`
	b.Counter = 2  // want `parallel test writes package-level variable "Counter"`
}

func TestSequentialWrite(t *testing.T) {
	counter = 1 // not parallel: silent
}

func TestParallelLocal(t *testing.T) {
	t.Parallel()
	local := 0
	local++
	_ = local
}

func TestParallelSubtest(t *testing.T) {
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		counter = 3 // want `parallel test writes package-level variable "counter"`
	})
}

func TestParallelParentSequentialChild(t *testing.T) {
	t.Parallel()
	t.Run("sub", func(t *testing.T) {
		counter = 4 // documented false negative: subtest unit is not itself parallel
	})
}

func TestMutexStillFlagged(t *testing.T) {
	t.Parallel()
	mu.Lock()
	counter = 5 // want `parallel test writes package-level variable "counter"`
	mu.Unlock()
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./analyzers/parallelglobal/`
Expected: FAIL — package `parallelglobal` does not exist.

- [ ] **Step 3: Implement the analyzer**

`analyzers/parallelglobal/parallelglobal.go`:

```go
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
				reportGlobalWrite(pass, lhs)
			}
		case *ast.IncDecStmt:
			reportGlobalWrite(pass, st.X)
		}
		return true
	})
}

func reportGlobalWrite(pass *analysis.Pass, target ast.Expr) {
	obj := targetObj(pass.TypesInfo, target)
	if obj == nil || !isGlobalVar(obj) {
		return
	}
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./analyzers/parallelglobal/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add analyzers/parallelglobal/
git commit -m "feat: add parallelglobal analyzer"
```

---

### Task 5: `exitfatal` analyzer

**Files:**
- Create: `analyzers/exitfatal/exitfatal.go`
- Test: `analyzers/exitfatal/exitfatal_test.go`, `analyzers/exitfatal/testdata/src/a/a_test.go`, `analyzers/exitfatal/testdata/src/tmdefer/tm_test.go`, `analyzers/exitfatal/testdata/src/tmclean/tm_test.go`, `analyzers/exitfatal/testdata/src/fixf/fixf_test.go`, `analyzers/exitfatal/testdata/src/fixf/fixf_test.go.golden`

**Interfaces:**
- Consumes: Task 1 helpers.
- Produces: `exitfatal.Analyzer *analysis.Analyzer` (name `"exitfatal"`), consumed by Task 6.

Detection rule (from spec): report `os.Exit` and `log.Fatal`/`Fatalf`/`Fatalln` (stdlib `log` package-level functions) whose innermost enclosing function is a test function or subtest literal. In `TestMain`, report them only when `TestMain`'s body contains a `defer` statement outside nested function literals (those defers are silently skipped). Suggested fix: `log.FatalX(...)` → `t.FatalX(...)` (`Fatalln` maps to `Fatal`).

- [ ] **Step 1: Write the failing test**

`analyzers/exitfatal/exitfatal_test.go`:

```go
package exitfatal_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/malikov73/flakylint/analyzers/exitfatal"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), exitfatal.Analyzer, "a", "tmdefer", "tmclean")
}

func TestSuggestedFix(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), exitfatal.Analyzer, "fixf")
}
```

`analyzers/exitfatal/testdata/src/a/a_test.go`:

```go
package a

import (
	"log"
	"os"
	"testing"
)

func TestExit(t *testing.T) {
	if os.Getenv("MUST_FAIL") != "" {
		os.Exit(1) // want `os.Exit inside a test terminates the whole test binary`
	}
}

func TestLogFatal(t *testing.T) {
	log.Fatal("boom") // want `log.Fatal inside a test terminates the whole test binary`
}

func TestLogFatalf(t *testing.T) {
	log.Fatalf("boom %d", 1) // want `log.Fatalf inside a test terminates the whole test binary`
}

func TestSubtestFatal(t *testing.T) {
	t.Run("sub", func(t *testing.T) {
		log.Fatalln("boom") // want `log.Fatalln inside a test terminates the whole test binary`
	})
}

func TestGoroutineFatal(t *testing.T) {
	go func() {
		log.Fatal("boom") // helper literal, not the test body: silent
	}()
}

func helper() {
	os.Exit(1) // not a test function: silent
}
```

`analyzers/exitfatal/testdata/src/tmdefer/tm_test.go`:

```go
package tmdefer

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	f, _ := os.CreateTemp("", "x")
	defer os.Remove(f.Name())
	os.Exit(m.Run()) // want `os.Exit in TestMain skips the function's pending defers`
}
```

`analyzers/exitfatal/testdata/src/tmclean/tm_test.go`:

```go
package tmclean

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run()) // canonical pattern, no defers: silent
}
```

`analyzers/exitfatal/testdata/src/fixf/fixf_test.go`:

```go
package fixf

import (
	"log"
	"testing"
)

func TestFix(t *testing.T) {
	log.Print("context")
	log.Fatalf("bad: %v", 1) // want `log.Fatalf inside a test terminates the whole test binary`
}
```

`analyzers/exitfatal/testdata/src/fixf/fixf_test.go.golden`:

```go
package fixf

import (
	"log"
	"testing"
)

func TestFix(t *testing.T) {
	log.Print("context")
	t.Fatalf("bad: %v", 1) // want `log.Fatalf inside a test terminates the whole test binary`
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./analyzers/exitfatal/`
Expected: FAIL — package `exitfatal` does not exist.

- [ ] **Step 3: Implement the analyzer**

`analyzers/exitfatal/exitfatal.go`:

```go
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
	if isExit {
		pass.Reportf(call.Pos(),
			"os.Exit inside a test terminates the whole test binary and skips cleanup; use t.Fatal or t.Skip")
		return
	}
	diag := analysis.Diagnostic{
		Pos: call.Pos(),
		End: call.End(),
		Message: fmt.Sprintf(
			"log.%s inside a test terminates the whole test binary and skips cleanup; use t.%s",
			logName, fatalFuncs[logName]),
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./analyzers/exitfatal/`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add analyzers/exitfatal/
git commit -m "feat: add exitfatal analyzer"
```

---

### Task 6: `cmd/flakylint` multichecker + smoke test

**Files:**
- Create: `cmd/flakylint/main.go`

**Interfaces:**
- Consumes: `httptestclose.Analyzer`, `sleepassert.Analyzer`, `parallelglobal.Analyzer`, `exitfatal.Analyzer`.
- Produces: the `flakylint` binary (entry point for goreleaser in Task 7 and for users).

- [ ] **Step 1: Write `cmd/flakylint/main.go`**

```go
// Command flakylint reports flaky-test patterns in Go test files.
package main

import (
	"golang.org/x/tools/go/analysis/multichecker"

	"github.com/malikov73/flakylint/analyzers/exitfatal"
	"github.com/malikov73/flakylint/analyzers/httptestclose"
	"github.com/malikov73/flakylint/analyzers/parallelglobal"
	"github.com/malikov73/flakylint/analyzers/sleepassert"
)

func main() {
	multichecker.Main(
		exitfatal.Analyzer,
		httptestclose.Analyzer,
		parallelglobal.Analyzer,
		sleepassert.Analyzer,
	)
}
```

- [ ] **Step 2: Self-lint must be clean**

Run: `go run ./cmd/flakylint ./...`
Expected: exit code 0, no output (our own test files contain none of the patterns; `testdata/` directories are ignored by the go tool).

- [ ] **Step 3: Smoke test on a deliberately flaky sample**

Create a scratch module (in the session scratchpad, not in the repo):

```bash
SCRATCH=$(mktemp -d)
mkdir -p "$SCRATCH/smoke"
cd "$SCRATCH/smoke"
cat > go.mod <<'EOF'
module smoke

go 1.25
EOF
cat > smoke_test.go <<'EOF'
package smoke

import (
	"log"
	"net/http/httptest"
	"testing"
	"time"
)

var counter int

func TestSmoke(t *testing.T) {
	t.Parallel()
	counter = 1
	srv := httptest.NewServer(nil)
	_ = srv.URL
	time.Sleep(10 * time.Millisecond)
	if counter == 0 {
		log.Fatal("boom")
	}
}
EOF
cd /Users/asman/Developer/personal/contributions/flakylint
go run ./cmd/flakylint "$SCRATCH/smoke/..." 2>&1 | sort
```

Expected: non-zero exit and four diagnostics, one from each analyzer (`parallelglobal` on `counter = 1`, `httptestclose` on the `srv := ...` line, `sleepassert` on `time.Sleep`, `exitfatal` on `log.Fatal`).

- [ ] **Step 4: Commit**

```bash
git add cmd/
git commit -m "feat: add flakylint multichecker command"
```

---

### Task 7: CI, release tooling, README

**Files:**
- Create: `.github/workflows/ci.yml`, `.github/workflows/release.yml`, `.golangci.yml`, `.goreleaser.yml`, `README.md`

**Interfaces:**
- Consumes: the whole repo (CI runs tests from Tasks 1–6; goreleaser builds `cmd/flakylint`).
- Produces: green CI on push, release automation on `v*` tags.

- [ ] **Step 1: Write `.github/workflows/ci.yml`**

```yaml
name: CI
on:
  push:
    branches: [main]
  pull_request:
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25.x"
      - name: gofmt
        run: test -z "$(gofmt -l .)"
      - name: vet
        run: go vet ./...
      - name: test
        run: go test ./...
      - name: self-lint
        run: go run ./cmd/flakylint ./...
  golangci:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25.x"
      - uses: golangci/golangci-lint-action@v7
```

- [ ] **Step 2: Write `.golangci.yml`**

```yaml
version: "2"
linters:
  default: standard
```

- [ ] **Step 3: Write `.goreleaser.yml` and `.github/workflows/release.yml`**

`.goreleaser.yml`:

```yaml
version: 2
project_name: flakylint
builds:
  - main: ./cmd/flakylint
    env:
      - CGO_ENABLED=0
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
archives:
  - formats: [tar.gz]
    format_overrides:
      - goos: windows
        formats: [zip]
changelog:
  use: github-native
```

`.github/workflows/release.yml`:

```yaml
name: Release
on:
  push:
    tags: ["v*"]
permissions:
  contents: write
jobs:
  goreleaser:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25.x"
      - uses: goreleaser/goreleaser-action@v6
        with:
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

- [ ] **Step 4: Write `README.md`**

```markdown
# flakylint

Static analysis for flaky Go tests. flakylint detects the known patterns
that make tests flaky — **before** they start flaking in CI.

Runtime flaky-test tools (retries, quarantine, dashboards) tell you a test
*became* flaky. flakylint is the other half: it flags the code patterns that
cause flakiness at review time, as a standard `go/analysis` linter. Use both.

## Checks

| Check | What it catches | Real-world incident |
|---|---|---|
| `httptestclose` | `httptest.NewServer` never closed — leaked port and goroutines; the GC finalizer races the HTTP connection pool | [google/go-github#4210](https://github.com/google/go-github/pull/4210) |
| `sleepassert` | `time.Sleep` in a test body to wait for concurrent work — races the scheduler, flakes under CI load; suggests `testing/synctest` (Go 1.24+) | [Go blog: synctest](https://go.dev/blog/synctest) |
| `parallelglobal` | writes to package-level variables in tests marked `t.Parallel()` — data race that only shows under parallel scheduling | — |
| `exitfatal` | `os.Exit` / `log.Fatal` in tests — kills the test binary, skips `t.Cleanup` and defers, poisons later tests; autofix to `t.Fatal` | — |

Design philosophy: **when in doubt, stay silent.** Every check is
conservative — e.g. `sleepassert` ignores sleeps in polling loops, and
`httptestclose` ignores servers that escape the function.

## Install

    go install github.com/malikov73/flakylint/cmd/flakylint@latest

Or download a binary from the releases page.

## Usage

    flakylint ./...

As a `go vet` tool:

    go vet -vettool=$(which flakylint) ./...

Disable an individual check:

    flakylint -sleepassert=false ./...

Checks with suggested fixes (`httptestclose`, `exitfatal`) apply them with
`-fix`.

## Roadmap

- map-iteration-order dependent assertions
- unclosed `resp.Body` in tests
- timeout contexts in parallel subtests
- goleak coverage check
- golangci-lint integration

## License

MIT
```

- [ ] **Step 5: Verify CI configs locally**

Run: `test -z "$(gofmt -l .)" && go vet ./... && go test ./... && go run ./cmd/flakylint ./...`
Expected: all pass, exit 0.

If `golangci-lint` is installed locally, also run `golangci-lint run`; fix any findings (typically unused parameters or shadowing) before committing. If it is not installed, rely on CI.

- [ ] **Step 6: Commit**

```bash
git add .github/ .golangci.yml .goreleaser.yml README.md
git commit -m "chore: add CI, release tooling, and README"
```

---

## Post-MVP checklist (not part of this plan)

Corpus run on kubernetes/grafana/prometheus/testcontainers-go, FP triage (< 5% target), upstream fix PRs, launch posts, golangci-lint submission — tracked in the design spec §6–7.
