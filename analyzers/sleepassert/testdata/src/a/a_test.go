package a

import (
	"testing"
	"testing/synctest"
	"time"
)

func work()              {}
func check(t *testing.T) { t.Helper() }

func TestSleep(t *testing.T) {
	go work()
	time.Sleep(50 * time.Millisecond) // want `time.Sleep synchronizes the test on real time`
	check(t)
}

func TestSubtestSleep(t *testing.T) {
	t.Run("sub", func(t *testing.T) {
		time.Sleep(time.Second) // want `time.Sleep synchronizes the test on real time`
	})
}

func TestPollingLoop(t *testing.T) {
	for i := 0; i < 10; i++ {
		time.Sleep(10 * time.Millisecond) // polling loop: silent
	}
}

func TestRangeLoop(t *testing.T) {
	for range 3 {
		time.Sleep(time.Millisecond) // polling loop: silent
	}
}

func TestZeroSleep(t *testing.T) {
	time.Sleep(0) // zero duration: silent
}

func TestInsideSynctest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		time.Sleep(time.Second) // inside synctest bubble: silent
	})
}

func TestGoroutineSleep(t *testing.T) {
	go func() {
		time.Sleep(time.Millisecond) // helper literal, not the test body: silent
	}()
}

func helperSleep() {
	time.Sleep(time.Millisecond) // not a test function: silent
}
