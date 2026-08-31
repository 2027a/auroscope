# ADR-0013: AUR supply-chain threat model and v1 test gates

## Status

Superseded by [ADR-0019](0019-minimal-llm-first-wrapper.md) on 2026-08-30.

## Context

AURoscope exists to give the user understandable evidence about possible AUR supply-chain attacks before an AUR recipe is built or installed. An earlier proposal expanded that goal into local-host hardening: hostile same-UID processes, inode races, approval replay, SQLite corruption and recovery, Pacman concurrency, PATH/editor manipulation, cleanup escape, and sudo hardening.

That broader boundary would overstate the product and impose disproportionate security machinery. AURoscope is an advisory review tool. It is not a sandbox, antivirus, endpoint-protection system, or local security boundary, and it never replaces the user's decision.

## Options considered

### A. Broad local and supply-chain threat model

Treat AUR inputs, local processes, local state, configuration, process execution, and privilege boundaries as adversarial and make their dedicated security suites v1 gates.

- Advantages: stronger local-hardening ambitions and a wider catalogue of negative tests.
- Costs: substantial complexity outside the product's purpose; implies guarantees AURoscope cannot honestly provide on a compromised account or machine.

### B. AUR supply-chain threat model

Treat AUR recipes, their repository files, source/upstream material, metadata, and analysis input as hostile. Inspect them without execution, present attributable evidence and explicit analysis failures, and leave every consequential decision to the user. Trust the local machine, user account, and versioned package-management toolchain for this model.

- Advantages: directly matches the product purpose; keeps claims and tests understandable; preserves meaningful proof without turning AURoscope into a sandbox.
- Costs: local compromise and malicious local interference are explicitly not prevented; trusted dependency behavior still needs ordinary compatibility and regression tests.

### C. No dedicated security quality gate

Rely only on ordinary functional tests.

- Advantage: smallest initial test burden.
- Cost: does not prove that inspection avoids executing hostile package content or that essential advisory behavior remains intact.

## Decision

Choose option B.

The hostile boundary is limited to AUR supply-chain input: `PKGBUILD`, `.SRCINFO`, install files, patches, units, rules, other repository files, source/upstream material and metadata, and any of that content passed to deterministic or LLM analysis. These inputs are data, never instructions to AURoscope or its model.

AURoscope must collect and inspect recipe material without executing or sourcing a `PKGBUILD` or package source. It must emit understandable, attributable indicators, keep deterministic evidence separate from LLM assessment, make analysis errors and limitations visible, and never make an autonomous install decision.

The local machine, local user account, other local processes, local configuration and editors, and the trusted versioned Paru/Pacman/makepkg/Git toolchain are outside this adversary model. Their ordinary correctness, compatibility, state integrity, and error handling still require proportional functional tests, but those tests are not security claims against a compromised host.

The v1 security quality gates are deliberately limited to:

1. a corpus of benign and suspicious recipes covering every advertised deterministic rule or indicator;
2. executable proof that inspection executes neither a `PKGBUILD` nor package source material;
3. presentation tests proving readable alerts, indicator provenance, explicit analysis failures, separation of deterministic and LLM evidence, and no automatic decision;
4. one disposable Arch end-to-end path proving inspection, evidence presentation, and a human decision occur before installation, without touching the real workstation.

## Consequences

- Documentation and reports must say that AURoscope helps review AUR supply-chain risk; they must not label a package safe or claim local containment.
- Symlinks, unusual paths, hostile filenames, prompt injection, obfuscation, and malformed/oversized analysis content remain relevant when they originate in hostile AUR input. Handling them safely is part of non-executing, intelligible inspection, not a promise to withstand a hostile local process.
- D4's exact-recipe identity guard remains a product correctness boundary against recipe drift in the normal workflow. Same-UID races and inode-swap attacks are not v1 security gates under this ADR.
- Replay hardening, hostile-process races, SQLite corruption/backup, Pacman concurrency attacks, PATH/editor attacks, cleanup escape, and general sudo hardening are removed from the v1 threat-model gates. They may still receive ordinary functional tests where needed.
- Additional sandboxing or local-host hardening requires a separately justified decision; it is not implied by v1.

## Evidence

- Mathieu narrowed the goal to AUR supply-chain indicators and explicitly excluded protection of the local machine in [issue #13, comment 1091](https://git.2027a.net/2027a/auroscope/issues/13#issuecomment-1091).
- The accepted concrete option and four bounded gates are in [comment 1117](https://git.2027a.net/2027a/auroscope/issues/13#issuecomment-1117).
- Authorization is Mathieu's exact standalone `GO DECISION` in [comment 1168](https://git.2027a.net/2027a/auroscope/issues/13#issuecomment-1168).

## Unresolved risks

- A malicious package can evade the advertised indicators or appear benign; advisory evidence is not proof of safety.
- Upstream source and compiled behavior are not exhaustively analyzed in v1.
- A compromised local account or machine can tamper with AURoscope, its state, its display, or the package-management flow.
- The LLM may be wrong or manipulated by package content; bounded input, local output validation, and the no-decision contract constrain its role but do not make its advice authoritative. Codex exposure reduction is best effort under ADR-0011, not a sandbox guarantee.
