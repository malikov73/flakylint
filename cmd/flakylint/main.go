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
