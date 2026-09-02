# ADR-0015: Minimal LLM-first Paru wrapper

## Status

Accepted on 2026-08-30 through [decision issue #19](https://git.2027a.net/2027a/auroscope/issues/19), authorized by Mathieu's exact standalone [`GO DECISION`](https://git.2027a.net/2027a/auroscope/issues/19#issuecomment-1314) comment.

## Context

AURoscope's previous design treated deterministic scanning as the core and LLM analysis as optional. It then accumulated a multi-stage closure protocol, broad SQLite lifecycle model, durable one-shot approvals, extensive recovery and retention, multiple inference backends, and many package boundaries.

That architecture no longer matched the product. AURoscope exists to add one LLM audit and one human decision between Paru's selection of an AUR recipe and Paru's normal build/install path.

## Decision

Build v1 as a minimal LLM-first wrapper:

1. Official repository updates and installs remain native Paru/Pacman operations with no audit.
2. Search and numbered selection remain Paru-native.
3. Every AUR package base in Paru's resolved build set—explicit target or dependency—enters AURoscope's audit path; official package dependencies never do.
4. Codex CLI is the single mandatory v1 LLM backend. There is no HTTP backend, automatic fallback, disabled audit mode, or competing deterministic security scanner.
5. AURoscope compares each exact recipe worktree with the last successfully approved/built commit, or audits the full recipe on first use.
6. The user chooses `approve`, `inspect`, `edit` and re-audit, `skip`, or `cancel` per package base.
7. Final dependency resolution, `makepkg`, Pacman installation, and native confirmation remain Paru responsibilities.
8. A minimal `PreBuildCommand` verifies `pkgbase`, commit, and manifest digest before build.
9. SQLite stores only successful baselines and audit history.
10. Implementation begins with one `internal/app` package. New abstractions require demonstrated current need.

## Required proof before planning

A disposable-Arch spike must prove the supported Paru surface for exact worktree acquisition, edit and re-audit of that same worktree, pre-build guard timing, skip/exclusion followed by fresh Paru resolution, and audit-free official package behavior.

If Paru cannot build the exact edited-and-re-audited worktree without AURoscope taking over its build role, planning is blocked and the architecture returns to Mathieu for a new decision. `edit` is not silently deferred from v1.

## Consequences

Positive:

- architecture matches the single product purpose;
- Paru/Pacman remain authoritative;
- the user sees an LLM audit before every AUR build;
- official packages remain frictionless;
- implementation and test scope are small enough for personal software.

Negative:

- v1 depends on Codex CLI availability and a supported output contract;
- only one Paru compatibility surface is supported initially;
- a skipped package may make Paru reject the remaining transaction;
- the same-UID race after the guard remains accepted;
- edit may be deferred if the exact worktree path is not supported.

## Superseded decisions

This ADR supersedes the active requirements of:

- ADR-0002 — broad selection compatibility and transitional parser;
- ADR-0003 — exhaustive multi-stage orchestration and closure comparison;
- ADR-0006 — eleven-package Go architecture;
- ADR-0008 — preselected TOML/shellwords dependencies;
- ADR-0009 — broad instrumental state model;
- ADR-0010 — durable one-shot approval lifecycle;
- ADR-0011 — deterministic scanner plus optional/multiple LLM backends;
- ADR-0012 — complex cleanup, backup, and retention policy;
- ADR-0013 — scanner-oriented v1 proof matrix;
- ADR-0014 — broad fail-closed classifier boundary.

ADR-0001 (Go/package), ADR-0004 (official update first), ADR-0005 (final recipe identity guard), and ADR-0007 (SQLite driver choice) remain compatible, subject to this ADR's reduced scope.

## Rejected alternatives

- Keep the prior architecture and merely reorder implementation: rejected because the wrong components remain.
- Replace Paru/libalpm: rejected because AURoscope is not a package manager.
- Add HTTP and Codex simultaneously: rejected because one personal v1 backend is enough.
- Remove final identity verification: rejected because Paru must build the recipe that Codex and the user reviewed.
