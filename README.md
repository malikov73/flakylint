# flakylint

> Static analysis for flaky Go tests — catch the patterns **before** they flake in CI.

[![CI](https://github.com/malikov73/flakylint/actions/workflows/ci.yml/badge.svg)](https://github.com/malikov73/flakylint/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/malikov73/flakylint.svg)](https://pkg.go.dev/github.com/malikov73/flakylint)
[![Go Report Card](https://goreportcard.com/badge/github.com/malikov73/flakylint)](https://goreportcard.com/report/github.com/malikov73/flakylint)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

```
$ flakylint ./...
worker_test.go:24:2: time.Sleep synchronizes the test on real time and flakes
    under CI load; use testing/synctest (Go 1.24+) or explicit synchronization
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

## Field-tested

Before the first release we ran flakylint against four large open-source
codebases and hand-triaged every reported finding (or a random sample of 25
per check where there were more):

| Repo | Findings | Triaged | Correctly flagged | False positives |
|---|---|---|---|---|
| kubernetes/kubernetes | 130 | 28 | 28 | **0** |
| grafana/grafana | 104 | 46 | 46 | **0** |
| prometheus/prometheus | 26 | 26 | 26 | **0** |
| testcontainers-go | 4 | 4 | 4 | **0** |

**0 false positives across 104 hand-checked findings.** Every diagnostic
pointed at code that really does what the check says it does. Full triage
notes live in [`docs/corpus/`](docs/corpus/).

Some favorites the corpus run surfaced:

- a Grafana test sleeping through an async DB insert, annotated by its own
  author with `// TOFIX: this is a hack`;
- an `httptest.Server` leaked inside a table-driven loop — in a file that
  correctly calls `defer server.Close()` in three other places;
- the self-aware ten-second sleep in Kubernetes quoted above.

## The checks

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
[`testing/synctest`](https://go.dev/blog/synctest) (Go 1.24+), which makes
such tests deterministic *and* instant, or to plain channel/WaitGroup
synchronization.

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

`t.Parallel()` plus a package-level variable write is a data race that only
manifests under particular schedules — the definition of a flake. The write
is flagged whether or not a mutex guards it: two parallel tests mutating
shared state generally break each other's assumptions either way.

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

**Autofix** (`-fix`): rewrites `log.Fatal*` → `t.Fatal*`.

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
    go install github.com/malikov73/flakylint/cmd/flakylint@latest
    flakylint ./...
```

flakylint only ever reports on `_test.go` files, so it is safe to run over
your whole module. Exit code is non-zero when findings are reported —
CI-gate friendly.

## Design philosophy

**When in doubt, stay silent.** A linter earns trust by being right, not by
being loud. Every check ships with explicit silence heuristics (escaping
servers, polling loops, synctest bubbles, goroutine literals, canonical
`TestMain` patterns), and each heuristic is locked in by regression tests.
The 0-FP corpus result above is this philosophy, measured.

flakylint deliberately does **not** try to detect flakiness at runtime.
Retries, quarantine and scoring are a different job, done well by other
tools — flakylint prevents the patterns those tools would later catch.

## Roadmap

- map-iteration-order dependent assertions
- unclosed `resp.Body` in tests
- timeout contexts in parallel subtests
- goleak coverage check
- minimum-duration threshold option for `sleepassert`
- golangci-lint integration

## Contributing

Issues and PRs welcome. Each analyzer lives in its own package under
[`analyzers/`](analyzers/) with `analysistest`-based tests — a new check is
one package plus a registration line in
[`cmd/flakylint/main.go`](cmd/flakylint/main.go).

## License

[MIT](LICENSE)
