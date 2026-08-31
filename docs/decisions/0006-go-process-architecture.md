# ADR-0006: Go process architecture

## Status

Superseded by [ADR-0019](0019-minimal-llm-first-wrapper.md) on 2026-08-30.

Accepted on 2026-08-30 through [decision D5](https://git.2027a.net/2027a/auroscope/issues/6), authorized by Mathieu's exact [`GO DECISION`](https://git.2027a.net/2027a/auroscope/issues/6#issuecomment-1123) comment.

## Context

AURoscope must preserve Paru-compatible passthrough behavior while also supporting intercepted planning and review flows. Its process layer must preserve raw arguments, terminal behavior, child exit information, cancellation, and cleanup without introducing abstractions that obscure those contracts.

The design investigation established that ordinary passthrough can inherit the caller's file descriptors and that the supported Paru selection contract needs terminal input and diagnostics while capturing only selected package names. A CLI framework could normalize or reject unknown Paru arguments. A PTY used for every child would combine human UI and machine-readable output. A plugin or multi-process framework has no demonstrated v1 use.

## Decision

AURoscope is one Go executable with explicit domain packages:

```text
cmd/auroscope
internal/cli
internal/config
internal/process
internal/paru
internal/recipe
internal/scanner
internal/llm
internal/review
internal/store
internal/approval
internal/report
```

These are domain boundaries, not a requirement to define an interface for every package. Interfaces are introduced only at nondeterministic boundaries that tests need to replace: clock and randomness, process execution, model transport, and store transactions.

The CLI and process contracts are:

- use the Go standard library rather than a CLI framework;
- classify only AURoscope-owned commands and the minimum Paru/Pacman grammar needed to distinguish passthrough from intercepted flows;
- preserve the original argument vector unchanged for transparent passthrough and do not normalize unknown Paru options;
- execute subprocesses from argument vectors without an implicit shell;
- inherit stdin, stdout, and stderr for transparent commands;
- for Paru native selection, inherit stdin and stderr while capturing stdout as the machine-oriented selection stream;
- create and manage child process groups explicitly;
- on SIGINT, SIGTERM, or SIGHUP, propagate cancellation to the child process group, wait for the child, retain its exit code and terminating signal separately, then invalidate pending approvals and clean transaction work;
- add no PTY dependency initially. A PTY may be introduced only if an executable contract test against a supported Paru/Pacman version proves inherited descriptors insufficient for a required interaction.

This decision selects architecture and process boundaries only. It does not select the SQLite driver or other direct Go dependencies, and it does not authorize production implementation.

## Consequences

Positive:

- Paru compatibility is not subordinated to a framework's parser;
- human-facing terminal streams remain separate from captured machine data;
- process, signal, and cleanup behavior stays explicit and testable;
- the initial dependency surface remains small;
- the architecture can grow at demonstrated domain boundaries without a speculative plugin system.

Negative or deferred:

- AURoscope must maintain a narrow argument classifier and generated help for its own commands;
- process-group and signal behavior is platform-specific and requires real Linux integration tests;
- preserving both wrapper categories and exact child exit/signal data requires explicit result types;
- if a supported dependency later requires a controlling terminal, PTY handling will need a separate justified dependency and contract.

## Rejected alternatives

### CLI framework

Rejected for the initial architecture because convenience features do not justify the risk of consuming, rejecting, or rewriting Paru arguments that AURoscope does not own.

### PTY for every child

Rejected because it would multiplex terminal UI with machine-readable output and make the selection stream more fragile. This does not forbid a narrowly proven PTY requirement later.

### Microservices or plugin framework

Rejected because v1 has one local orchestration workflow and no second concrete use that justifies distributed process boundaries or a generic extension API.

## Verification obligations

Implementation must prove the decision with:

- classifier tests showing transparent argument vectors are byte-for-byte unchanged;
- supported-version contract tests for inherited terminal descriptors and Paru selection stream separation;
- real process-group tests for SIGINT, SIGTERM, and SIGHUP;
- tests preserving child exit code and terminating signal independently;
- cancellation tests showing the child is awaited before approval invalidation and cleanup;
- a failing executable contract test before any PTY dependency is proposed.

## Evidence

- [`docs/architecture/design-proposals.md`](../architecture/design-proposals.md), sections 3, 9, and 10: package boundaries, CLI/process alternatives, signal handling, tests, and unresolved compatibility risks.
- Go [`os/exec`](https://pkg.go.dev/os/exec): commands do not invoke a shell implicitly.
- Go [`os/signal`](https://pkg.go.dev/os/signal): explicit signal notification and handling primitives.
- [Decision D5 discussion](https://git.2027a.net/2027a/auroscope/issues/6), especially the final concrete proposal immediately preceding authorization.
