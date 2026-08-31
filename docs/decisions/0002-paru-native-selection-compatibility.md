# ADR-0002: Paru native-selection compatibility boundary

## Status

Superseded by [ADR-0019](0019-minimal-llm-first-wrapper.md) on 2026-08-30.

Accepted by Mathieu through [`GO DECISION`](https://git.2027a.net/2027a/auroscope/issues/2#issuecomment-1120) on 2026-08-30.

## Context

AURoscope must preserve Paru's native numbered search-selection experience while recovering the selected package names. Released Paru `v2.1.0` writes both its human menu and selected names to stdout, so it does not provide a clean machine stream. Commit [`d1dfbc48895a2a3b3fadc14569c3f2ffb5112b2e`](https://github.com/Morganamilo/paru/commit/d1dfbc48895a2a3b3fadc14569c3f2ffb5112b2e) fixes that redirection: on the verified post-fix build, the menu is on stderr and selected names alone are on stdout.

The fix was merged upstream and remains in `master`, but no stable release containing it existed when this decision was accepted, and there was no reliable public release schedule. Requiring the future stable release immediately would preserve a clean contract but could block production search selection indefinitely. Treating a `paru-git` commit as the production minimum would instead make an unpublished moving build part of the support boundary.

## Options considered

### A — Require the first stable release containing `d1dfbc4`

This gives AURoscope a clean, stable stream contract and avoids parsing human UI, but blocks production native search selection until an unannounced release exists.

### B — Support a pinned `paru-git` commit in production

This makes the fixed stream available immediately, but ties production support to an unpublished build and increases compatibility and packaging risk.

### C — Parse the Paru 2.1.0 human menu

This supports the current stable release, but the format is human-facing, localized, and inherently more fragile than a machine stream.

### Accepted variant — A with a strictly bounded transitional C

This keeps A as the durable boundary while permitting a temporary, fail-closed adapter for the verified 2.1.0 behavior.

## Decision

- The durable production compatibility target is the first stable Paru release containing `d1dfbc4`.
- Until such a release exists and passes AURoscope's executable capability tests, Paru `2.1.0` may be supported by an isolated transitional adapter that parses only the verified selection behavior.
- The transitional adapter must be explicitly identified as temporary, narrowly accept the tested format, reject ambiguous or divergent output, and fail without producing selected targets when its contract does not hold.
- Capability is determined by executable behavior, not by `paru --version` alone. The supported-version matrix must test stream separation, selected and cancelled exit behavior, output bounds, and selected-name validation.
- A pinned post-fix upstream commit may be used for design, development, and contract testing, but is not a production minimum.
- The transitional 2.1.0 adapter is removed once a stable release containing `d1dfbc4` passes the capability matrix.

This decision applies to native search-selection capture. Other Paru planning, guard, cache, and execution contracts retain their independently defined compatibility gates.

## Consequences

Positive:

- users can retain native search selection on current Paru 2.1.0 while the stable release cadence remains uncertain;
- the long-term contract stays on separated terminal and machine streams;
- an upstream commit is not silently promoted to a production dependency;
- capability tests catch patched builds or future regressions that a version string cannot distinguish.

Negative:

- AURoscope temporarily owns a parser for human-facing Paru output;
- localization or formatting changes can make 2.1.0 selection unsupported even when its version string is unchanged;
- two selection adapters must coexist until the transitional one is retired.

## Residual risks and required proof

- The exact transitional grammar, locale policy, output limits, and ambiguity rejection need executable fixtures before implementation is accepted.
- A future stable release may change streams or exit codes again; every supported Paru build remains subject to the contract matrix.
- The unusual verified exit code `1` may count as selection success only in the exact selection subprocess when bounded, valid selected names were recovered and the child was not terminated by a signal.

## Evidence

- Paru `v2.1.0`, commit `70f66dc9eddb40e264ee6c9197541262b7792c9c`.
- Selection-stream fix `d1dfbc48895a2a3b3fadc14569c3f2ffb5112b2e`.
- Verified post-fix reference commit `9ac3578807a87858651e81a02586ceb947686e7c`.
- Project evidence: [`docs/dependency-notes/paru-contract.md`](../dependency-notes/paru-contract.md).
- Accepted proposal and research: [issue #2](https://git.2027a.net/2027a/auroscope/issues/2), especially [the final proposal](https://git.2027a.net/2027a/auroscope/issues/2#issuecomment-1113).
