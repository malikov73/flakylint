package fixf

import (
	"log"
	"testing"
)

func TestFix(t *testing.T) {
	log.Print("context")
	log.Fatalf("bad: %v", 1) // want `log.Fatalf inside a test terminates the whole test binary`
}

func BenchmarkFix(b *testing.B) {
	log.Fatal("bad") // want `log.Fatal inside a test terminates the whole test binary`
}

func TestNoName(_ *testing.T) {
	log.Fatal("bad") // want `log.Fatal inside a test terminates the whole test binary`
}
