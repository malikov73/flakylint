// Package hardport reports tests that bind a hardcoded, non-zero port.
//
// A test that listens on a fixed address such as ":8080" flakes when the
// port is already taken: `go test -p` runs package tests in parallel, CI
// runners share a host, and a leaked process from an earlier run can still
// hold the port. Listening on ":0" lets the kernel pick a free port, which
// the test then reads back via the listener's Addr.
package hardport

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
	"net"
	"strconv"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/malikov73/flakylint/internal/testfuncs"
)

var Analyzer = &analysis.Analyzer{
	Name:     "hardport",
	Doc:      "reports tests binding a hardcoded, non-zero port; the port can be taken by parallel tests or CI jobs, causing flakes",
	URL:      "https://github.com/malikov73/flakylint",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// listenFuncs are the package-level listen functions we inspect, paired with
// the argument index that carries the address. Dial-side calls and the typed
// net.ListenTCP/UDP forms are intentionally out of scope.
var listenFuncs = []struct {
	pkg, name string
	argIdx    int
}{
	{"net", "Listen", 1},
	{"net", "ListenPacket", 1},
	{"net/http", "ListenAndServe", 0},
	{"net/http", "ListenAndServeTLS", 0},
}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	nodeFilter := []ast.Node{(*ast.CallExpr)(nil), (*ast.CompositeLit)(nil)}
	insp.Preorder(nodeFilter, func(n ast.Node) {
		if !testfuncs.InTestFile(pass, n) {
			return
		}
		switch node := n.(type) {
		case *ast.CallExpr:
			checkCall(pass, node)
		case *ast.CompositeLit:
			checkServerLit(pass, node)
		}
	})
	return nil, nil
}

func checkCall(pass *analysis.Pass, call *ast.CallExpr) {
	for _, f := range listenFuncs {
		if !testfuncs.IsPkgFunc(pass.TypesInfo, call, f.pkg, f.name) {
			continue
		}
		if f.argIdx >= len(call.Args) {
			return
		}
		reportIfHardcoded(pass, call.Args[f.argIdx])
		return
	}
}

// checkServerLit reports a hardcoded Addr in an http.Server composite literal
// (both `http.Server{...}` and `&http.Server{...}` reach here as the literal).
func checkServerLit(pass *analysis.Pass, lit *ast.CompositeLit) {
	if !isHTTPServer(pass.TypesInfo.TypeOf(lit)) {
		return
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Addr" {
			continue
		}
		reportIfHardcoded(pass, kv.Value)
		return
	}
}

func isHTTPServer(t types.Type) bool {
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Pkg() != nil && obj.Pkg().Path() == "net/http" && obj.Name() == "Server"
}

func reportIfHardcoded(pass *analysis.Pass, expr ast.Expr) {
	addr, ok := hardcodedAddr(pass, expr)
	if !ok {
		return
	}
	pass.Report(analysis.Diagnostic{
		Pos:     expr.Pos(),
		End:     expr.End(),
		Message: fmt.Sprintf("test binds hardcoded address %q; the port can be taken by parallel tests or CI jobs — listen on \":0\" and read the real address back", addr),
	})
}

// hardcodedAddr returns the constant address string of expr and true when it
// resolves to a constant "host:port" with a numeric, non-zero port. Named
// consts resolve too, since TypesInfo records their constant value. It stays
// silent (false) for non-constant addresses, unix sockets and other strings
// SplitHostPort rejects, named ports like ":http", and the wildcard port 0.
func hardcodedAddr(pass *analysis.Pass, expr ast.Expr) (string, bool) {
	tv, ok := pass.TypesInfo.Types[expr]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	s := constant.StringVal(tv.Value)
	_, port, err := net.SplitHostPort(s)
	if err != nil {
		return "", false
	}
	n, err := strconv.Atoi(port)
	if err != nil || n == 0 {
		return "", false
	}
	return s, true
}
