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
