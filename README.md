# flakylint

Static analysis for flaky Go tests. flakylint detects the known patterns
that make tests flaky — **before** they start flaking in CI.

Runtime flaky-test tools (retries, quarantine, dashboards) tell you a test
*became* flaky. flakylint is the other half: it flags the code patterns that
cause flakiness at review time, as a standard `go/analysis` linter. Use both.

## Checks

| Check | What it catches | Real-world incident |
|---|---|---|
| `httptestclose` | `httptest.NewServer` never closed — leaked port and goroutines; the GC finalizer races the HTTP connection pool | [google/go-github#4210](https://github.com/google/go-github/pull/4210) |
| `sleepassert` | `time.Sleep` in a test body to wait for concurrent work — races the scheduler, flakes under CI load; suggests `testing/synctest` (Go 1.24+) | [Go blog: synctest](https://go.dev/blog/synctest) |
| `parallelglobal` | writes to package-level variables in tests marked `t.Parallel()` — data race that only shows under parallel scheduling | — |
| `exitfatal` | `os.Exit` / `log.Fatal` in tests — kills the test binary, skips `t.Cleanup` and defers, poisons later tests; autofix `log.Fatal*` → `t.Fatal*` | — |

Design philosophy: **when in doubt, stay silent.** Every check is
conservative — e.g. `sleepassert` ignores sleeps in polling loops, and
`httptestclose` ignores servers that escape the function.

## Install

    go install github.com/malikov73/flakylint/cmd/flakylint@latest

Or download a binary from the releases page.

## Usage

    flakylint ./...

As a `go vet` tool:

    go vet -vettool=$(which flakylint) ./...

Disable an individual check:

    flakylint -sleepassert=false ./...

Checks with suggested fixes (`httptestclose`, `exitfatal`) apply them with
`-fix`.

## Roadmap

- map-iteration-order dependent assertions
- unclosed `resp.Body` in tests
- timeout contexts in parallel subtests
- goleak coverage check
- golangci-lint integration

## License

MIT
