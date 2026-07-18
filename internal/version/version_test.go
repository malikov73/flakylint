package version

import (
	"runtime/debug"
	"testing"
)

func TestFormat_ldflagsWin(t *testing.T) {
	got := format("v0.2.0", "abc1234", "2026-07-18T00:00:00Z", nil, false)
	want := "flakylint v0.2.0 (commit abc1234, built 2026-07-18T00:00:00Z)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFormat_fallbackFromBuildInfo(t *testing.T) {
	info := &debug.BuildInfo{
		Main: debug.Module{Version: "v0.3.1"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "deadbeef"},
			{Key: "vcs.time", Value: "2026-07-18T12:00:00Z"},
		},
	}
	got := format("", "", "", info, true)
	want := "flakylint v0.3.1 (commit deadbeef, built 2026-07-18T12:00:00Z)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFormat_fallbackDevelWhenNoBuildInfo(t *testing.T) {
	got := format("", "", "", nil, false)
	want := "flakylint devel (commit none, built unknown)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFormat_fallbackDevelWhenBuildInfoIsDevel(t *testing.T) {
	info := &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}
	got := format("", "", "", info, true)
	want := "flakylint devel (commit none, built unknown)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFormat_partialLdflagsMixWithBuildInfo(t *testing.T) {
	info := &debug.BuildInfo{
		Main:     debug.Module{Version: "v0.3.1"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "deadbeef"}},
	}
	got := format("v0.2.0", "", "", info, true)
	want := "flakylint v0.2.0 (commit deadbeef, built unknown)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
