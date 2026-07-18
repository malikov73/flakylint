package a

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestProdLookalike carries the flagged shape but lives in a non-test file,
// so the InTestFile guard must keep maporder silent here.
func TestProdLookalike(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	var got []string
	for k := range m {
		got = append(got, k)
	}
	assert.Equal(t, []string{"a", "b"}, got)
}
