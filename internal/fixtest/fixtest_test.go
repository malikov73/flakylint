// Package fixtest is an end-to-end guard that the built flakylint binary's
// -fix mode never produces an unsafe rewrite.
//
// The per-analyzer analysistest golden files already pin the exact edits for
// each rule. This test complements them: it builds the real binary and runs
// -fix over a module that mixes safe fixes with the two autofix counterexamples
// from the external audit (an httptest constructor in an if-initializer and a
// log.Fatal whose testing receiver is shadowed), then asserts that every file
// still parses and that a second -fix run is a no-op.
package fixtest_test

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fixtureModule is a self-contained, stdlib-only test module. It pairs safe
// fixes (standalone httptest constructor, log.Fatal with a live receiver) with
// the counterexamples that must never be rewritten.
const fixtureModule = `package fixture

import (
	"log"
	"net/http/httptest"
	"testing"
)

func TestSafeCleanup(t *testing.T) {
	srv := httptest.NewServer(nil)
	_ = srv.URL
}

func TestSafeFatal(t *testing.T) {
	log.Fatal("boom")
}

func TestInitializer(t *testing.T) {
	if srv := httptest.NewServer(nil); srv.URL != "" {
		t.Log(srv.URL)
	}
}

func TestShadow(t *testing.T) {
	{
		t := 1
		_ = t
		log.Fatal("boom")
	}
}
`

func TestSuggestedFixesStayParseableAndIdempotent(t *testing.T) {
	bin := buildBinary(t)

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module fixture\n\ngo 1.25\n")
	testFile := filepath.Join(dir, "fixture_test.go")
	writeFile(t, testFile, fixtureModule)

	runFix(t, bin, dir)

	// (a) The whole module must still parse after -fix.
	first := readFile(t, testFile)
	if _, err := parser.ParseFile(token.NewFileSet(), testFile, first, parser.AllErrors); err != nil {
		t.Fatalf("file no longer parses after -fix:\n%v\n\n%s", err, first)
	}

	// The safe fixes must have been applied...
	assertContains(t, first, "t.Cleanup(srv.Close)", "standalone httptest constructor should gain a cleanup")
	assertContains(t, first, "t.Fatal(\"boom\")", "log.Fatal with a live receiver should be rewritten")
	// ...while the two counterexamples must be left untouched.
	assertContains(t, first, "if srv := httptest.NewServer(nil); srv.URL != \"\" {",
		"constructor in an if-initializer must not be edited")
	assertContains(t, first, "\t\tlog.Fatal(\"boom\")",
		"log.Fatal with a shadowed receiver must not be rewritten to t.Fatal")

	// (b) A second -fix run must change nothing.
	runFix(t, bin, dir)
	second := readFile(t, testFile)
	if first != second {
		t.Fatalf("-fix is not idempotent; second run changed the file:\n%s", second)
	}
}

// buildBinary compiles the flakylint command into the test's temp dir and
// returns the resulting binary path.
func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "flakylint")
	cmd := exec.Command("go", "build", "-mod=mod", "-o", bin, "./cmd/flakylint")
	cmd.Dir = moduleRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building flakylint failed: %v\n%s", err, out)
	}
	return bin
}

// runFix runs the built binary with -fix over the module in dir. -fix exits 0
// even when it applies edits, so a non-zero code is a real failure.
func runFix(t *testing.T, bin, dir string) {
	t.Helper()
	cmd := exec.Command(bin, "-fix", "./...")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("flakylint -fix failed: %v\n%s", err, out)
	}
}

// moduleRoot returns the flakylint module root derived from this file's path
// (internal/fixtest/fixtest_test.go).
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine caller path")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}

func assertContains(t *testing.T, haystack, needle, msg string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("%s: expected to find %q in:\n%s", msg, needle, haystack)
	}
}
