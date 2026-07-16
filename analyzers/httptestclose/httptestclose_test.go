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
