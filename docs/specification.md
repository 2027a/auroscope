# AURoscope v1 specification

## 1. Purpose

AURoscope is Mathieu's normal Paru entry point. It keeps official repository operations transparent and inserts one Codex CLI audit step only when Paru would build an AUR recipe.

The product is successful when the user can search, install, or update through the familiar Paru interface, inspect an LLM audit for each affected AUR package base, decide what proceeds, and let Paru/Pacman complete the real transaction.

## 2. Supported commands

```console
auroscope
auroscope <search terms>
auroscope -S <packages>
auroscope -Syu
auroscope <other Paru arguments>
```

- Bare `auroscope` performs the normal official update first, then audits pending AUR updates.
- Search terms use Paru's native interactive search and numbered selection.
- Explicit installs may select official packages, AUR packages, or both.
- Operations that cannot build an AUR recipe pass through with native terminal behavior and exit status.

## 3. Official packages are transparent

For a system update, AURoscope first runs the official repository phase through Paru/Pacman (`paru -Syu --repo` or the exact equivalent proven for the supported Paru version).

During this phase AURoscope performs:

- no Codex call;
- no recipe collection;
- no report;
- no additional confirmation;
- no filtering of official packages.

Stdin, stdout, stderr, prompts, and exit status remain native. Failure or cancellation stops the operation before the AUR phase.

For explicit or mixed installs, official targets are never audited. They remain in the final native Paru transaction alongside approved AUR targets.

## 4. Paru remains authoritative

Paru is responsible for:

- search and numbered selection;
- identifying package origin;
- dependency and provider resolution;
- preparing AUR worktrees;
- invoking `makepkg`;
- invoking Pacman and presenting its final confirmation.

AURoscope does not maintain a package index, parse Pacman databases as a resolver, call `makepkg` as a replacement builder, or install packages directly.

V1 supports one explicitly versioned Paru machine-readable surface. Unsupported or ambiguous output fails with a compatibility error rather than growing fallback parsers.

## 5. AUR audit input

For every AUR `pkgbase` in Paru's resolved build set—explicit target or dependency—AURoscope audits the exact worktree intended for the final Paru build.

The audit bundle contains only bounded required data:

- current Git commit and a deterministic recipe manifest digest;
- diff from the last successfully approved and built commit, with current text
  files always shown as text: a recipe `.gitattributes` (`-diff`, `binary`,
  textconv) cannot hide or rewrite it;
- all tracked recipe files in every audit mode, including unchanged install,
  removal, helper scripts and auxiliary packaging files;
- full text for every UTF-8 file, NUL characters included; metadata and hashes
  only for non-UTF-8 binary files. `PKGBUILD` and `*.install` files must be
  UTF-8 text, otherwise audit preparation fails (issue #104);
- committed `.SRCINFO` and minimal package metadata as untrusted data.

AURoscope never sources or executes `PKGBUILD`, `.install`, patches, source archives, or another package-supplied file while preparing the audit.

The diff supplements the complete current recipe context rather than filtering
it. Existing file and bundle limits still apply: an oversized bundle fails the
audit preparation instead of silently dropping scripts. Downloaded upstream
sources, generated build artifacts and untracked build leftovers are not collected.

The first implementation task is a disposable-Arch spike proving how Paru exposes the exact worktree before build, how an edited worktree is reused, and how `PreBuildCommand` observes that same worktree.

## 6. Codex CLI audit

### Accepted provider extension, candidate execution in #68

[ADR-0066](decisions/0066-configurable-audit-provider.md)
amends the Codex-only/no-HTTP restriction below and the multiple-backend exclusion
in section 12. Codex CLI remains the default. Execution issue #68 implements the
listed alternatives; package delivery and live qualification are separate gates.
The accepted target allows explicit selection of one provider in the shared
configuration file: Codex CLI, Claude Code, Anthropic API, OpenAI API, or an
OpenAI-compatible API with configurable URL. API keys are referenced through
environment variables; CLI authentication remains native. Issues #65 and #64
retain model and prompt configuration and must share this file.

For that extension, references to Codex in the audit/review flow mean the chosen
provider; Codex-specific admission and invocation protections still apply to
Codex. Audit scope, strict local JSON validation, human approval, final identity
guard, and native official operations remain unchanged. Provider failure offers
retry, skip, or cancel without automatic fallback or audit bypass. No new
provider is qualified by this decision. Separate `MODE: EXECUTION` issue #68
authorizes implementation and packaged verification. The candidate configuration
and its bounded evidence are in the
[provider contract note](dependency-notes/audit-providers.md).

### Current Codex implementation contract

Codex CLI is the only v1 LLM backend. The model is configurable, but there is no HTTP backend, automatic fallback, or product mode with the LLM disabled.

The accepted version policy is an inclusive minimum of `0.150.1` with no ceiling, including future stable major versions. Admit only the strict stable banner `codex-cli MAJOR.MINOR.PATCH`, comparing the triplet numerically; reject older versions, prereleases, and malformed banners. Admission does not mean test qualification. Preserve the read-only invocation, isolated empty stdin, private working directory, timeout/signals, and strict local report validation. [ADR-0046](decisions/0046-codex-minimum-version-without-ceiling.md) records this design change. Execution issue #45 implements admission in the candidate source and package; its installed-package gate passed with 0.153.4 admission. The [dependency note](dependency-notes/codex-cli-contract.md) records the successful real audit, direct sandbox enforcement check and their limits. Future versions remain unqualified by admission, and PR #48 still requires human merge.

Codex runs outside the recipe repository in a private temporary directory containing only the audit bundle. Package content is labelled as untrusted data.

The model evaluates **packaging security**, not the inherent safety of the upstream application or opaque binaries. Its scope is the recipe and its change: `PKGBUILD`, install and auxiliary files, source provenance and URL changes, integrity checks, build/package commands, destinations, permissions, privilege, services, hooks, persistence, sensitive-data access, and obfuscation. An unchanged source URL that points to the expected official upstream keeps the upstream software outside this audit. The same presumption applies on a first full audit when the source clearly points to the package's declared official upstream. Opacity of upstream binaries or inability to inspect upstream internals is not a finding, uncertainty, or reason to raise risk. A tracked recipe file without text that the recipe sources, executes, or installs as a script or hook is in scope and must be reported. Changed, mutable, mismatched, redirected, or unofficial sources and weakened integrity checks remain in scope.

A valid response is bounded JSON with:

- concise change summary;
- risk level;
- findings with file, line or range, evidence, and explanation;
- uncertainty and missing context;
- suggested points for human inspection.

The response contains no install, allow, deny, or action field. Local code validates JSON structure, sizes, paths, references, and encoding. It does not implement a competing deterministic security scanner.

If Codex fails or returns invalid output, the audit has not occurred. The user may retry, skip that package, or cancel; AURoscope does not silently continue.

## 7. Human decision

For each AUR `pkgbase`, AURoscope presents only the Codex assessment summary and packaging-risk level by default. The detailed findings, uncertainty, recipe identity, and diff are shown only when the user requests the full report. The user chooses by number, initial, or full word:

- `approve`: allow this exact recipe identity into the final Paru run;
- `inspect`: open the report/diff and ask again;
- `edit`: edit the recipe, recompute its identity, rerun Codex, and ask again;
- `skip`: exclude this AUR package base from the final run;
- `cancel`: cancel the AUR phase.

AURoscope never decides for the user. Paru retains its normal final transaction confirmation.

If an approved package depends on a skipped package, Paru remains authoritative: it may find another valid solution or reject the transaction. AURoscope does not calculate a separate dependency closure.

## 8. Final Paru execution

After decisions, AURoscope relaunches Paru with:

- all selected official targets;
- approved AUR targets;
- explicit exclusions for skipped AUR targets.

Paru performs a normal fresh resolution, builds AUR packages with `makepkg`, and installs through Pacman.

A transaction-private `PreBuildCommand` checks the actual worktree's `pkgbase`, commit, and manifest digest against the audit decision. An unexpected package base or changed identity aborts the AUR phase and returns that package to audit.

The transaction handoff is a private file under `XDG_RUNTIME_DIR` or a private temporary fallback. The local account is trusted; v1 does not add PID/inode binding or a durable anti-replay state machine.

## 9. Persistence

SQLite stores two concepts:

```text
packages(pkgbase, last_successful_commit, last_manifest_digest, updated_at)
audits(pkgbase, commit, previous_commit, codex_json, decision, created_at)
```

The baseline advances only after Paru reports successful completion. Reports are reconstructible from audit data. V1 does not add event sourcing, process-session tables, durable approval lifecycles, artifact reuse, automatic backup, or elaborate retention machinery.

## 10. Minimal implementation shape

```text
cmd/auroscope/main.go
internal/app/run.go
internal/app/paru.go
internal/app/recipe.go
internal/app/codex.go
internal/app/review.go
internal/app/state.go
internal/app/guard.go
```

This is one executable and one internal package. Files may change during planning, but new packages and interfaces require a demonstrated current use.

## 11. Verification

Required before v1:

- unit tests for audit bundle construction, Codex JSON validation, baseline selection, decisions, and guard identity;
- one supported-version Paru contract suite;
- one pinned Codex CLI contract test plus deterministic fake-Codex tests;
- integration fixtures for first audit, differential update, suspicious change, edit and re-audit, skip, cancellation, and identity drift;
- one disposable-Arch end-to-end path from Paru selection through audit and final Paru build/install;
- explicit proof that official packages receive no audit and that automated tests never touch the workstation Pacman state.

## 12. Out of scope

- deterministic security rule engine;
- HTTP or multiple LLM backends;
- Paru/libalpm resolver replacement;
- direct `makepkg` or Pacman orchestration as a replacement for Paru;
- exhaustive source or compiled-binary analysis;
- build sandboxing;
- cached package artifact reuse;
- local-host or same-UID adversary protection;
- plugins, daemon, multi-platform support, or enterprise retention/recovery.
