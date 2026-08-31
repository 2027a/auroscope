# ADR-0009: Keep the SQLite state model strictly instrumental

## Status

Superseded by [ADR-0019](0019-minimal-llm-first-wrapper.md) on 2026-08-30.

Accepted by Mathieu in [decision issue #9](https://git.2027a.net/2027a/auroscope/issues/9) after the concrete C-minimal proposal in [comment 1112](https://git.2027a.net/2027a/auroscope/issues/9#issuecomment-1112) and the explicit `GO DECISION` in [comment 1207](https://git.2027a.net/2027a/auroscope/issues/9#issuecomment-1207).

## Context

AURoscope needs SQLite to preserve exact recipe identity, inspection evidence, explicit human authority, readable status, and enough execution history to recover safely after a crash. A pure event log would make ordinary status queries and cross-row invariants needlessly complex. Mutable current-state tables alone would lose the history needed to prove what was inspected, decided, built, and installed.

Mathieu selected the mixed model while explicitly rejecting scope creep. The v1 database must be an instrumental product model, not a generic event store or speculative persistence framework.

## Decision

Adopt normalized immutable evidence and decision records plus only the mutable lifecycle rows required by the v1 workflow.

A table or column enters v1 only when it directly supports at least one of these concrete needs:

- exact identity or the anti-TOCTOU boundary;
- inspection evidence;
- an explicit human decision;
- a readable product status;
- crash recovery.

It must also have a concrete product read or a testable invariant. Data without either is deferred.

The minimal v1 table groups are:

- migrations: `schema_migrations`;
- recipe identity: `package_bases`, `recipe_identities`, `recipe_files`;
- inspection and decision: `inspections`, `deterministic_findings`, `human_decisions`;
- execution and guard: `transactions`, `transaction_args`, `transaction_recipes`, `approvals`, `process_sessions`;
- outcome evidence: `builds`, `package_artifacts`, `install_outcomes`.

`llm_assessments` is introduced only by the forward migration that delivers LLM assessment. The same rule applies to report-retention data or any later feature: persistence arrives with a real workflow, query, and invariant rather than in advance.

The required v1 invariants are limited to:

1. foreign keys are enabled, and the identity, inspection, decision, and approval chain remains coherent;
2. an approval can be armed only from a human `approve` decision for the same inspection and therefore the same recipe identity;
3. `armed → claimed` is atomic and one-shot, and lifecycle transitions are monotonic;
4. no package artifact is reusable without a successful build tied to the exact recipe identity and recorded artifact hash;
5. recipe paths, process arguments, and working directories retain reversible byte representations; escaped text is display-only.

The schema grows through forward migrations only when an implemented feature requires it.

## Explicitly deferred

- a generic event table, global event-sourcing, or replay framework;
- a generic provenance graph, entity-attribute-value storage, or plugin schema;
- distributed orchestration or authoritative SQLite state on a network filesystem;
- v1 down migrations;
- speculative fields without an identified query or invariant;
- the LLM assessment table before the LLM assessment feature exists.

## Consequences

Positive:

- status and recovery remain direct relational queries;
- immutable evidence and decisions retain an audit trail without event-sourcing machinery;
- critical approval and artifact-reuse rules can be enforced and tested at the database boundary;
- the initial schema stays bounded by actual v1 behavior.

Negative or deferred:

- new features require explicit forward migrations rather than preallocated generic storage;
- some cross-table invariants need carefully reviewed composite keys, constraints, triggers, or store-level transactional checks;
- exact column definitions, SQL encoding of each invariant, and crash/concurrency behavior still require migration and integration tests in the implementation lane.

## Evidence and unresolved risks

The accepted proposal is the scope-narrowed state model in [comment 1112](https://git.2027a.net/2027a/auroscope/issues/9#issuecomment-1112). The options and accepted boundary are synchronized in [`docs/architecture/design-proposals.md`](../architecture/design-proposals.md); operational tuning and retention recommendations there remain separately identified proposals rather than speculative v1 schema.

The implementation must prove foreign-key enforcement, atomic one-shot claims under concurrency, monotonic transitions, byte-safe round trips, artifact-reuse rejection, migration safety, and recovery after interrupted lifecycle changes. This ADR does not authorize production code or an implementation plan.
