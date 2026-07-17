// Command flakylint reports flaky-test patterns in Go test files.
package main

import (
	"golang.org/x/tools/go/analysis/multichecker"

	"github.com/malikov73/flakylint/analyzers/exitfatal"
	"github.com/malikov73/flakylint/analyzers/httptestclose"
	"github.com/malikov73/flakylint/analyzers/parallelglobal"
	"github.com/malikov73/flakylint/analyzers/sleepassert"
	"github.com/malikov73/flakylint/internal/nolint"
)

func main() {
	multichecker.Main(
		nolint.Wrap(exitfatal.Analyzer),
		nolint.Wrap(httptestclose.Analyzer),
		nolint.Wrap(parallelglobal.Analyzer),
		nolint.Wrap(sleepassert.Analyzer),
	)
}
