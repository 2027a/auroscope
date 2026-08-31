# ADR-0014: Reject local PKGBUILD inputs in v1

## Status

Superseded by [ADR-0019](0019-minimal-llm-first-wrapper.md) on 2026-08-30.

Decision issue: [#14](https://git.2027a.net/2027a/auroscope/issues/14). Mathieu authorized the final proposal with [`GO DECISION`](https://git.2027a.net/2027a/auroscope/issues/14#issuecomment-1173).

## Context

AURoscope v1 is intended to inspect AUR recipes. Paru also supports local and configured PKGBUILD sources through operations such as `-B`, targetless `-U`, path-like sync targets, and PKGBUILD repositories.

Those inputs cannot safely enter the ordinary AURoscope guard flow. Paru 2.1.0 may invoke `makepkg --printsrcinfo` while loading or constructing a local PKGBUILD repository, before `PreBuildCommand`. Makepkg sources the PKGBUILD even for `--printsrcinfo`, so hostile recipe code can execute before AURoscope verifies the reviewed identity. Paru also handles sync targets beginning with `./` before the normal mode flow.

A warning-only pass-through would imply protection that AURoscope cannot provide. Supporting local recipes safely now would require a separate collection, resolution, or isolation boundary and would materially expand v1 beyond its AUR-only purpose.

## Decision

AURoscope v1 supports inspection of AUR recipes only. Before starting any Paru process for an intercepted flow, it must reject as unsupported:

- `-B` local build operations;
- targetless `-U` operations;
- local or path-like targets, including `./`, `../`, absolute paths, and `file:` targets;
- any user mode containing `pkgbuilds` or its short selector `p`;
- any unexpected PKGBUILD-repository record returned by a supported planning adapter.

The diagnostic must state clearly that local PKGBUILD inputs are outside v1 scope. `-U` with explicit package archive files or URLs remains the separately classified package-archive pass-through; it does not gain recipe inspection through this decision.

For supported intercepted flows, AURoscope must neutralize config-provided PKGBUILD-repository modes with final trusted mode-reset flags:

- repository-only intent: `--repo`;
- AUR-only intent, including `-Sua`: `--aur`;
- combined/default repository and AUR intent: `--repo --mode=aur`.

AURoscope must fail closed if classification cannot prove that the operation stays within a supported boundary. Safe local-recipe support is deferred to a separate future design decision if a concrete need arises.

## Alternatives considered

### Pass through with a warning

- **Advantage:** preserves more of Paru's command surface.
- **Rejected because:** local PKGBUILD code may execute before the guard, making the wrapper's safety boundary misleading.

### Design safe local-recipe support for v1

- **Advantage:** covers advanced Paru workflows and ad hoc recipes.
- **Rejected because:** it requires a materially broader parser, resolver, sandbox, or proxy boundary for a use case outside the accepted v1 purpose.

## Consequences

Positive:

- no local recipe can silently cross the executable boundary before AURoscope's guard;
- the v1 promise remains narrow and honest: AUR recipe inspection only;
- configured `PkgbuildsOnly` and PKGBUILD repositories cannot leak into supported intercepted flows through Paru mode merging.

Negative:

- AURoscope v1 is not a transparent replacement for Paru's local-build and PKGBUILD-repository features;
- users must invoke another deliberate workflow for local recipes and receive no AURoscope protection there;
- argument and planning adapters must recognize these inputs and fail before launching Paru.

## Unresolved risks

- Future Paru versions may change argument handling or mode-reset semantics; supported versions still require executable capability tests.
- Package archives installed through explicit `-U` are outside recipe inspection and need a separate artifact-inspection decision if that scope is added.
- Supporting local recipes safely remains unresolved rather than implicitly solved by this rejection boundary.

## Evidence

- [`docs/architecture/design-proposals.md`](../architecture/design-proposals.md), argument classification and planning sections.
- [`docs/dependency-notes/paru-contract.md`](../dependency-notes/paru-contract.md), verified against Paru `v2.1.0` and post-release commit `9ac3578`.
- [`docs/dependency-notes/pacman-makepkg-contract.md`](../dependency-notes/pacman-makepkg-contract.md), verified makepkg PKGBUILD-sourcing behavior.
- The accepted clarification in [issue comment 1115](https://git.2027a.net/2027a/auroscope/issues/14#issuecomment-1115).
