package nolint_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/malikov73/flakylint/analyzers/sleepassert"
	"github.com/malikov73/flakylint/internal/nolint"
)

func TestWrap(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), nolint.Wrap(sleepassert.Analyzer), "a")
}
