package p

import (
	"os"
	"testing"
)

func TestSimple(t *testing.T) { // want `testfunc t`
	t.Parallel()                      // want `parallel`
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

// Negative cases: same names, wrong package or non-testing receiver.
// None of these must be detected, so none carries a diagnostic marker.

func Exit(code int) { _ = code } // local Exit, not os.Exit

type fakeT struct{}

func (fakeT) Parallel()          {} // method named Parallel, receiver not *testing.T
func (fakeT) Run(string, func()) {} // method named Run, not a subtest

func TestNegatives(t *testing.T) { // want `testfunc t`
	Exit(0) // IsPkgFunc("os","Exit") must reject the wrong package
	var f fakeT
	f.Parallel()          // IsTestingMethod("Parallel") must reject the wrong receiver
	f.Run("x", func() {}) // IsTestingMethod("Run") must reject the wrong receiver
}
