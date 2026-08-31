# ADR-0003: Multi-stage Paru orchestration without resolver replacement

## Status

Superseded by [ADR-0019](0019-minimal-llm-first-wrapper.md) on 2026-08-30. The original decision was accepted on 2026-08-30; its human-menu compatibility clause was later amended by [ADR-0002](0002-paru-native-selection-compatibility.md).

Decision issue: [#3](https://git.2027a.net/2027a/auroscope/issues/3). Mathieu authorized this exact decision with [`GO DECISION`](https://git.2027a.net/2027a/auroscope/issues/3#issuecomment-1119).

## Context

AURoscope must preserve Paru as the selector, dependency resolver, AUR builder, and Pacman frontend while ensuring that the exact AUR recipes are reviewed and approved before executable package work begins.

A single Paru invocation with all review logic in `PreBuildCommand` would stay close to normal Paru behavior, but the hook runs too late to prepare the complete review and durable decision state cleanly. It also leaves ambiguous handling for Paru's later review path and cached package artifacts.

Replacing or linking Paru/libalpm internals could expose a richer typed plan, but would turn AURoscope into another package-manager frontend, tightly couple it to Paru and libalpm internals, and violate the product boundary.

The grounded Paru investigation also found that no stable JSON transaction-plan API exists and that the machine-oriented `--order` grammar has already diverged between documentation and post-release code (`INSTALL` versus `REPO`). Released Paru 2.1.0 additionally cannot cleanly separate the interactive human menu from selected targets until the post-release `d1dfbc4` fix.

## Decision

Adopt multi-stage Paru orchestration:

1. Paru performs native selection and produces a plan through a versioned, contract-tested machine-oriented adapter.
2. AURoscope records the plan identity, collects and inspects the exact recipes, and presents the evidence for human review before execution.
3. After approval, Paru resolves again and remains responsible for dependency resolution, building, and installation.
4. A transaction-scoped guard compares the current execution plan with the approved plan. Any drift in versions, origins, dependencies, or recipe identities stops execution and returns the transaction to review.
5. Adapters reject unknown or incompatible records. AURoscope does not parse Paru's human-facing menu except through ADR-0002's isolated, temporary, fail-closed Paru 2.1.0 selection adapter, and never replaces Paru's resolver.

The exact plan schema, approval handoff, hook timing, upgrade phasing, and compatibility floor remain governed by their dedicated design decisions. This ADR fixes the orchestration boundary, not those separate contracts.

## Alternatives considered

### Single Paru invocation with `PreBuildCommand` audit

- **Advantage:** minimal orchestration and behavior closest to bare Paru.
- **Rejected because:** review and state preparation occur at the wrong boundary, and recipe/cache drift cannot be represented cleanly as a complete pre-execution decision.

### Reimplement or link Paru/libalpm internals

- **Advantage:** richer typed planning data.
- **Rejected because:** excessive complexity and coupling; it would make AURoscope a competing resolver/frontend.

## Consequences

Positive:

- the user reviews and approves before executable AUR recipe work;
- Paru retains selection, resolution, build, and installation semantics;
- planning evidence and human decisions have a clear durable boundary;
- drift is handled fail-closed instead of being guessed or silently accepted.

Negative:

- planning and execution perform separate resolver runs;
- every supported Paru version needs a contract-tested adapter;
- changed plans require another review cycle;
- production search selection temporarily carries a bounded human-output parser risk on Paru 2.1.0; the durable path still requires a supported stable Paru build exposing clean machine output.

## Unresolved risks

- Paru has no stable JSON transaction-plan API.
- Machine-oriented text records can change across releases or diverge from their manuals.
- The execution plan may legitimately change between the two resolver runs; AURoscope must explain the drift without attempting to repair it silently.
- The exact transaction guard and anti-replay protocol are separate consequential decisions.

## Evidence

- [`docs/architecture/design-proposals.md`](../architecture/design-proposals.md), especially the Paru orchestration options and disposable spikes.
- [`docs/dependency-notes/paru-contract.md`](../dependency-notes/paru-contract.md), grounded in Paru `v2.1.0`, post-release commit `9ac3578`, and selection-stream fix `d1dfbc4`.
- Paru `v2.1.0` manuals and source cited by those documents.
- Accepted restatement in [issue comment 1109](https://git.2027a.net/2027a/auroscope/issues/3#issuecomment-1109).
