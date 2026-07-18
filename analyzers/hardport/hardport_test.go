package hardport_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/malikov73/flakylint/analyzers/hardport"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), hardport.Analyzer, "a")
}
