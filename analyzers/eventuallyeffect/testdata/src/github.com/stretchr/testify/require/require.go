// Package require is a minimal stand-in for testify's require package, used
// only so the eventuallyeffect testdata can reference the real import paths.
package require

import (
	"time"

	"github.com/stretchr/testify/assert"
)

// TestingT is the subset of *testing.T that the assertions accept.
type TestingT interface {
	Errorf(format string, args ...interface{})
}

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

func EventuallyWithT(t TestingT, condition func(collect *assert.CollectT), waitFor, tick time.Duration, msgAndArgs ...interface{}) bool {
	return true
}

func EventuallyWithTf(t TestingT, condition func(collect *assert.CollectT), waitFor, tick time.Duration, msg string, args ...interface{}) bool {
	return true
}
