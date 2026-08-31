# ADR-0008: Minimal direct Go dependencies beyond SQLite

## Status

Superseded by [ADR-0019](0019-minimal-llm-first-wrapper.md) on 2026-08-30.

## Context

AURoscope needs readable configuration and safe parsing of conventional `$VISUAL`/`$EDITOR` values while keeping its direct dependency surface small and auditable. A zero-dependency policy would either replace TOML with a poorer configuration format or require bespoke command-line parsing. Broad CLI, configuration, migration, logging, PTY, or LLM frameworks would add abstractions and supply-chain surface without a demonstrated need.

Mathieu accepted the latest concrete D7 proposal in issue [#8](https://git.2027a.net/2027a/auroscope/issues/8): the focused TOML and shell-word modules, standard-library implementations for the remaining needs, and no framework or PTY dependency without evidence.

## Decision

Beyond the separately selected SQLite driver, AURoscope will have these direct Go dependencies:

- `github.com/pelletier/go-toml/v2` for `config.toml`. The implementation must use strict decoding so unknown keys fail validation. Configured commands are represented as argv arrays. The exact module version will be reviewed and pinned when implementation begins rather than being selected implicitly by the Go toolchain.
- `github.com/mattn/go-shellwords v1.0.14` only to split fallback `$VISUAL` and `$EDITOR` values containing arguments. The parser must explicitly set environment expansion and backtick expansion to false, must not invoke a shell, and must append the report path as a separate argv element after parsing.

AURoscope will not initially add direct dependencies for:

- migrations, which use ordered SQL embedded with `go:embed` and are applied transactionally;
- LLM response validation, which uses typed `encoding/json`, `DisallowUnknownFields`, explicit bounds, and semantic validation;
- LLM backends, which use a narrow standard-library OpenAI-compatible HTTP adapter and optional standard-library Codex process invocation under [ADR-0011](0011-deterministic-scanner-and-llm-contracts.md);
- logging, which uses concise stderr diagnostics plus structured SQLite state;
- PTY handling, unless an executable dependency contract proves inherited descriptors insufficient.

No CLI, configuration, migration, logging, or LLM framework is accepted. Codex CLI is an optional external executable backend, not a Go module or framework dependency. Any future direct dependency requires a concrete need; a structurally consequential addition requires its own ADR.

## Options considered

### Zero additional dependencies

Use JSON configuration and either reject editor arguments or implement shell-word parsing locally.

This minimizes modules but degrades configuration usability or creates a fragile parser for a security-sensitive process boundary.

### Focused dependencies

Use one TOML decoder and one shell-word parser, with the standard library for the remaining contracts.

This adds two narrow, ordinary modules while avoiding bespoke parsing and preserving explicit control over process execution. This option is accepted.

### Frameworks

Adopt broad CLI, configuration, migration, logging, or LLM frameworks.

This could reduce some initial wiring, but adds transitive code, conventions, and behavior that the design does not need. This option is rejected.

## Consequences

Positive:

- readable TOML with strict typo detection;
- conventional editor environment values without `sh -c` or custom parsing;
- a small, reviewable direct dependency set;
- explicit standard-library contracts for migrations, JSON validation, HTTP, and logging.

Negative or deferred:

- the exact `go-toml/v2` version still requires review and pinning at implementation time;
- `go-shellwords` is accepted at the old but current `v1.0.14` tag and must be treated as a narrowly scoped parser, not a command executor;
- more local validation and wiring is required than with frameworks;
- a PTY dependency may still become necessary if an executable contract demonstrates a real controlling-terminal requirement.

## Evidence

- Accepted proposal: issue [#8](https://git.2027a.net/2027a/auroscope/issues/8), especially the concrete restatement in [comment 1108](https://git.2027a.net/2027a/auroscope/issues/8#issuecomment-1108).
- Authorization: Mathieu's exact `GO DECISION` in [comment 1125](https://git.2027a.net/2027a/auroscope/issues/8#issuecomment-1125).
- `go-toml/v2` `v2.4.3` (`071a36c2a57244f2e70369bfc69889fda2a1f60f`) exposes `Decoder.DisallowUnknownFields`; its strict-decoder tests passed under `go test ./...` during decision finalization. This version is evidence for the API, not the implementation pin.
- `go-shellwords` `v1.0.14` (`2b31b13123b0c5253ca007689e2e11c1512e9624`) exposes per-parser `ParseEnv` and `ParseBacktick` controls, both false by default; its test suite passed under `go test ./...` during decision finalization.
- Project evidence is distilled in [`docs/dependency-notes/go-direct-dependencies.md`](../dependency-notes/go-direct-dependencies.md).
