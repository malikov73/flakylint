# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased] — v0.2.0

### Added

- Three new checks, enabled by default:
  - `hardport` — hardcoded TCP/UDP ports in test listeners.
  - `maporder` — assertions that depend on map-iteration order.
  - `eventuallyeffect` — count-dependent side effects inside polling callbacks.
- `version` command: `flakylint version` (also `--version` / `-version`) prints
  the build version, commit, and date, falling back to
  `runtime/debug.ReadBuildInfo` when built without release ldflags.

### Changed

- Autofix safety: suggested fixes are no longer offered when they would corrupt
  the source. `httptestclose` keeps the diagnostic but drops the fix inside
  `if`/`for`/`switch` statement initializers; `exitfatal` suppresses its fix when
  the testing receiver name is shadowed at the call site.
- Precision narrowing from corpus evaluation and an external audit:
  `hardport` validates the network argument and port range and no longer flags
  `http.Server{Addr:}` literals; `eventuallyeffect` ignores channels declared
  inside the callback; `maporder` is now source-order aware and ignores
  per-iteration accumulators used only as testify message arguments.

### Upgrade notes

- The new default-enabled checks may surface findings in downstream CI that
  previous versions did not report. Pin `@v0.2.0` in CI installs (rather than
  `@latest`) so a future release does not change results unexpectedly.

## [0.1.1] — 2026-07-17

### Added

- `//nolint` inline suppression, honored by the flakylint binary.

### Changed

- `parallelglobal` reports one diagnostic per test and variable instead of one
  per write site.
- `exitfatal` messages use the real testing parameter name.

### Fixed

- Documentation corrections and per-check corpus/usage notes.

## [0.1.0] — 2026-07-17

### Added

- Initial release with four checks: `httptestclose`, `sleepassert`,
  `parallelglobal`, and `exitfatal`.
- Corpus-validated across real-world Go repositories.
- CI, goreleaser release tooling, and README.

[Unreleased]: https://github.com/malikov73/flakylint/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/malikov73/flakylint/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/malikov73/flakylint/releases/tag/v0.1.0
