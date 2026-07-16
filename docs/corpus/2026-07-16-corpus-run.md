# Corpus run — 2026-07-16

flakylint @ `ebd5145` run against four large open-source Go codebases to measure
the false-positive rate before public launch. Launch gate per the design spec:
FP < 5% among triaged findings.

## Method

Shallow clones, full typecheck via the go/analysis driver. Every finding (or a
random sample of 25 per analyzer where a repo produced more than 60) was
triaged by reading the flagged code in context and classified:

- **TP-valuable** — rule correctly applied, genuine flake risk a maintainer
  would accept a fix for
- **TP-noisy** — rule correctly applied per its documented semantics, but a
  reasonable maintainer would shrug
- **FP** — the analyzer misapplied its own documented rule

## Results

| Repo | Findings | Triaged | TP-valuable | TP-noisy | FP | FP rate |
|---|---|---|---|---|---|---|
| kubernetes/kubernetes | 130 | 28 | 18 | 10 | 0 | 0% |
| grafana/grafana | 104 | 46 | 21 | 25 | 0 | 0% |
| prometheus/prometheus | 26 | 26 | 9 | 17 | 0 | 0% |
| testcontainers/testcontainers-go | 4 | 4 | 4 | 0 | 0 | 0% |
| **Total** | **264** | **104** | **52** | **52** | **0** | **0%** |

**FP rate: 0/104 = 0%.** Launch gate passed.

Per-analyzer across the corpus: sleepassert 239, httptestclose 24,
exitfatal 1, parallelglobal 0.

## Highlight findings (launch-post / upstream-PR material)

- **kubernetes** `test/integration/apiserver/admissionwebhook/broken_webhook_test.go:71` —
  `time.Sleep(10*time.Second)` whose own preceding comment admits "there is no
  guarantee... we will just wait 10s, which is long enough in most cases".
  flakylint independently rediscovered a maintainer-confessed flaky pattern.
- **grafana** `pkg/tests/web/index_view_test.go:187` — `time.Sleep(1*time.Second)`
  with the developer's own comment `// TOFIX: this is a hack` masking a wait on
  an async DB insert.
- **grafana** `pkg/services/ngalert/remote/alertmanager_test.go:234` — httptest
  server leaked in a table-driven loop while the same file correctly does
  `defer server.Close()` in three other places.
- **grafana** `pkg/registry/apis/iam/team_hooks_test.go:495` — `require.Fail`
  inside an async callback guarded only by a 50ms sleep — real "Fail in
  goroutine after test completed" panic risk, repeated 4× in the file.
- **prometheus** `storage/remote/write_handler_test.go:1697` — httptest server
  in a table-driven test never closed; leaks a port per subtest.
- **testcontainers-go** `logconsumer_test.go:308,318,334,338`
  (`TestContainerLogWithErrClosed`) — four fixed 1–3s sleeps letting async
  docker log streaming settle before exact-count asserts; not in any polling
  loop. Prime upstream-PR candidate.

## Caveats

- **Noise profile:** 50% of triaged findings are TP-noisy, almost all from
  sleepassert — the rule is honest but chatty on integration-test suites.
  Worth stating plainly in the README ("sleepassert is advisory") and/or
  considering a minimum-duration threshold before the golangci-lint submission.
- **parallelglobal fired 0 times corpus-wide** — inconclusive (no evidence of
  FPs, but also no evidence of the rule earning its keep on real code yet).
- Coverage gaps: kubernetes `staging/` skipped (vendored duplicates);
  grafana build-tag-gated (`integration`/`enterprise`) test files not compiled;
  grafana's go.work submodules required per-directory runs;
  testcontainers `modules/azurite` untested (pre-existing broken go.sum upstream).
- Per-repo details: `report-kubernetes.md`, `report-prometheus.md`,
  `report-testcontainers.md` in this directory (grafana's full breakdown lives
  in the table above; its raw outputs were session-scratch only).
