// Package assert is a minimal stand-in for testify's assert package, used
// only so the eventuallyeffect testdata can reference the real import paths.
package assert

import "time"

// TestingT is the subset of *testing.T that the assertions accept.
type TestingT interface {
	Errorf(format string, args ...interface{})
}

// CollectT is the stand-in for testify's assert.CollectT, the value passed to
// EventuallyWithT callbacks.
type CollectT struct{}

func (c *CollectT) Errorf(format string, args ...interface{}) {}

func Eventually(t TestingT, condition func() bool, waitFor, tick time.Duration, msgAndArgs ...interface{}) bool {
	return true
}

func Eventuallyf(t TestingT, condition func() bool, waitFor, tick time.Duration, msg string, args ...interface{}) bool {
	return true
}

func Never(t TestingT, condition func() bool, waitFor, tick time.Duration, msgAndArgs ...interface{}) bool {
	return true
}

func Neverf(t TestingT, condition func() bool, waitFor, tick time.Duration, msg string, args ...interface{}) bool {
	return true
}

func EventuallyWithT(t TestingT, condition func(collect *CollectT), waitFor, tick time.Duration, msgAndArgs ...interface{}) bool {
	return true
}

func EventuallyWithTf(t TestingT, condition func(collect *CollectT), waitFor, tick time.Duration, msg string, args ...interface{}) bool {
	return true
}
