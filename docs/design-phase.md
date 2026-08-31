# AURoscope design status

## Current phase

The minimal LLM-first architecture was accepted by Mathieu in [decision issue #19](https://git.2027a.net/2027a/auroscope/issues/19) with an exact standalone [`GO DECISION`](https://git.2027a.net/2027a/auroscope/issues/19#issuecomment-1314) on 2026-08-30.

Production implementation was authorized separately by Forgejo issue #24 on branch `execution/issue-24-auroscope-v1`. The issue #24 lane implements the accepted minimal v1 and verifies it with deterministic fake-Paru/fake-Codex tests plus a guarded disposable-Arch E2E script.

Before that authorization, the next permitted work was:

1. a narrow disposable-Arch spike proving the Paru worktree/audit/edit/final-build path;
2. a new short implementation plan grounded in that evidence;
3. separate explicit authorization before production implementation.

## Accepted product frame

AURoscope has one purpose: audit exact AUR recipe changes with Codex CLI before Paru builds them, present the result, and collect the user's per-package decision.

- Official packages remain native Paru/Pacman operations and are never audited.
- Search, selection, resolution, build, and installation remain Paru responsibilities.
- Codex CLI is the only v1 LLM backend and is not optional in an AUR audit.
- AURoscope keeps only minimal audit/baseline state and a final recipe-identity guard.
- The local machine and account are trusted.

See [`architecture/minimal-v1.md`](architecture/minimal-v1.md), [`specification.md`](specification.md), and [`ADR-0015`](decisions/0015-minimal-llm-first-wrapper.md).

## Required spike

The new plan must not guess the central Paru integration. In disposable Arch, prove:

1. native search/selection and AUR origin identification;
2. the exact AUR worktree is available before package-supplied code;
3. an edited recipe can be re-audited and the same worktree reaches final build, or `edit` is explicitly deferred;
4. `PreBuildCommand` sees the exact worktree before build;
5. skipped AUR targets can be excluded and Paru performs the final fresh resolution;
6. official repository operations remain transparent and audit-free.

## Decision protocol

Consequential design issues begin with `MODE: DECISION`. Ordinary comments continue discussion only. Mathieu accepts the latest proposal by posting a standalone comment exactly equal to `GO DECISION`.

Finalization may update ADRs and design/specification documents, but does not authorize an implementation plan or production code. Executable follow-up uses a separate issue whose first non-empty line is `MODE: EXECUTION`.

## Historical material

The previous architecture made the LLM optional and introduced a deterministic scanner, broad state model, durable approval protocol, closure comparison, and extensive recovery machinery. It is superseded and archived under [`archive/pre-llm-first/`](archive/pre-llm-first/). Historical documents are evidence only, not instructions.
