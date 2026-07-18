package maporder_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/malikov73/flakylint/analyzers/maporder"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), maporder.Analyzer, "a")
}
