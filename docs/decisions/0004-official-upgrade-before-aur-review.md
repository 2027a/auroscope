# ADR-0004: Complete official upgrade before AUR review

## Status

Accepted on 2026-08-30; amended by [ADR-0015](0015-minimal-llm-first-wrapper.md). The native official-update-first boundary remains active. AUR dependency closure and final execution are delegated to Paru under ADR-0015 rather than implemented by AURoscope.

Decision issue: [#4](https://git.2027a.net/2027a/auroscope/issues/4). Mathieu authorized the latest concrete proposal with an exact [`GO DECISION`](https://git.2027a.net/2027a/auroscope/issues/4#issuecomment-1200).

## Context

AURoscope must preserve supported complete Arch repository upgrades, allow selected AUR recipes to be deferred, and inspect AUR recipes before they are built. Arch does not support refreshing repository databases and then selectively withholding official upgrades.

The initial proposal pre-reviewed an AUR plan before the official upgrade, then repeated that planning and review against the updated system. That first AUR plan would necessarily be provisional: official package versions, providers, origins, and dependency closure can change during the complete repository upgrade. It adds work without removing the possibility that an already-installed AUR package is temporarily incompatible after official packages change.

Mathieu also clarified the product boundary: AURoscope audits AUR recipes only. It must not add a second review layer over Paru/Pacman's normal official repository transaction.

## Decision

For a normal system-upgrade flow, use two ordered phases:

1. Run the complete official repository upgrade with native Paru/Pacman behavior (`paru -Syu --repo` or a contract-tested equivalent). AURoscope adds no audit or approval step to this phase, excludes no official package, and preserves Paru/Pacman's terminal interaction and status.
2. After the official phase succeeds, ask Paru for the AUR package bases in its resolved build set.
3. Audit each AUR package base and collect the user's per-package decision.
4. Relaunch Paru with approved AUR targets and explicit skipped targets. Paru performs the fresh authoritative resolution, builds and installs what remains, or rejects the transaction if the remaining targets cannot be resolved.

There is no provisional AUR review before the official upgrade. “Inspect before mutation” applies to the AUR phase before any AUR recipe is built, not globally before the native official transaction.

If the official phase fails or is cancelled, AURoscope preserves that result and does not proceed to AUR planning or execution. It never withholds an official repository package to accommodate an AUR package.

## Alternatives considered

### Combined repository and AUR upgrade with injected exclusions

- **Advantage:** resembles one transaction.
- **Rejected because:** refresh and exclusion behavior makes the supported complete official-upgrade boundary weak and risks creating or prolonging an unsupported partial upgrade.

### Preliminary AUR review, complete official upgrade, then AUR re-review

- **Advantage:** identifies possible AUR consequences before any package mutation.
- **Rejected because:** the first plan is provisional and must be recomputed after the official upgrade; it duplicates review without eliminating the intermediate compatibility risk.

### Complete official upgrade, then AUR review

- **Advantages:** simplest honest sequence; official packages remain a complete native Paru/Pacman transaction; AUR review uses the real post-upgrade state; AUR work remains independently deferrable.
- **Accepted cost:** an already-installed AUR package can be temporarily incompatible between completion of the official phase and completion or deferral of the AUR phase.

## Consequences

Positive:

- official repository upgrades are complete and are not fragmented for AUR policy;
- AURoscope does not duplicate Paru/Pacman's review or confirmation for official packages;
- AUR planning and approval are based on the current post-upgrade state rather than a knowingly provisional plan;
- user authority remains intact before any AUR recipe is built.

Negative:

- the upgrade is visibly phased rather than one combined transaction;
- a failed or deferred AUR phase can leave an installed AUR package temporarily incompatible with updated official libraries;
- Paru may reject the final AUR transaction when skipped targets make the remaining request unsatisfiable; AURoscope reports Paru's result without computing its own dependency closure.

## Unresolved risks

- The one supported Paru query/worktree/exclusion surface must be proven by the disposable-Arch spike required by ADR-0015.
- AUR compatibility with newly upgraded official libraries cannot be guaranteed by recipe review alone.

## Evidence

- Pacman 7.1 documents `-Syu` as refresh plus complete system upgrade; the verified project finding is recorded in [`docs/dependency-notes/pacman-makepkg-contract.md`](../dependency-notes/pacman-makepkg-contract.md).
- Paru/Pacman command and planning evidence is recorded in [`docs/architecture/design-proposals.md`](../architecture/design-proposals.md) and [`docs/dependency-notes/paru-contract.md`](../dependency-notes/paru-contract.md).
- Accepted concrete restatement: [issue comment 1146](https://git.2027a.net/2027a/auroscope/issues/4#issuecomment-1146).
