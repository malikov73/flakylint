package p

import (
	"os"
	"testing"
)

func TestSimple(t *testing.T) { // want `testfunc t`
	t.Parallel() // want `parallel`
	t.Run("sub", func(t *testing.T) { // want `subtest t`
	})
}

func BenchmarkB(b *testing.B) { // want `testfunc b`
}

func FuzzF(f *testing.F) { // want `testfunc f`
}

func TestMain(m *testing.M) { // want `testmain`
	os.Exit(m.Run()) // want `osexit`
}

// Not tests: lowercase after prefix, helper signature, no param.
func Testhelper(t *testing.T) {
	_ = t
}

func helper(t *testing.T) {
	_ = t
}
