package a

import (
	"testing"
	"time"
)

var ready bool

func work() { ready = true }

// Case 1: same-line //nolint:sleepassert suppresses the finding.
func TestSameLineNamed(t *testing.T) {
	go work()
	time.Sleep(10 * time.Millisecond) //nolint:sleepassert
	if !ready {
		t.Fatal("not ready")
	}
}

// Case 2: //nolint:sleepassert on the line directly above suppresses.
func TestPrevLineNamed(t *testing.T) {
	go work()
	//nolint:sleepassert
	time.Sleep(10 * time.Millisecond)
	if !ready {
		t.Fatal("not ready")
	}
}

// Case 3: bare //nolint suppresses every check.
func TestSameLineBare(t *testing.T) {
	go work()
	time.Sleep(10 * time.Millisecond) //nolint
	if !ready {
		t.Fatal("not ready")
	}
}

// Case 4: a trailing explanation after the directive is allowed.
func TestTrailingReason(t *testing.T) {
	go work()
	time.Sleep(10 * time.Millisecond) //nolint:sleepassert // inherited test
	if !ready {
		t.Fatal("not ready")
	}
}

// Case 5: a directive naming a different check does not suppress.
func TestWrongName(t *testing.T) {
	go work()
	//nolint:httptestclose
	time.Sleep(10 * time.Millisecond) // want `time.Sleep synchronizes the test on real time`
	if !ready {
		t.Fatal("not ready")
	}
}

// Case 6: a plain finding with no directive is reported.
func TestPlain(t *testing.T) {
	go work()
	time.Sleep(10 * time.Millisecond) // want `time.Sleep synchronizes the test on real time`
	if !ready {
		t.Fatal("not ready")
	}
}

// Case 7: a //nolintish prose comment is not a directive.
func TestNolintish(t *testing.T) {
	go work()
	//nolintish note about timing
	time.Sleep(10 * time.Millisecond) // want `time.Sleep synchronizes the test on real time`
	if !ready {
		t.Fatal("not ready")
	}
}
