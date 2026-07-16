package parallelglobal_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/malikov73/flakylint/analyzers/parallelglobal"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), parallelglobal.Analyzer, "a")
}
