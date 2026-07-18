package a

import (
	"log"
	"os"
	"testing"
)

func TestExit(t *testing.T) {
	if os.Getenv("MUST_FAIL") != "" {
		os.Exit(1) // want `os.Exit inside a test terminates the whole test binary`
	}
}

func TestLogFatal(t *testing.T) {
	log.Fatal("boom") // want `log.Fatal inside a test terminates the whole test binary`
}

func TestLogFatalf(t *testing.T) {
	log.Fatalf("boom %d", 1) // want `log.Fatalf inside a test terminates the whole test binary`
}

func TestSubtestFatal(t *testing.T) {
	t.Run("sub", func(t *testing.T) {
		log.Fatalln("boom") // want `log.Fatalln inside a test terminates the whole test binary`
	})
}

func TestGoroutineFatal(t *testing.T) {
	go func() {
		log.Fatal("boom") // helper literal, not the test body: silent
	}()
}

func BenchmarkFatal(b *testing.B) {
	log.Fatal("boom") // want `log.Fatal inside a test terminates the whole test binary and skips cleanup; use b.Fatal`
}

func BenchmarkExit(b *testing.B) {
	os.Exit(1) // want `os.Exit inside a test terminates the whole test binary and skips cleanup; use b.Fatal or b.Skip`
}

func TestShadowed(t *testing.T) {
	{
		t := 1
		_ = t
		// t is shadowed by an int here, so no t.Fatal drop-in is named and no fix is offered.
		log.Fatal("boom") // want `route the failure through the test's`
	}
}

func TestShadowedExit(t *testing.T) {
	{
		t := 1
		_ = t
		os.Exit(1) // want `route the failure through the test's`
	}
}

func helper() {
	os.Exit(1) // not a test function: silent
}
