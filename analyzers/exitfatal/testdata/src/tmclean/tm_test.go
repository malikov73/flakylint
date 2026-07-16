package tmclean

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run()) // canonical pattern, no defers: silent
}
