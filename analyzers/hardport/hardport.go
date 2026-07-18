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
// the argument indices for the network and the address. networkIdx is -1 when
// the network is implicitly tcp (the http.ListenAndServe family). Dial-side
// calls and the typed net.ListenTCP/UDP forms are intentionally out of scope.
var listenFuncs = []struct {
	pkg, name           string
	networkIdx, addrIdx int
}{
	{"net", "Listen", 0, 1},
	{"net", "ListenPacket", 0, 1},
	{"net/http", "ListenAndServe", -1, 0},
	{"net/http", "ListenAndServeTLS", -1, 0},
}

// portNetworks are the networks that bind a numeric TCP/UDP port; only these
// carry the "port already taken" flake. unix/unixgram/ip and anything else
// bind a path or protocol, not a port.
var portNetworks = map[string]bool{
	"tcp": true, "tcp4": true, "tcp6": true,
	"udp": true, "udp4": true, "udp6": true,
}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		if !testfuncs.InTestFile(pass, n) {
			return
		}
		checkCall(pass, n.(*ast.CallExpr))
	})
	return nil, nil
}

func checkCall(pass *analysis.Pass, call *ast.CallExpr) {
	for _, f := range listenFuncs {
		if !testfuncs.IsPkgFunc(pass.TypesInfo, call, f.pkg, f.name) {
			continue
		}
		if f.addrIdx >= len(call.Args) {
			return
		}
		if f.networkIdx >= 0 {
			if f.networkIdx >= len(call.Args) || !isPortNetwork(pass, call.Args[f.networkIdx]) {
				return // non-constant or non-TCP/UDP network: no port to flake
			}
		}
		reportIfHardcoded(pass, call.Args[f.addrIdx])
		return
	}
}

// isPortNetwork reports whether expr is a constant string naming a TCP or UDP
// network. A non-constant or unrecognized network is not a port bind.
func isPortNetwork(pass *analysis.Pass, expr ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[expr]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return false
	}
	return portNetworks[constant.StringVal(tv.Value)]
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
// resolves to a constant "host:port" with a numeric port in 1..65535. Named
// consts resolve too, since TypesInfo records their constant value. It stays
// silent (false) for non-constant addresses, unix sockets and other strings
// SplitHostPort rejects, named ports like ":http", the wildcard port 0, and
// out-of-range ports.
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
	if err != nil || n < 1 || n > 65535 {
		return "", false
	}
	return s, true
}
