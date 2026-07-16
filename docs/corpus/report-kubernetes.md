# flakylint false-positive corpus evaluation — kubernetes/kubernetes

- Repo: `kubernetes/kubernetes` @ `92bf0c19` (shallow clone, depth 1, HEAD as of 2026-07-16)
- Invocation: `GOFLAGS=-mod=vendor GOTOOLCHAIN=auto flakylint <pattern>` (vendored deps, all 4 analyzers enabled by default)
- Trees scanned: `./pkg/...`, `./cmd/...`, `./plugin/...`, `./test/...`
- Trees skipped: `./staging/...` (per instructions).
- No driver/typecheck errors observed in any of the four runs. Exit code 3 ("findings reported") on `pkg`, `cmd`, `test`; exit code 0 (clean) on `plugin`.

## Per-analyzer counts (raw, all 4 trees combined)

| Analyzer | Message prefix | Count |
|---|---|---|
| sleepassert | `time.Sleep synchronizes the test on real time...` | 127 |
| httptestclose | `httptest server is never closed...` | 3 |
| parallelglobal | `parallel test writes package-level variable...` | 0 |
| exitfatal | `os.Exit inside a test` / `log.Fatal* inside a test` / `...in TestMain skips` | 0 |
| **Total** | | **130** |

`parallelglobal` and `exitfatal` found zero matches across `pkg/cmd/plugin/test` — not a tooling failure (no driver errors), the codebase's test suite simply doesn't (in the scanned trees) exhibit `t.Parallel()` tests writing package globals, or `os.Exit`/`log.Fatal*` calls inside test bodies/TestMain-with-defers.

## Triage

Total findings (130) exceeds the 60-finding full-triage threshold, so triage was a random sample of up to 25 per analyzer (seeded, `random.seed(42)` over the full ordered finding list):

- **httptestclose**: 3 findings total → triaged all 3 (below the 25 cap).
- **sleepassert**: 127 findings → triaged a random sample of 25 (~20%).
- **parallelglobal / exitfatal**: 0 findings → nothing to triage.

### Results

| Analyzer | Triaged | TP-valuable | TP-noisy | FP |
|---|---|---|---|---|
| httptestclose | 3 | 2 | 1 | 0 |
| sleepassert | 25 | 16 | 9 | 0 |
| **Total** | **28** | **18** | **10** | **0** |

**FP rate among triaged findings: 0 / 28 = 0%.**

Every triaged finding was a correct application of the analyzer's documented rule: every flagged `time.Sleep` sat directly in a test/subtest body (not inside a polling `for`/`range` loop, not inside a `synctest` bubble, not `time.Sleep(0)`), and every flagged `httptest.NewServer`/`NewTLSServer` genuinely had no `.Close()` call and did not escape its enclosing function. No instance of "sleep inside a retry loop" or "server closed via defer/Cleanup the tool missed" was found in the sample.

The valuable/noisy split reflects real-world severity, not tool correctness: sleeps guarding a *negative* assertion ("verify nothing happened") with a generous safety margin (5x, 10-15s) were graded TP-noisy — a maintainer would shrug, since a scheduling delay there causes a false pass, not a flake. Sleeps racing a *positive* assertion against async controller/goroutine completion with a tight or CI-load-sensitive margin (500ms-2s) were graded TP-valuable — a scheduling delay there causes an outright false test failure, exactly the flake pattern flakylint targets.

## Most compelling TP-valuable findings (launch-post / upstream-PR material)

### 1. Explicitly self-documented as best-effort/unreliable

`test/integration/apiserver/admissionwebhook/broken_webhook_test.go:71`

```go
// There is no guarantee on how long it takes the apiserver to honor the configuration and there is
// no API to determine if the configuration is being honored, so we will just wait 10s, which is long enough
// in most cases.
time.Sleep(10 * time.Second)

// test whether the webhook blocks requests
t.Logf("Attempt to create Deployment which should fail due to the webhook")
```
The comment is a maintainer admission that this *can* flake ("long enough in most cases"). This is the single strongest case in the sample — flakylint is flagging exactly the pattern the surrounding comment already confesses to.

### 2. Maintainer already has a TODO to fix this exact pattern

`pkg/controller/endpointslice/endpointslice_controller_test.go:1429` (flagged sleeps at :1416 and later in the same function)

```go
// TestPodUpdatesBatching verifies that endpoint updates caused by pod updates are batched together.
// This test uses real time.Sleep, as there is no easy way to mock time in endpoints controller now.
// TODO(mborsz): Migrate this test to mock clock when possible.
func TestPodUpdatesBatching(t *testing.T) {
	t.Parallel()
	...
	time.Sleep(add.delay)
	...
	time.Sleep(tc.finalDelay)
```
flakylint independently rediscovered a known-flaky pattern that already has an open TODO against it.

### 3. Tight timing margin racing a controller's own reconcile loop

`pkg/controller/volume/attachdetach/reconciler/reconciler_test.go:709`

```go
// Mock NodeStatusUpdate fail
rc.(*reconciler).nodeStatusUpdater = statusupdater.NewFakeNodeStatusUpdater(true /* returnError */)
reconciliationLoopFunc(ctx)
// The first detach will be triggered after at least 50ms (maxWaitForUnmountDuration in test).
time.Sleep(100 * time.Millisecond)
reconciliationLoopFunc(ctx)
```
Only a 2x margin (100ms sleep vs. a documented 50ms trigger threshold) — CPU contention in CI is a realistic way to blow this margin and assert on stale state.

### 4. `httptest.Server` leaked across a large, expensive apiserver test file

`pkg/controlplane/instance_test.go:295` and `:352`

```go
func TestAPIVersionOfDiscoveryEndpoints(t *testing.T) {
	apiserver, etcdserver, _, assert := newInstance(t)
	defer etcdserver.Terminate(t)

	server := httptest.NewServer(apiserver.ControlPlane.GenericAPIServer.Handler.GoRestfulContainer.ServeMux)
	// ... server.URL used repeatedly, server.Close() never called ...
```
`pkg/controlplane` builds and tears down a real (heavyweight) apiserver instance per test; two of its tests bind an additional `httptest.Server` and never release it. In a package this size, leaked listeners/goroutines are a plausible contributor to the kind of port-exhaustion flakes k8s CI has historically chased.

### 5. Sleep-choreographed goroutine ordering in a concurrency test

`pkg/volume/util/nestedpendingoperations/nestedpendingoperations_test.go:738-754`

```go
opZContinueCh <- true
time.Sleep(delay)
op2ContinueCh <- true
time.Sleep(delay)
op1ContinueCh <- true
time.Sleep(delay)
...
time.Sleep(delay)
err4 := grm.Run(mainVolumeName, "" /* podName */, node2, volumetypes.GeneratedOperations{OperationFunc: operation4})
```
The test relies on `delay` being long enough for goroutine scheduling to land operations in a specific order — a textbook flakiness pattern in a package whose entire purpose is testing concurrency semantics.

## Coverage gaps

- `./staging/...` skipped entirely (by design/instruction — not attributable to a tool limitation).
- No `-mod=vendor` typecheck failures or driver errors surfaced in any of the four scanned trees; coverage of `pkg/cmd/plugin/test` is effectively complete for those trees.
- `plugin/...` produced zero findings (exit 0) — small tree, plausible true negative, not investigated further.
