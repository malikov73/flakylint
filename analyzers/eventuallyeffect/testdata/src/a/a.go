package a

import (
	"time"

	"github.com/stretchr/testify/require"
)

// pollProd carries the flagged shape but lives in a non-test file, so the
// InTestFile guard must keep eventuallyeffect silent here.
func pollProd() {
	attempts := 0
	require.Eventually(nil, func() bool {
		attempts++
		return attempts > 3
	}, time.Second, 10*time.Millisecond)
}
