// Command flakylint reports flaky-test patterns in Go test files.
package main

import (
	"fmt"
	"os"

	"golang.org/x/tools/go/analysis/multichecker"

	"github.com/malikov73/flakylint/analyzers/eventuallyeffect"
	"github.com/malikov73/flakylint/analyzers/exitfatal"
	"github.com/malikov73/flakylint/analyzers/hardport"
	"github.com/malikov73/flakylint/analyzers/httptestclose"
	"github.com/malikov73/flakylint/analyzers/maporder"
	"github.com/malikov73/flakylint/analyzers/parallelglobal"
	"github.com/malikov73/flakylint/analyzers/sleepassert"
	"github.com/malikov73/flakylint/internal/nolint"
	versionpkg "github.com/malikov73/flakylint/internal/version"
)

// Build metadata, injected via -ldflags -X by goreleaser. Empty in plain
// `go build`/`go install` builds, where the version command reconstructs
// what it can from runtime/debug.ReadBuildInfo.
var (
	version string
	commit  string
	date    string
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-version":
			fmt.Println(versionpkg.Format(version, commit, date))
			return
		}
	}

	multichecker.Main(
		nolint.Wrap(eventuallyeffect.Analyzer),
		nolint.Wrap(exitfatal.Analyzer),
		nolint.Wrap(hardport.Analyzer),
		nolint.Wrap(httptestclose.Analyzer),
		nolint.Wrap(maporder.Analyzer),
		nolint.Wrap(parallelglobal.Analyzer),
		nolint.Wrap(sleepassert.Analyzer),
	)
}
