// Package assert is a minimal stand-in for testify's assert package, used
// only so the maporder testdata can reference the real import paths.
package assert

// TestingT is the subset of *testing.T that the assertions accept.
type TestingT interface {
	Errorf(format string, args ...interface{})
}

func Equal(t TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool { return true }

func Equalf(t TestingT, expected, actual interface{}, msg string, args ...interface{}) bool {
	return true
}

func EqualValues(t TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool {
	return true
}

func ElementsMatch(t TestingT, listA, listB interface{}, msgAndArgs ...interface{}) bool { return true }

func Len(t TestingT, object interface{}, length int, msgAndArgs ...interface{}) bool { return true }

func Contains(t TestingT, s, contains interface{}, msgAndArgs ...interface{}) bool { return true }

func Subset(t TestingT, list, subset interface{}, msgAndArgs ...interface{}) bool { return true }
