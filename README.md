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

Most flaky Go tests are not mysterious. They follow a handful of well-known,
statically detectable patterns: sleeping instead of synchronizing, leaking an
`httptest.Server`, racing parallel tests on shared package state, killing the
test binary before cleanup runs. The industry answer so far has been **runtime
detection** — rerun, score, quarantine — which only tells you a test *became*
flaky, after it has already cost you reruns and trust. flakylint flags the
patterns at review time, as a standard `go/analysis` linter — the same
machinery as `go vet`. Use it *alongside* your runtime tooling, not instead
of it.

## Install

```
go install github.com/malikov73/flakylint/cmd/flakylint@latest
```

Or grab a prebuilt binary from the
[releases page](https://github.com/malikov73/flakylint/releases).

## Usage

```
flakylint ./...                  # report findings (exit 3 if any)
flakylint -fix ./...             # apply suggested fixes
flakylint -fix -diff ./...       # preview fixes as a unified diff
flakylint -sleepassert=false ./...   # disable an individual check
go vet -vettool=$(which flakylint) ./...   # as a go vet tool
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
your whole module. Exit codes and `//nolint` suppression are covered in the
[Reference](#reference) section.

## The checks

Seven checks, each with explicit silence heuristics — expand a check for
examples and the exact rules.

### `sleepassert` — sleeping instead of synchronizing

`time.Sleep` before an assertion is a bet that the CI runner is as fast as
your laptop. It loses under load.

<details>
<summary>Example, fix, and silence rules</summary>

```go
func TestWorker(t *testing.T) {
	go worker.Run()
	time.Sleep(100 * time.Millisecond) // ← races the scheduler
	if got := worker.Count(); got != 5 { ... }
}
```

The diagnostic points you to
[`testing/synctest`](https://go.dev/blog/synctest) (Go 1.25+), which makes
such tests deterministic *and* instant, or to plain channel/WaitGroup
synchronization. `testing/synctest` is stable since Go 1.25; earlier releases
shipped it only as a `GOEXPERIMENT`, with a different API.

Stays silent for sleeps inside polling/retry loops, inside `synctest`
bubbles, in goroutines and helpers, and for `time.Sleep(0)`. This is the
most opinionated check — on integration-heavy suites treat it as advisory,
or disable it: `-sleepassert=false`.

</details>

### `httptestclose` — leaked httptest servers

An unclosed `httptest.Server` keeps its port and goroutines alive and can
fail later tests with sporadic connection errors.
**Autofix**: inserts `t.Cleanup(srv.Close)`.

<details>
<summary>Example, fix, and silence rules</summary>

```go
func TestHandler(t *testing.T) {
	srv := httptest.NewServer(mux)   // ← never closed
	resp, _ := http.Get(srv.URL + "/health")
	// ...
}
```

After the test returns, the GC finalizer eventually closes the server —
racing the HTTP client's connection pool, which produces sporadic
`connection broken: CloseIdleConnections called` failures. Google's
go-github hit exactly this
([google/go-github#4210](https://github.com/google/go-github/pull/4210)).

The fix is only offered where inserting a statement is syntactically safe
(not inside `if`/`for`/`switch` initializers — there the finding stands
without a rewrite). Stays silent when the server escapes the function
(returned, passed to a helper, stored in a struct) — someone else may own
its lifecycle.

</details>

### `parallelglobal` — parallel tests sharing package state

A `t.Parallel()` test writing a package-level variable couples tests that
are supposed to be independent — what one test observes depends on scheduling.

<details>
<summary>Example and silence rules</summary>

```go
var counter int

func TestIncrement(t *testing.T) {
	t.Parallel()
	counter = 0        // ← races every other parallel test in the package
	// ...
}
```

The write is flagged whether or not a mutex guards it. A lock removes the
memory race but not the logical cross-test interference and order-dependence,
so two parallel tests mutating shared state generally break each other's
assumptions either way. One diagnostic per test and variable.

</details>

### `exitfatal` — killing the test binary

`os.Exit` and `log.Fatal` terminate the whole test process: `t.Cleanup` and
deferred teardown never run, poisoning every test that follows.
**Autofix**: rewrites `log.Fatal*` → `t.Fatal*`.

<details>
<summary>Example, fix, and silence rules</summary>

```go
func TestConfig(t *testing.T) {
	cfg, err := Load("testdata/cfg.yaml")
	if err != nil {
		log.Fatal(err)   // ← kills the whole binary, skips ALL cleanup
	}
	// ...
}
```

In `TestMain`, `os.Exit(m.Run())` is flagged only when the function has
pending `defer`s that it would silently skip. The `log.Fatal*` → `t.Fatal*`
rewrite is withheld where the test's receiver name is shadowed at the call
site — the finding stands, and the diagnostic asks you to route the failure
through the test's `*testing.T` instead.

</details>

### `hardport` — hardcoded ports in tests

Binding a fixed, non-zero port (`:8080`) flakes under `go test -p`
parallelism, shared CI runners, and leaked processes — listen on `":0"`.

<details>
<summary>Example, fix, and silence rules</summary>

```go
func TestServer(t *testing.T) {
	ln, _ := net.Listen("tcp", ":8080") // ← fixed port, taken under parallelism
	// ...
}
```

The fix is to listen on `":0"` and read the real address back:

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

</details>

### `maporder` — asserting on map iteration order

The Go spec leaves map iteration order unspecified and the runtime varies it
between runs — an order-sensitive assertion on values collected from a map
range can go green for months and then flake.

<details>
<summary>Example, fix, and silence rules</summary>

```go
func TestKeys(t *testing.T) {
	var got []string
	for k := range m {
		got = append(got, k) // ← collected in random order
	}
	assert.Equal(t, []string{"a", "b", "c"}, got) // ← flaky
}
```

Sort the accumulator first, or assert order-insensitively:

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

</details>

### `eventuallyeffect` — side effects in polling callbacks

testify's `Eventually` retries its callback an unpredictable number of times;
count-dependent effects inside it (`attempts++`, channel sends) make test
state depend on machine speed.

<details>
<summary>Example, fix, and silence rules</summary>

```go
require.Eventually(t, func() bool {
	resp, _ := http.Post(url, "application/json", body)
	attempts++                                          // ← grows with the poll count
	return resp.StatusCode == http.StatusOK
}, time.Second, 10*time.Millisecond)
```

The condition must be a pure observation — act once, then poll on a read-only
predicate, or assert on captured results after `Eventually` returns:

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

</details>

## Corpus evaluation

Every check is validated against kubernetes/kubernetes, grafana/grafana,
prometheus/prometheus, and testcontainers-go before release, with every
reported finding (or a 25-per-check random sample) hand-triaged.

**0 rule misclassifications across 104 reviewed findings** for the v0.1
checks — 52 actionable, 52 technically correct but low-value:

| Check | Findings | Triaged | Correctly flagged | False positives |
|---|---|---|---|---|
| `sleepassert` | 239 | 79 | 79 | **0** |
| `httptestclose` | 24 | 24 | 24 | **0** |
| `exitfatal` | 1 | 1 | 1 | **0** |
| `parallelglobal` | 0 | — | — | — |

The v0.2.0 checks went through the same gate: the first pass caught 3 rule
misclassifications, both offending rules were narrowed, and the re-run adds
no findings across all four repos. Full accounting, per-repo numbers, and
reproduction details: [`docs/corpus/`](docs/corpus/).

Some favorites the corpus surfaced: a Grafana test sleeping through an async
DB insert annotated by its own author with `// TOFIX: this is a hack`; an
`httptest.Server` leaked inside a table-driven loop — in a file that
correctly calls `defer server.Close()` in three other places; and a
self-aware Kubernetes ten-second sleep admitting *"there is no guarantee...
we will just wait 10s, which is long enough in most cases."*

## Design philosophy

**When in doubt, stay silent.** A linter earns trust by being right, not by
being loud. Every check ships with explicit silence heuristics (escaping
servers, polling loops, synctest bubbles, goroutine literals, canonical
`TestMain` patterns), and each heuristic is locked in by regression tests.
The 0-misclassification corpus result above is this philosophy, measured.

flakylint deliberately does **not** try to detect flakiness at runtime.
Retries, quarantine and scoring are a different job, done well by other
tools — flakylint prevents the patterns those tools would later catch.

## Reference

### Exit codes

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

The name after the colon is the check name; list several comma-separated to
silence more than one. A bare `//nolint` suppresses every check on that line.

### Version

`flakylint version` prints the build version, commit, and date.

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
