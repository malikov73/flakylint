package eventuallyeffect_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/malikov73/flakylint/analyzers/eventuallyeffect"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), eventuallyeffect.Analyzer, "a")
}
