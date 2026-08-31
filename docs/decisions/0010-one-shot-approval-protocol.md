# ADR-0010: One-shot approval handoff and binding

## Status

Superseded by [ADR-0019](0019-minimal-llm-first-wrapper.md) on 2026-08-30.

## Context

AURoscope must prove that Paru is about to execute the exact recipe and transaction the user approved. A hash of `PKGBUILD` alone is too narrow: a recipe can depend on adjacent tracked files, file types and modes matter, and a local workspace can diverge or be substituted without changing that one file.

The handoff to Paru must also resist accidental or stale replay without pretending to protect against a fully compromised process running as the same Unix user. The guard boundary and its residual post-guard same-UID race are defined separately by [ADR-0005](0005-recipe-identity-guard-boundary.md).

## Options considered

### A. Bearer token in the environment

Pass a secret approval token through Paru's environment.

- Advantage: simple handoff.
- Costs: subprocesses inherit the token; secret handling adds exposure without providing useful protection against the same-UID adversary.

### B. Secret one-shot file

Place a secret token in a private transaction file consumed by the guard.

- Advantage: limits some accidental reuse.
- Costs: adds secret creation, transport, cleanup, and failure paths while still not defeating a same-UID attacker.

### C. Non-secret transaction ID bound in SQLite

Pass only a validated transaction ID and make the authoritative approval a one-shot SQLite state transition bound to the approved recipe, inspection, process, and workspace.

- Advantages: no bearer secret; explicit replay prevention; transactional lifecycle and durable audit evidence; fail-closed process and workspace checks.
- Costs: requires careful SQLite transactions, `/proc` process identity checks, complete no-follow manifest collection, and cleanup of every terminal state.

## Decision

Choose option C.

An approval binds all of the following:

- transaction ID;
- source kind, source namespace, and `pkgbase`;
- Git commit OID and tree OID;
- canonical complete recipe manifest digest and every relevant file's path bytes, object type, mode, size, content hash, and Git blob OID;
- the inspection and its scanner contract version;
- the append-only human `approve` decision;
- workspace device and inode;
- owner UID and the expected Paru process identity;
- expiry.

The commit identifies the candidate; it is not a risk score. `PKGBUILD` is part of the identity but is not sufficient by itself. A single `recipe_digest` may summarize the canonical manifest in the user interface, while the complete manifest remains available for explanation and verification.

The wrapper creates one `armed` approval per approved transaction recipe. The default lifetime is 30 minutes and the configurable maximum is 2 hours. It generates a private transaction-specific Paru configuration whose fixed hook command is equivalent to:

```text
exec /usr/bin/auroscope guard --transaction <id>
```

The transaction ID is non-secret, lowercase hexadecimal, and no package-derived text enters the command. The wrapper records the child Paru PID, start time, executable identity, and UID. The guard must verify that exact live parent process, the expected source/namespace/`pkgbase`, and the workspace device/inode.

Before claiming approval, the guard computes the complete recipe identity without following symlinks and requires it to match the approved identity. In a short `BEGIN IMMEDIATE` transaction it atomically changes exactly one matching unexpired approval from `armed` to `claimed`; missing, duplicate, expired, unexpected, or mismatched records fail closed. It then recomputes the complete no-follow manifest and requires equality with both the approved digest and the pre-claim digest before succeeding.

A successful exact build changes `claimed` to `consumed`. Any mismatch invalidates the approval. Cancellation, failure, signal, process mismatch, or any other terminal transaction state invalidates every remaining `armed` or `claimed` approval. Startup recovery expires stale approvals and invalidates claims whose recorded process session is no longer the same live process. A claimed or consumed approval can never authorize a later Paru process or transaction.

An inspection with status `partial` or `failed` may still receive an explicit human approval only after its limitations and errors are shown and recorded. Failure is neither an autonomous veto nor silent approval.

## Consequences

- Approval is an auditable one-shot capability represented by database state, not by possession of a secret string.
- Replay attempts, stale transactions, changed recipes, substituted workspaces, and unexpected package bases fail closed.
- SQLite transition tests must cover contention, duplicate claims, expiry, cancellation, crash recovery, and all invalidation paths.
- Integration tests must mutate every bound identity component before and around the claim and verify rejection before recipe-supplied code executes.
- The process contract depends on trustworthy PID start-time and executable identity checks; unavailable or contradictory process evidence fails closed.
- Cached package reuse remains governed by exact artifact hashes tied to a successful build of the same recipe identity; recipe approval alone does not approve an arbitrary package artifact.

## Evidence

- Accepted proposal: issue [#10](https://git.2027a.net/2027a/auroscope/issues/10), clarified in [comment 1118](https://git.2027a.net/2027a/auroscope/issues/10#issuecomment-1118).
- Authorization: the exact standalone `GO DECISION` in [comment 1145](https://git.2027a.net/2027a/auroscope/issues/10#issuecomment-1145), authored by Mathieu.
- The detailed protocol and schema relationships are recorded in [`docs/architecture/design-proposals.md`](../architecture/design-proposals.md).
- The final pre-recipe-execution timing boundary is accepted separately in [ADR-0005](0005-recipe-identity-guard-boundary.md).

## Unresolved risks

- Another process running as the same UID can still race the worktree after the guard returns. Eliminating that residual race requires a stronger isolation boundary.
- A compromised user account can inspect or modify the user's process and database state; this protocol does not claim to defend against it.
- Exact descriptor-relative manifest algorithms, process-identity portability, and crash-injection cases still require executable implementation-lane tests.
