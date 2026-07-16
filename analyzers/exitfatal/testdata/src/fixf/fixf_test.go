package fixf

import (
	"log"
	"testing"
)

func TestFix(t *testing.T) {
	log.Print("context")
	log.Fatalf("bad: %v", 1) // want `log.Fatalf inside a test terminates the whole test binary`
}
