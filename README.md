# flakylint

> Static analysis for flaky Go tests — catch the patterns **before** they flake in CI.

[![CI](https://github.com/malikov73/flakylint/actions/workflows/ci.yml/badge.svg)](https://github.com/malikov73/flakylint/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/malikov73/flakylint.svg)](https://pkg.go.dev/github.com/malikov73/flakylint)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

```
$ flakylint ./...
worker_test.go:24:2: time.Sleep synchronizes the test on real time and flakes
    under CI load; use testing/synctest (Go 1.25+) or explicit synchronization
    (channel, sync.WaitGroup)
server_test.go:31:2: httptest server is never closed; the leaked port and
    goroutines can make later tests flaky
```

## Why

A flaky test fails your CI on Tuesday, passes on retry, and slowly teaches
your team to ignore red builds. The industry answer so far has been
**runtime detection**: rerun the suite, score the history, quarantine the
noisy tests. Useful — but it only tells you a test *became* flaky, after it
has already cost you reruns and trust.

The thing is, most flaky Go tests are not mysterious. They follow a handful
of well-known, statically detectable patterns: sleeping instead of
synchronizing, leaking an `httptest.Server`, racing parallel tests on shared
package state, killing the test binary before cleanup runs. Even Kubernetes
has a test that sleeps ten seconds with a comment admitting *"there is no
guarantee... we will just wait 10s, which is long enough in most cases."*

flakylint flags those patterns at review time, as a standard `go/analysis`
linter — the same machinery as `go vet`. It is the missing static half of
your flaky-test strategy. Use it *alongside* your runtime tooling, not
instead of it.

## Corpus evaluation

Before the first release we ran flakylint against four large open-source
codebases and hand-triaged every reported finding (or a random sample of 25
per check where there were more):

| Repo | Findings | Triaged | Correctly flagged | False positives |
|---|---|---|---|---|
| kubernetes/kubernetes | 130 | 28 | 28 | **0** |
| grafana/grafana | 104 | 46 | 46 | **0** |
| prometheus/prometheus | 26 | 26 | 26 | **0** |
| testcontainers-go | 4 | 4 | 4 | **0** |

**0 rule misclassifications across 104 reviewed findings — 52 actionable, 52
technically correct but low-value.** Every diagnostic pointed at code that
really does what the check says it does; whether that code is worth changing is
a separate, human call. Full triage notes live in
[`docs/corpus/`](docs/corpus/).

Broken out by check:

| Check | Findings | Triaged | Correctly flagged | False positives |
|---|---|---|---|---|
| `sleepassert` | 239 | 79 | 79 | **0** |
| `httptestclose` | 24 | 24 | 24 | **0** |
| `exitfatal` | 1 | 1 | 1 | **0** |
| `parallelglobal` | 0 | — | — | — |

`parallelglobal` produced no findings on the corpus — the pattern is rare in
mature codebases, which is exactly why it survives review when it does appear.

The three v0.2.0 checks (`hardport`, `maporder`, `eventuallyeffect`) went
through the same gate before release. The first corpus pass caught 3 rule
misclassifications, and both offending rules were narrowed (plain
last-write-wins captures and per-iteration accumulators are now silent by
design). A large batch of grafana `eventuallyeffect` hits were correct under
the old rule but idiomatic noise — that signal is what drove the narrowing. On
the re-run the narrowed checks add **no findings** across all four repos: a
compatibility result — mature suites already hold ports on `:0`, sort before
asserting, and keep polling callbacks pure — verified against synthetic
positive controls, not a claim about how often the checks fire in the wild.
Full write-up:
[`docs/corpus/2026-07-18-corpus-v020.md`](docs/corpus/2026-07-18-corpus-v020.md).

Some favorites the corpus run surfaced:

- a Grafana test sleeping through an async DB insert, annotated by its own
  author with `// TOFIX: this is a hack`;
- an `httptest.Server` leaked inside a table-driven loop — in a file that
  correctly calls `defer server.Close()` in three other places;
- the self-aware ten-second sleep in Kubernetes quoted above.

## The checks

[`httptestclose`](#httptestclose--leaked-httptest-servers) · [`sleepassert`](#sleepassert--sleeping-instead-of-synchronizing) · [`parallelglobal`](#parallelglobal--parallel-tests-sharing-package-state) · [`exitfatal`](#exitfatal--killing-the-test-binary) · [`hardport`](#hardport--hardcoded-ports-in-tests) · [`maporder`](#maporder--asserting-on-map-iteration-order) · [`eventuallyeffect`](#eventuallyeffect--side-effects-in-polling-callbacks)

### `httptestclose` — leaked httptest servers

```go
func TestHandler(t *testing.T) {
	srv := httptest.NewServer(mux)   // ← never closed
	resp, _ := http.Get(srv.URL + "/health")
	// ...
}
```

An unclosed `httptest.Server` keeps a port bound and goroutines running
after the test returns. Eventually the GC finalizer closes it — racing the
HTTP client's connection pool, which produces sporadic
`connection broken: CloseIdleConnections called` failures. Google's
go-github hit exactly this
([google/go-github#4210](https://github.com/google/go-github/pull/4210)).

**Autofix** (`-fix`): inserts `t.Cleanup(srv.Close)`.
Stays silent when the server escapes the function (returned, passed to a
helper, stored in a struct) — someone else may own its lifecycle.

### `sleepassert` — sleeping instead of synchronizing

```go
func TestWorker(t *testing.T) {
	go worker.Run()
	time.Sleep(100 * time.Millisecond) // ← races the scheduler
	if got := worker.Count(); got != 5 { ... }
}
```

A sleep that "waits for the goroutine" is a bet that the CI runner is as
fast as your laptop. It loses under load. The diagnostic points you to
[`testing/synctest`](https://go.dev/blog/synctest) (Go 1.25+), which makes
such tests deterministic *and* instant, or to plain channel/WaitGroup
synchronization. `testing/synctest` is stable since Go 1.25; earlier releases
shipped it only as a `GOEXPERIMENT`, with a different API.

Stays silent for sleeps inside polling/retry loops, inside `synctest`
bubbles, in goroutines and helpers, and for `time.Sleep(0)`. This is the
most opinionated check — on integration-heavy suites treat it as advisory,
or disable it: `-sleepassert=false`.

### `parallelglobal` — parallel tests sharing package state

```go
var counter int

func TestIncrement(t *testing.T) {
	t.Parallel()
	counter = 0        // ← races every other parallel test in the package
	// ...
}
```

`t.Parallel()` plus a package-level variable write couples tests that are
supposed to be independent: what one test observes now depends on how the
scheduler interleaves it with every other parallel test in the package — the
definition of a flake. The write is flagged whether or not a mutex guards it. A
lock removes the memory race but not the logical cross-test interference and
order-dependence, so two parallel tests mutating shared state generally break
each other's assumptions either way.

### `exitfatal` — killing the test binary

```go
func TestConfig(t *testing.T) {
	cfg, err := Load("testdata/cfg.yaml")
	if err != nil {
		log.Fatal(err)   // ← kills the whole binary, skips ALL cleanup
	}
	// ...
}
```

`os.Exit` and `log.Fatal` terminate the entire test process: `t.Cleanup`
callbacks and deferred teardown (containers, temp dirs, servers) never run,
poisoning every test that follows. In `TestMain`, `os.Exit(m.Run())` is
flagged only when the function has pending `defer`s that it would silently
skip.

**Autofix** (`-fix`): rewrites `log.Fatal*` → `t.Fatal*` — except where the
test's receiver name is shadowed at the call site, where the finding stands but
the rewrite is withheld (the diagnostic then asks you to route the failure
through the test's `*testing.T` rather than naming `t.Fatal` as the drop-in).

### `hardport` — hardcoded ports in tests

```go
func TestServer(t *testing.T) {
	ln, _ := net.Listen("tcp", ":8080") // ← fixed port, taken under parallelism
	// ...
}
```

A test that binds a fixed, non-zero port is a bet that nothing else on the
host wants it. It loses when `go test -p` runs package tests in parallel,
when several CI jobs share a runner, or when a leaked process from an earlier
run still holds the port — and the failure looks like a flake, not a bug. The
fix is to listen on `":0"` and read the real address back:

```go
ln, _ := net.Listen("tcp", ":0")
addr := ln.Addr().String() // ← kernel-assigned free port
```

or let `httptest.NewServer` pick the port for you.

Flags `net.Listen` / `net.ListenPacket` (only for a constant `tcp*`/`udp*`
network) and `http.ListenAndServe` / `ListenAndServeTLS`. Stays silent for
named ports (`":http"`), computed addresses (`fmt.Sprintf`), unix sockets and
other non-port networks, non-constant networks, out-of-range ports, the
wildcard port `":0"`, and Dial-side literals — those are not the bug.

### `maporder` — asserting on map iteration order

```go
func TestKeys(t *testing.T) {
	var got []string
	for k := range m {
		got = append(got, k) // ← collected in random order
	}
	assert.Equal(t, []string{"a", "b", "c"}, got) // ← flaky
}
```

The Go spec leaves map iteration order **unspecified**, and the runtime varies
it between runs, so a slice or string built by ranging over a map has no stable
order. An order-sensitive assertion against a fixed expected value passes only
when the order happens to line up — the test can go green for months and then
flake for no visible reason. Sort the accumulator first, or assert
order-insensitively:

```go
sort.Strings(got)
assert.Equal(t, []string{"a", "b", "c"}, got)
// or
assert.ElementsMatch(t, []string{"a", "b", "c"}, got)
```

Flags `assert`/`require` `Equal`/`EqualValues`, `reflect.DeepEqual`, and
`slices.Equal`/`EqualFunc` on a value accumulated in a map-range loop (testify
in its package-level form). The check is source-order aware: an assertion is
flagged only when it runs **after** the map-range loop that fills the
accumulator, and a `sort.*`/`slices.Sort*` call silences only the assertions
that follow it — a sort placed after the assertion does not help. Stays silent
when the accumulator is sorted before the assertion, asserted order-insensitively
(`ElementsMatch`, `Len`, `Contains`, `Subset`), used only as a testify message
argument, or escapes the test — passed to a helper, returned, reassigned,
address-taken, captured by a nested closure, or mixed with appends from outside
the loop. No autofix: choosing between sorting and `ElementsMatch` is a semantic
call only the author can make.

### `eventuallyeffect` — side effects in polling callbacks

```go
require.Eventually(t, func() bool {
	resp, _ := http.Post(url, "application/json", body)
	attempts++                                          // ← grows with the poll count
	return resp.StatusCode == http.StatusOK
}, time.Second, 10*time.Millisecond)
```

testify's `Eventually`/`Never`/`EventuallyWithT` retry their condition callback
until it passes — an **unpredictable** number of times that depends on machine
speed and scheduling. A callback that mutates state shared with the test makes
the outcome depend on that poll count: `attempts` ends up holding however many
ticks happened to fit on this run. The condition must be a pure observation —
act once, then poll on a read-only predicate, or assert on captured results
after `Eventually` returns:

```go
resp, _ := http.Post(url, "application/json", body) // act once
require.Eventually(t, func() bool {
	return ready(resp) // pure observation
}, time.Second, 10*time.Millisecond)
```

Flags, inside the callback of `assert`/`require` `Eventually`, `Eventuallyf`,
`Never`, `Neverf`, `EventuallyWithT`, and `EventuallyWithTf` (package-level
form), only **count-dependent** effects on a captured or package-level variable
— increments (`x++`, `x--`), compound assignments (`x += ...`, `x |= ...`), and
self-appends (`x = append(x, ...)`) — plus a send on a captured or package-level
channel. A send on a channel created inside the callback (`ch := make(...); ch
<- v`) never escapes a single tick and stays silent. A plain overwrite
(`x = v`, including multi-assign `a, b = f()`) is silent: it is last-write-wins,
the idiomatic way to capture the final successful tick's result, and flagging it
produced false positives on real code. Stays silent, too, for variables declared
inside the callback, the blank identifier, and — a deliberate v1 boundary —
keyed or field writes through a captured map, slice, or pointer (`m[k] = v`,
`p.f = v`), which are common in idempotent polling. Method calls (HTTP/DB
effects, mutex, `t.Log`, the `*assert.CollectT` of `EventuallyWithT`) are out of
scope in v1. No autofix. This ports testing-library's
[`no-wait-for-side-effects`](https://github.com/testing-library/eslint-plugin-testing-library/blob/main/docs/rules/no-wait-for-side-effects.md)
idea to Go.

## Install

```
go install github.com/malikov73/flakylint/cmd/flakylint@latest
```

Or grab a prebuilt binary from the
[releases page](https://github.com/malikov73/flakylint/releases).

## Usage

Run on your module:

```
flakylint ./...
```

Apply the suggested fixes (`httptestclose`, `exitfatal`):

```
flakylint -fix ./...
```

Add `-diff` to preview: `flakylint -fix -diff ./...` prints the rewrites as a
unified diff without touching files, and exits 0 even when findings exist.

Disable an individual check:

```
flakylint -sleepassert=false ./...
```

As a `go vet` tool (integrates with existing tooling and editors):

```
go vet -vettool=$(which flakylint) ./...
```

In GitHub Actions:

```yaml
- name: flakylint
  run: |
    # pin a release in CI; @latest suits interactive install only
    go install github.com/malikov73/flakylint/cmd/flakylint@v0.2.0
    flakylint ./...
```

flakylint only ever reports on `_test.go` files, so it is safe to run over
your whole module. The exit code depends on how you invoke it:

| Invocation | With findings | Exit code |
|---|---|---|
| `flakylint ./...` (text) | yes | `3` |
| `flakylint ./...` (text) | none | `0` |
| `flakylint -json ./...` | yes | `0` |
| `flakylint -fix ./...` | yes | `0` |
| `flakylint -fix -diff ./...` | yes | `0` |
| `go vet -vettool=$(which flakylint) ./...` | yes | `1` |

Treat the text mode's non-zero exit (`3`) as the CI gate. The `-json` and
`-fix` modes exit `0` even with findings so they compose with other tooling,
and under `go vet` the standard vet exit code (`1`) applies.

### Suppressing a finding

Silence a specific finding with a `//nolint` comment on the reported line or
the line directly above it:

```go
time.Sleep(10 * time.Millisecond) //nolint:sleepassert // waiting on a real socket
```

The name after the colon is the check name (`httptestclose`, `sleepassert`,
`parallelglobal`, `exitfatal`, `hardport`, `maporder`, `eventuallyeffect`);
list several
comma-separated to silence more than one. A bare `//nolint` suppresses every
check on that line.

## Design philosophy

**When in doubt, stay silent.** A linter earns trust by being right, not by
being loud. Every check ships with explicit silence heuristics (escaping
servers, polling loops, synctest bubbles, goroutine literals, canonical
`TestMain` patterns), and each heuristic is locked in by regression tests.
The 0-misclassification corpus result above is this philosophy, measured.

flakylint deliberately does **not** try to detect flakiness at runtime.
Retries, quarantine and scoring are a different job, done well by other
tools — flakylint prevents the patterns those tools would later catch.

## Roadmap

- `time.Now()` used as an expected value in assertions
- static goroutine-leak check (complementing runtime [goleak](https://github.com/uber-go/goleak))
- minimum-duration threshold option for `sleepassert`
- golangci-lint integration

## Contributing

Issues and PRs welcome. Each analyzer lives in its own package under
[`analyzers/`](analyzers/) with `analysistest`-based tests — a new check is
one package plus a registration line in
[`cmd/flakylint/main.go`](cmd/flakylint/main.go).

## License

[MIT](LICENSE)
