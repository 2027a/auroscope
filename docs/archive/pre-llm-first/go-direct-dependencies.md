> **SUPERSEDED BY ADR-0015 — HISTORICAL EVIDENCE ONLY. DO NOT USE AS AN ACTIVE DEPENDENCY DECISION.**

# Dependency note — Direct Go dependencies for configuration and editor argv

## Context

ADR-0008 accepts two narrow direct dependencies beyond SQLite: `github.com/pelletier/go-toml/v2` for strict TOML configuration and `github.com/mattn/go-shellwords v1.0.14` for fallback `$VISUAL`/`$EDITOR` argument splitting.

## Verified findings

### `github.com/pelletier/go-toml/v2`

The API was inspected at tag `v2.4.3`, commit `071a36c2a57244f2e70369bfc69889fda2a1f60f`. `Decoder.DisallowUnknownFields()` sets strict decoding and returns an error when a TOML key does not match a non-ignored struct field. The repository contains strict-decoder tests covering unknown keys.

The accepted dependency is the v2 module; `v2.4.3` is evidence for the required API, not the implementation pin. Review and pin the exact version when implementation begins.

### `github.com/mattn/go-shellwords`

Tag `v1.0.14`, commit `2b31b13123b0c5253ca007689e2e11c1512e9624`, exposes `Parser.ParseEnv` and `Parser.ParseBacktick`. Both package defaults are false, and `NewParser()` copies those package-level values into the parser.

AURoscope must explicitly set both parser fields to false rather than rely on mutable package defaults. The module is only an argv splitter: do not use its command-execution behavior, do not invoke `sh -c`, and append the report path as a separate argv element after parsing.

## Source consulted

- `go-toml/v2` `v2.4.3`: `unmarshaler.go`, `unmarshaler_test.go`, `README.md`, `go.mod`
- `go-shellwords` `v1.0.14`: `shellwords.go`, `shellwords_test.go`, `README.md`, `go.mod`

Exact source was cloned temporarily under `/tmp/hermes-dependency-context/auroscope-d7/`; no dependency source is vendored into AURoscope.

## Verification

- `go test ./...` in the `go-toml/v2` `v2.4.3` source: all packages passed.
- `go test ./...` in the `go-shellwords` `v1.0.14` source: passed.

## Reuse rule

Use strict TOML decoding for AURoscope configuration, and treat editor environment values as argv data with environment and backtick expansion explicitly disabled.
