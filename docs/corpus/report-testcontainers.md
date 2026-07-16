# flakylint corpus evaluation: testcontainers/testcontainers-go

## Coverage
- Fresh shallow clone (depth 1) in workspace; local dev clone untouched.
- Root module: full run, exit 3 (findings).
- All 85 modules/* sub-modules: go mod download + analyzer run each. 84/85 built and analyzed cleanly (0 findings). 1/85 (azurite) failed to build (pre-existing go.sum gap, unrelated to flakylint).
- examples/nginx (only real example dir): analyzed, 0 findings. Not skipped.
- Raw output saved to corpus/raw-testcontainers.txt.

## Per-analyzer counts
- sleepassert: 4
- httptestclose: 0
- parallelglobal: 0
- exitfatal: 0
- Total: 4 (<=60, all triaged, no sampling)

## Triage (4/4)
- TP-valuable: 4
- TP-noisy: 0
- FP: 0
- FP rate among triaged: 0%

## Best finding
logconsumer_test.go:308,318,334,338 (root pkg) in TestContainerLogWithErrClosed: four fixed time.Sleep(1-3s) calls used to let async docker log streaming "settle" before asserting exact log-message counts via require.Equalf. Not inside any polling loop. Genuine flake risk under CI load; a bounded poll on consumer.Msgs() would be the fix. Same anti-pattern repeated 4x in one function -> good single-PR candidate, and notable since the flakylint author contributes to this repo.

```go
time.Sleep(time.Second * 1)
existingLogs := len(consumer.Msgs())
...
hitNginx()
time.Sleep(time.Second * 1)
msgs := consumer.Msgs()
require.Equalf(t, 1, len(msgs)-existingLogs, "logConsumer should have 1 new log message, instead has: %v", msgs[existingLogs:])
```

## True-negative sanity checks (analyzer precision evidence)
- modules/dex/dex_test.go:208 time.Sleep(50ms) inside a polling for-loop -> correctly not flagged.
- modules/dex/examples_test.go log.Fatalf x6 inside ExampleXxx funcs (no *testing.T) -> correctly not flagged by exitfatal (confirmed against analyzers/exitfatal/exitfatal.go: only fires in TestFunc/TestMain/subtest literals).
- 0 httptest.NewServer in any modules/*_test.go -> httptestclose silence expected.
- 10 files use t.Parallel(); none flagged by parallelglobal; spot check found no package-level var writes (not exhaustively re-verified per site).

## Coverage gaps
1. modules/azurite: `go build ./...` itself fails (missing go.sum entries for golang.org/x/sys/unix via moby/go-archive+moby/term+gopsutil, and golang.org/x/crypto/ssh via root module's port_forwarding.go) - pre-existing depth-1 clone go.sum inconsistency, not a flakylint driver error. Left unmodified (read-only constraint); azurite untested by any analyzer.
2. Only sleepassert fired anywhere in ~86 modules - small sample for judging httptestclose/parallelglobal/exitfatal precision on this repo specifically; absence here reflects this codebase's test style (context/wait-strategy container readiness, few raw httptest servers, no package-level test globals), not necessarily analyzer weakness.
3. Analysis was per-module-boundary (each modules/* has its own go.mod); cross-module effects beyond per-module `go mod download` resolution weren't exercised.
