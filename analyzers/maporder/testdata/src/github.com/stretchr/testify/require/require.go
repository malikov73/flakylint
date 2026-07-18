// Package require is a minimal stand-in for testify's require package, used
// only so the maporder testdata can reference the real import paths.
package require

// TestingT is the subset of *testing.T that the assertions accept.
type TestingT interface {
	Errorf(format string, args ...interface{})
}

func Equal(t TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool { return true }

func ElementsMatch(t TestingT, listA, listB interface{}, msgAndArgs ...interface{}) bool { return true }

func Len(t TestingT, object interface{}, length int, msgAndArgs ...interface{}) bool { return true }
