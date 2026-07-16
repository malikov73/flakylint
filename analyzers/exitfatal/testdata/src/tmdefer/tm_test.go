package tmdefer

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	f, _ := os.CreateTemp("", "x")
	defer os.Remove(f.Name())
	os.Exit(m.Run()) // want `os.Exit in TestMain skips the function's pending defers`
}
