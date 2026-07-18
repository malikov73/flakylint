package a

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var pkgReady bool

var pkgCount int

var pkgCh = make(chan int, 1)

func expensive() int { return 42 }

func get() (int, error) { return 1, nil }

// --- flagged (count-dependent effects) --------------------------------------

func TestAttempts(t *testing.T) {
	attempts := 0
	require.Eventually(t, func() bool {
		attempts++ // want `this write to "attempts" makes test state depend on the poll count`
		return attempts > 3
	}, time.Second, 10*time.Millisecond)
}

func TestCompoundAssign(t *testing.T) {
	total := 0
	assert.Eventually(t, func() bool {
		total += 2 // want `this write to "total" makes test state depend on the poll count`
		return total > 6
	}, time.Second, 10*time.Millisecond)
}

func TestPackageVar(t *testing.T) {
	assert.Eventually(t, func() bool {
		pkgCount++ // want `this write to "pkgCount" makes test state depend on the poll count`
		return pkgCount > 3
	}, time.Second, 10*time.Millisecond)
}

func TestChannelSend(t *testing.T) {
	ch := make(chan struct{}, 1)
	require.Eventually(t, func() bool {
		ch <- struct{}{} // want `this channel send happens once per poll tick`
		return true
	}, time.Second, 10*time.Millisecond)
}

func TestPackageChannelSend(t *testing.T) {
	require.Eventually(t, func() bool {
		pkgCh <- 1 // want `this channel send happens once per poll tick`
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

func TestAppendVariadic(t *testing.T) {
	var acc []int
	extra := []int{1, 2}
	assert.Eventually(t, func() bool {
		acc = append(acc, extra...) // want `this write to "acc" makes test state depend on the poll count`
		return len(acc) > 3
	}, time.Second, 10*time.Millisecond)
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
		count := 0
		assert.Eventually(t, func() bool {
			count++ // want `this write to "count" makes test state depend on the poll count`
			return count > 3
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

// --- silent -----------------------------------------------------------------

// Plain overwrite: last-write-wins, the idiomatic way to capture the final
// tick's value. Silent — the deliberate boundary.
func TestCapturedOverwrite(t *testing.T) {
	state := "pending"
	assert.Eventually(t, func() bool {
		state = "ready"
		return state == "ready"
	}, time.Second, 10*time.Millisecond)
}

// Plain overwrite of a package-level var: also last-write-wins, silent.
func TestPackageOverwrite(t *testing.T) {
	assert.Eventually(t, func() bool {
		pkgReady = true
		return pkgReady
	}, time.Second, 10*time.Millisecond)
}

// The prometheus/prometheus idiom: multi-assign captures the last read, guarded
// by the returned status, and the result is read after Eventually returns.
func TestGuardedLastRead(t *testing.T) {
	var r int
	var err error
	require.Eventually(t, func() bool {
		r, err = get()
		return err == nil && r > 0
	}, time.Second, 10*time.Millisecond)
	_ = r
}

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

// A channel created inside the callback never escapes a single tick, so the
// send is local bookkeeping — silent (review counterexample).
func TestLocalChannelSend(t *testing.T) {
	require.Eventually(t, func() bool {
		ch := make(chan int, 1)
		ch <- 1
		return <-ch == 1
	}, time.Second, 10*time.Millisecond)
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
