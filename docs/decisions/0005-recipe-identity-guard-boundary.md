# ADR-0005: Recipe identity guard boundary

## Status

Accepted on 2026-08-30; amended by [ADR-0015](0015-minimal-llm-first-wrapper.md). The final exact recipe-identity check and `--skipreview` boundary remain active. The former durable approval/atomic-claim machinery is superseded.

## Context

AURoscope must ensure that the AUR recipe executed by Paru is exactly the recipe the user reviewed. The product specification previously said that Paru's `PreBuildCommand` verifies identity "immediately before each build".

That timing is not available in Paru 2.1. Paru downloads recipes, invokes every package-base `PreBuildCommand`, may perform other trusted orchestration such as installing official repository dependencies, and only then starts individual builds. Paru's own recipe review also normally occurs after `PreBuildCommand`.

The identity boundary covers the complete reviewed recipe manifest, not only `PKGBUILD`: commit and tree identity plus the relevant tracked files, modes, types, paths, and content hashes.

## Options considered

### A. Contract the actual pre-execution boundary

Treat the guard as the last complete recipe-identity verification after AURoscope review and before any recipe-supplied code is executed. Run Paru execution with `--skipreview` so Paru cannot edit the recipe after the guard.

- Advantages: matches verified Paru behavior; keeps the guard narrow; ensures no unaudited recipe code runs before verification.
- Costs: the check is not adjacent to each individual `makepkg` invocation; a different process running as the same Unix user can still mutate files after the guard returns.

### B. Require an upstream per-build Paru hook

- Advantage: preserves the literal "immediately before each build" wording.
- Costs: blocks production on an upstream change and still cannot eliminate mutation by a compromised same-UID process after the hook returns.

### C. Interpose an AURoscope `makepkg` proxy

- Advantage: permits another identity check at each `makepkg` invocation.
- Costs: creates a substantially broader process boundary around repeated source, prepare, build, VCS `pkgver()`, and possible chroot flows; complexity is disproportionate to the current threat model.

## Decision

Choose option A.

After AURoscope review and before the first execution of any code supplied by a recipe, the guard must recompute the complete identity of every planned recipe and require an exact match with the identity that was reviewed and approved. Any mismatch aborts the operation and returns the recipe to review.

The execution invocation must use `--skipreview`, because Paru's post-hook review could otherwise modify recipe content after verification. The contract does not claim that `PreBuildCommand` runs immediately before each individual build. Trusted Paru operations between the guard and `makepkg` are permitted provided they neither execute nor modify recipe content.

## Consequences

- The specification must describe a final pre-recipe-execution identity boundary rather than a per-build-adjacent hook.
- Tests must prove that changed recipe identity is rejected at the guard and that `--skipreview` removes Paru's known post-guard edit path.
- The guarantee covers hostile recipe substitution before recipe execution and accidental or concurrent drift observed by the guard.
- Mutation after the guard by another process with the same UID remains a documented residual race. Eliminating it would require a stronger isolation boundary and is outside this decision.
- This decision does not claim that a reviewed recipe makes upstream sources or built artifacts safe.

## Evidence

- Paru `v2.1.0`, commit `70f66dc9eddb40e264ee6c9197541262b7792c9c`: `man/paru.conf.5` and `src/install.rs` establish the `PreBuildCommand` ordering.
- Disposable Paru 2.1 integration spikes recorded in [`docs/dependency-notes/paru-contract.md`](../dependency-notes/paru-contract.md).
- Accepted proposal and authorization: [Forgejo issue #5](https://git.2027a.net/2027a/auroscope/issues/5), authorized by Mathieu's exact `GO DECISION` comment `#issuecomment-1122`.

## Unresolved risks

- A separate same-UID process can mutate the worktree after the guard returns.
- ADR-0015 settles the handoff with a private transaction file containing approved recipe identities and an exact manifest check. No durable or atomic approval-claim lifecycle is required; the remaining manifest details are grounded by the required Paru spike.
