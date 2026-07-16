package sleepassert_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/malikov73/flakylint/analyzers/sleepassert"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), sleepassert.Analyzer, "a")
}
