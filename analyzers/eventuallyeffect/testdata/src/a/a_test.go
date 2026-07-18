package a

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var pkgReady bool

func expensive() int { return 42 }

// --- flagged ---------------------------------------------------------------

func TestAttempts(t *testing.T) {
	attempts := 0
	require.Eventually(t, func() bool {
		attempts++ // want `this write to "attempts" makes test state depend on the poll count`
		return attempts > 3
	}, time.Second, 10*time.Millisecond)
}

func TestCapturedAssign(t *testing.T) {
	state := "pending"
	assert.Eventually(t, func() bool {
		state = "ready" // want `this write to "state" makes test state depend on the poll count`
		return state == "ready"
	}, time.Second, 10*time.Millisecond)
}

func TestPackageVar(t *testing.T) {
	assert.Eventually(t, func() bool {
		pkgReady = true // want `this write to "pkgReady" makes test state depend on the poll count`
		return pkgReady
	}, time.Second, 10*time.Millisecond)
}

func TestChannelSend(t *testing.T) {
	ch := make(chan struct{}, 1)
	require.Eventually(t, func() bool {
		ch <- struct{}{} // want `this channel send happens once per poll tick`
		return true
	}, time.Second, 10*time.Millisecond)
}

func TestAppendCaptured(t *testing.T) {
	var results []int
	assert.Eventuallyf(t, func() bool {
		results = append(results, 1) // want `this write to "results" makes test state depend on the poll count`
		return len(results) > 3
	}, time.Second, 10*time.Millisecond, "waiting")
}

func TestNever(t *testing.T) {
	seen := 0
	require.Never(t, func() bool {
		seen++ // want `this write to "seen" makes test state depend on the poll count`
		return seen > 100
	}, time.Second, 10*time.Millisecond)
}

func TestNestedSubtest(t *testing.T) {
	t.Run("sub", func(t *testing.T) {
		done := false
		assert.Eventually(t, func() bool {
			done = true // want `this write to "done" makes test state depend on the poll count`
			return done
		}, time.Second, 10*time.Millisecond)
	})
}

func TestNestedGoroutine(t *testing.T) {
	attempts := 0
	require.Eventually(t, func() bool {
		go func() {
			attempts++ // want `this write to "attempts" makes test state depend on the poll count`
		}()
		return attempts > 3
	}, time.Second, 10*time.Millisecond)
}

// --- silent ----------------------------------------------------------------

func TestLocalOnly(t *testing.T) {
	items := []int{1, 2, 3}
	require.Eventually(t, func() bool {
		n := len(items)
		total := 0
		total += n
		return total == 3
	}, time.Second, 10*time.Millisecond)
}

func TestNestedLocalOnly(t *testing.T) {
	require.Eventually(t, func() bool {
		go func() {
			acc := 0
			acc++
			_ = acc
		}()
		return true
	}, time.Second, 10*time.Millisecond)
}

func TestBlank(t *testing.T) {
	assert.Eventually(t, func() bool {
		_ = expensive()
		return true
	}, time.Second, 10*time.Millisecond)
}

func TestIndexedAndField(t *testing.T) {
	m := map[string]int{}
	type box struct{ n int }
	p := &box{}
	assert.Eventually(t, func() bool {
		m["k"] = 1
		p.n = 2
		return m["k"] == 1 && p.n == 2
	}, time.Second, 10*time.Millisecond)
}

func TestReadsOnly(t *testing.T) {
	x := []int{1, 2, 3}
	require.Eventually(t, func() bool {
		return len(x) == 3
	}, time.Second, 10*time.Millisecond)
}

func TestNotEventually(t *testing.T) {
	attempts := 0
	t.Cleanup(func() {
		attempts++
	})
	inc := func() {
		attempts++
	}
	inc()
	_ = attempts
}

func TestOutsideCallback(t *testing.T) {
	ch := make(chan int, 1)
	count := 0
	count++
	ch <- count
	assert.Eventually(t, func() bool {
		return count > 0
	}, time.Second, 10*time.Millisecond)
}

func TestCollectT(t *testing.T) {
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		c.Errorf("not yet")
	}, time.Second, 10*time.Millisecond)
}
