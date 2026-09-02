# Minimal LLM-first v1 architecture

## Product boundary

AURoscope is a transparent Paru wrapper with one intervention: before Paru builds an AUR recipe, Codex CLI audits the exact change and the user decides whether that package base proceeds.

```text
Paru/Pacman official phase (no audit)
              ↓
Paru selects/resolves AUR package bases
              ↓
AURoscope builds exact recipe diffs
              ↓
Codex CLI audit
              ↓
user decision per pkgbase
              ↓
Paru fresh resolution + makepkg + Pacman
```

## Minimal components

Start with `cmd/auroscope` and one `internal/app` package containing straightforward files for Paru invocation, recipe acquisition/diff, Codex, review, state, and the final guard.

There is no separate resolver, rule engine, backend abstraction, approval service, event log, plugin system, or background process.

## Native Paru/Pacman behavior

- System update: run the complete official repository update first with native Paru/Pacman streams, prompts, and status. Do not inspect official packages.
- Search: preserve Paru's interactive search and numbered selection.
- Explicit/mixed install: audit only selected AUR package bases; official targets remain in the native final transaction.
- Other non-building Paru operations pass through without audit.
- Final execution always returns to Paru. AURoscope never replaces `makepkg` or Pacman.

## Audit

For each exact AUR worktree in Paru's resolved build set, including AUR dependencies, compare against the last successfully approved/built commit. First use sends the complete recipe. Build a bounded bundle of the diff, needed files, untrusted `.SRCINFO`, minimal metadata, commit, and manifest digest. Official dependencies are never audited.

Codex CLI is the single v1 backend. It runs outside the recipe worktree and returns validated structured JSON. If Codex fails, the user can retry, skip, or cancel; there is no silent bypass.

## Decision and edit

The menu is `approve | inspect | edit+reaudit | skip | cancel`. The Paru spike must prove that the edited and re-audited worktree is exactly the worktree Paru builds. If it cannot, planning is blocked and the architecture returns to Mathieu for a new decision; `edit` is not silently deferred.

## Final guard

A private transaction file lists approved `pkgbase`, commit, and manifest digests. `PreBuildCommand` checks the actual Paru worktree. Unexpected or changed input aborts and returns to audit.

The guard addresses ordinary recipe drift. It does not attempt to defend against the same Unix user or a compromised host.

## State

SQLite keeps last successful baselines and audit history only. The baseline advances after successful Paru completion. No durable approval lifecycle, process table, artifact reuse, automatic backup, or complex retention is part of v1.

## Complexity gate

A new abstraction or dependency enters v1 only when a failing test or the supported-version Paru/Codex spike demonstrates the need. “Future extensibility” is not sufficient.

## First proof

Before the implementation plan, run one disposable-Arch spike proving the exact Paru worktree lifecycle, edit/re-audit path, pre-build guard, skip/exclusion behavior, final native Paru execution, and audit-free official package path.
