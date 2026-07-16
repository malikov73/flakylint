package a

import (
	"sync"
	"testing"

	"b"
)

var (
	counter int
	state   = map[string]int{}
	mu      sync.Mutex
)

func TestParallelWrite(t *testing.T) {
	t.Parallel()
	counter = 1 // want `parallel test writes package-level variable "counter"`
}

func TestParallelCompound(t *testing.T) {
	t.Parallel()
	counter += 1   // want `parallel test writes package-level variable "counter"`
	counter++      // want `parallel test writes package-level variable "counter"`
	state["k"] = 1 // want `parallel test writes package-level variable "state"`
	b.Counter = 2  // want `parallel test writes package-level variable "Counter"`
}

func TestSequentialWrite(t *testing.T) {
	counter = 1 // not parallel: silent
}

func TestParallelLocal(t *testing.T) {
	t.Parallel()
	local := 0
	local++
	_ = local
}

func TestParallelSubtest(t *testing.T) {
	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		counter = 3 // want `parallel test writes package-level variable "counter"`
	})
}

func TestParallelParentSequentialChild(t *testing.T) {
	t.Parallel()
	t.Run("sub", func(t *testing.T) {
		counter = 4 // documented false negative: subtest unit is not itself parallel
	})
}

func TestMutexStillFlagged(t *testing.T) {
	t.Parallel()
	mu.Lock()
	counter = 5 // want `parallel test writes package-level variable "counter"`
	mu.Unlock()
}
