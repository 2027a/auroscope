# ADR-0007: Use mattn/go-sqlite3 with CGO for v1

## Status

Accepted on 2026-08-30; amended by [ADR-0019](0019-minimal-llm-first-wrapper.md). The `mattn/go-sqlite3 v1.14.50` CGO driver choice remains active. Broader backup, recovery, concurrency, and retention obligations are superseded unless the minimal state implementation demonstrates a concrete need.

Accepted by Mathieu in [decision issue #7](https://git.2027a.net/2027a/auroscope/issues/7) after the concrete proposal in [comment 1111](https://git.2027a.net/2027a/auroscope/issues/7#issuecomment-1111) and the explicit `GO DECISION` in [comment 1124](https://git.2027a.net/2027a/auroscope/issues/7#issuecomment-1124).

## Context

AURoscope needs an embedded SQLite driver for its authoritative state store. The initial release target is Arch Linux on `linux/amd64`, built in an Arch environment where a C toolchain is practical. The driver choice affects dependency count, binary size, build requirements, and cross-compilation.

A minimal spike created a database, enabled foreign keys and WAL, and verified a foreign-key constraint:

| Driver | Version | Stripped binary | Modules | Build/runtime result |
|---|---|---:|---:|---|
| `github.com/mattn/go-sqlite3` | `v1.14.50` | 3,608,224 bytes | 2 | CGO required; the `CGO_ENABLED=0` stub is not usable at runtime |
| `modernc.org/sqlite` | `v1.57.0` | 6,299,940 bytes | 26 | pure Go; cross-build succeeded; requires Go 1.25 or newer |

Calling the external SQLite CLI was also considered, but it would make transactions and error handling process-mediated and unnecessarily fragile.

## Decision

- Use `github.com/mattn/go-sqlite3 v1.14.50` for the Arch `linux/amd64` v1.
- Accept CGO and the C build toolchain as build-time requirements. Do not promise a pure-Go, static, or distribution-independent binary.
- Pin the driver version and SQLite compile options. Any option change that affects SQLite behavior must be explicit and covered by tests.
- Before release, test foreign-key enforcement, WAL, busy timeout, migrations, integrity/corruption handling, backup/restore, and concurrent access.
- Reconsider a pure-Go driver only when supported cross-target releases become a concrete requirement; do not pay its measured binary and dependency cost pre-emptively.
- Do not use the SQLite CLI as the application persistence interface.

## Consequences

Positive:

- the selected driver is mature and has a small direct dependency surface;
- the measured v1 binary is smaller than the tested pure-Go alternative;
- the decision fits the single supported Arch target without adding a large transitive module graph.

Negative:

- builds require CGO and a C compiler/toolchain;
- cross-compilation is more complicated and target-specific;
- `CGO_ENABLED=0` may still produce a binary, but that binary must never be treated as operational without a runtime database smoke test;
- expanding the supported platform matrix may require revisiting this ADR.

## Evidence and unresolved risks

The spike and options are summarized in [`docs/architecture/design-proposals.md`](../architecture/design-proposals.md) and [`docs/dependency-notes/sqlite-go-driver.md`](../dependency-notes/sqlite-go-driver.md).

The accepted driver does not by itself settle schema, migration, transaction, locking, recovery, or retention design. Those remain separate design decisions. SQLite compile options and the final Arch packaging build must be verified in the implementation lane; this ADR does not authorize production code or an implementation plan.
