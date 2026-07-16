# flakylint false-positive corpus evaluation: prometheus/prometheus

- Target: `github.com/prometheus/prometheus` (shallow clone, commit `57a1157` - "Merge pull request #18242 from mmorel-35/gocritic-nestingReduce")
- Command: `flakylint ./...` from repo root, `GOTOOLCHAIN=auto`
- Exit code: 3 (findings present, no driver/build errors)
- Raw output: `raw-prometheus.txt` (26 lines, all findings - no analyzer crashes, no build-failure lines)
- Coverage: full `./...` run completed cleanly; `go mod download` succeeded with no errors. No packages skipped or failed to build.

## Per-analyzer counts

| Analyzer | Count |
|---|---|
| httptestclose | 1 |
| sleepassert | 25 |
| parallelglobal | 0 |
| exitfatal | 0 |
| **Total** | **26** |

Since total findings (26) is <= 60, **all 26 findings were triaged** (no sampling needed).

## Triage results

| Class | Count |
|---|---|
| TP-valuable | 9 |
| TP-noisy | 17 |
| FP | 0 |
| **Triaged total** | **26** |

**FP rate among triaged: 0 / 26 = 0%.** Every finding was a correct application of the analyzer's documented rule.

### Breakdown

**httptestclose (1 total, 1 triaged)**
- `storage/remote/write_handler_test.go:1697` - TP-valuable. `srv := httptest.NewServer(handler)` inside a table-driven `t.Run` subtest with no `defer srv.Close()`, no `t.Cleanup`, and no `Close()` call anywhere else in the function (verified via grep). Every subtest run leaks a listening port and server goroutine.

**sleepassert (25 total, 25 triaged) - 8 TP-valuable, 17 TP-noisy**

TP-valuable (genuine flake risk, fixed-duration sleeps racing background work with no polling):
- `discovery/manager_test.go:1720` - 200ms sleep waiting for `discoveryManager.Run()` goroutine to start before a shutdown-race test.
- `rules/manager_test.go:1162,1170,1238,1244` - 4 fixed sleeps (3-5s) waiting for rule-manager evaluation cycles / staleness-marker propagation; tight coupling to real wall-clock scheduling under CI load.
- `storage/remote/azuread/azuread_test.go:416` - 4s sleep waiting for a background token-refresh goroutine, followed by `AssertNumberOfCalls(t, "GetToken", 2)`, an exact-count assertion tied to real-time scheduling with only a 1s margin against a 5s token expiry.
- `tsdb/db_append_v2_test.go:2650,2743` and `tsdb/db_test.go:4247,4340` - identical anti-pattern duplicated in 4 places: `time.Sleep(time.Second)` then `require.False(t, compactionComplete.Load(), ...)`, asserting a background goroutine has *not* finished - the classic "absence of an event within a timeout is not proof" flaky pattern.

TP-noisy (correct application, low practical value - mostly inherent to testing time-based features or already covered by other synchronization):
- `discovery/file/file_test.go:370,428,449` - 1s sleeps for negative-outcome checks ("verify nothing happened"); idiomatic in this codebase's `defaultWait`-based test helper, generous margin.
- `storage/remote/queue_manager_test.go:2056,2058,2060,2066` - sleeps of exactly `sampleAgeLimit` multiples, inherent to testing a real-time-based sample-age-expiry feature.
- `tsdb/chunks/chunk_write_queue_test.go:162` - 10ms sleep + `require.False(addedJob.Load())`; matches the sleep-assert pattern but low real flake risk since the queue-full block is deterministic.
- `util/logging/dedupe_test.go:50` - tests an actual time-based log-dedupe feature; only inequality (`Len==2`) asserted with generous 2x margin.
- `util/notifications/notifications_test.go:80,143,184` - sleeps before `unsubscribe()`/close, but correctness is already guaranteed downstream by `wg.Wait()` after channel close (buffered-channel drain-before-close semantics), making the sleeps effectively redundant rather than load-bearing.
- `util/stats/stats_test.go:35,50,76` - tests actual `Timer`/duration instrumentation; assertions are `Greater`/`GreaterOrEqual`, not tight equality, so no real upper-bound flake risk.

## 3-5 most compelling TP-valuable findings

### 1. `storage/remote/write_handler_test.go:1697` (httptestclose)
```go
srv := httptest.NewServer(handler)

// Send message and do the parse response flow.
c := &Client{Client: srv.Client(), urlString: srv.URL, timeout: 5 * time.Minute, writeProtoMsg: tt.msgType}

stats, err := c.Store(t.Context(), tt.payload, 0)
require.NoError(t, err)
```
No `defer srv.Close()` anywhere in the enclosing `t.Run` closure or the outer test function - verified no other `Close()` call touches `srv`. Runs once per table-driven case, leaking a port + goroutine each time.

### 2. `tsdb/db_test.go:4247` and 3 near-identical copies (sleepassert)
```go
go func() {
    defer compactionComplete.Store(true)
    compactionErr <- db.CompactOOOHead(ctx)
}()

// Give CompactOOOHead time to start work.
// If it does not wait for the querier to be closed, then the query will return incorrect results or fail.
time.Sleep(time.Second)
require.False(t, compactionComplete.Load(), "compaction completed before reading chunks or closing querier")
```
Textbook flaky pattern: a fixed 1s sleep is used to "prove" a background goroutine hasn't finished. Under CI scheduling pressure this can spuriously pass without ever exercising the blocking behavior it's meant to verify, and it's duplicated 4x across `db_test.go`/`db_append_v2_test.go`.

### 3. `storage/remote/azuread/azuread_test.go:416` (sleepassert)
```go
// Token set to refresh at half of the expiry time. The test tokens are set to expiry in 5s.
// Hence, the 4 seconds wait to check if the token is refreshed.
time.Sleep(4 * time.Second)

require.NotEmpty(t, mustGetAccessToken(t, actualTokenProvider))
mockCred.AssertNumberOfCalls(t, "GetToken", 2)
```
An exact call-count assertion tied to a background refresh timer racing a fixed 4s sleep against a 5s expiry - a 1s margin that a loaded CI runner can plausibly eat.

### 4. `rules/manager_test.go:1162` (sleepassert)
```go
err := ruleManager.Update(2*time.Second, files, labels.EmptyLabels(), "", nil)
time.Sleep(4 * time.Second)
require.NoError(t, err)
...
time.Sleep(5 * time.Second)
require.Equal(t, 0, countStaleNaN(t, storage), "invalid count of staleness markers after stopping the engine")
```
Long fixed sleeps standing in for "wait for N evaluation cycles to complete" - no channel/poll signal, purely time-based.

## Coverage gaps

- `parallelglobal` and `exitfatal` fired zero findings on this module - either the analyzers' target patterns genuinely don't occur in prometheus/prometheus, or their heuristics are narrower than what this codebase's test style would trigger (e.g. parallel subtests writing shared state via mutex-guarded structs rather than bare package vars, and no direct `os.Exit`/`log.Fatal` calls inside `_test.go` files in this repo). Cannot distinguish "correctly silent" from "under-triggering" without a second corpus known to contain these patterns.
- Coverage was single-module/root only; prometheus has no nested Go modules requiring separate `go mod download` runs, so no partial-coverage caveat applies here.
