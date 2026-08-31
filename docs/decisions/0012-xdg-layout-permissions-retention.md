# ADR-0012: XDG layout, private permissions, and bounded retention

## Status

Superseded by [ADR-0019](0019-minimal-llm-first-wrapper.md) on 2026-08-30.

## Context

AURoscope stores trusted configuration, authoritative SQLite state, generated reports, reconstructible recipe/model data, and short-lived transaction handoffs. Mixing these artifacts under one root would blur durability and cleanup boundaries. Predictable or weakly protected work directories would also expose recipe contents, model material, approvals, and process handoffs to other local users or to symlink/path-substitution attacks.

Retention must preserve enough decision and outcome history to diagnose older incidents without allowing any category to grow indefinitely by default. Reports and caches are reconstructible and can expire sooner than authoritative history.

Mathieu accepted the amended D11 proposal in issue [#12](https://git.2027a.net/2027a/auroscope/issues/12): retain the proposed XDG structure and ownership/symlink controls, but replace unlimited and longer retention defaults with shorter finite values that remain configurable.

## Options considered

### A. Standard XDG roots with indefinite authoritative history

Separate config, state, cache, and runtime according to XDG, but retain identities, decisions, and outcomes until explicit purge.

This preserves maximum history, but violates the requirement that nothing be unlimited by default and allows silent long-term growth.

### B. Standard XDG roots with tiered finite retention

Separate artifacts by durability, protect every root and sensitive file, clean runtime work defensively, and apply shorter age/quota defaults by artifact class.

This preserves a useful one-year audit window while bounding all default retention. It requires explicit purge jobs and configurable policy, but does not make reconstructible artifacts durable by accident. This option is accepted.

### C. Uniform short retention

Apply one short age limit, such as 30 days, to state history, reports, backups, and cache.

This is simple, but discards decision and outcome evidence too quickly for investigation while retaining reconstructible data longer than necessary relative to its value.

## Decision

Use these exact roots and default modes:

```text
${XDG_CONFIG_HOME:-$HOME/.config}/auroscope/config.toml                  0600
${XDG_STATE_HOME:-$HOME/.local/state}/auroscope/state.db                0600
${XDG_STATE_HOME:-$HOME/.local/state}/auroscope/reports/YYYY/MM/...     0600 files, 0700 dirs
${XDG_STATE_HOME:-$HOME/.local/state}/auroscope/backups/                 0700
${XDG_CACHE_HOME:-$HOME/.cache}/auroscope/recipes/<source>/<pkgbase>/    0700 root
${XDG_CACHE_HOME:-$HOME/.cache}/auroscope/model/                         0700, optional
${XDG_RUNTIME_DIR}/auroscope/<transaction>/                              preferred, 0700
${TMPDIR:-/tmp}/auroscope-<uid>-<random>/<transaction>/                  fallback, 0700
```

AURoscope starts with umask `0077`. Its roots and directories are `0700`; sensitive regular files, including SQLite state, reports, model request/response material, and SQLite `-wal`/`-shm` files, are `0600`. XDG roots must be absolute. AURoscope refuses sensitive final components that are symlinks, roots not owned by the current user, and roots writable by group or other users.

When `XDG_RUNTIME_DIR` is set, it must be absolute, owned by the user, and safely permissioned. Otherwise AURoscope creates an unpredictable private fallback with `os.MkdirTemp` under `${TMPDIR:-/tmp}`. It never assumes `/tmp` is memory-backed.

Configuration is strict TOML. Unknown keys are errors; defaults live in the executable, so a config file is optional. Configured commands use argv arrays. If no configured viewer exists, AURoscope tries `$VISUAL`, then `$EDITOR`, then a pager. Environment command strings are split with `go-shellwords` while environment and backtick expansion remain disabled; AURoscope never uses `sh -c` and appends the report path as its own argv element.

Each transaction owns one recorded work root bound to transaction ID, UID, creation time, random nonce, device, and inode. Cleanup is descriptor-relative and does not follow symlinks. Startup recovery and `auroscope cleanup` inspect only direct children of configured AURoscope roots and cross-check SQLite ownership, UID, device/inode, age, and PID plus process start time. Active recorded sessions are never removed. Stale runtime handoffs have a 24-hour grace period; actual temporary trees from failed work are removed when recovered.

Apply these default retention limits:

- recipe identities, human decisions, and build/install outcomes: 365 days;
- generated reports and retained model JSON: 30 days;
- SQLite state backups: 7 days;
- reconstructible recipe cache: 14 days or 512 MiB under LRU pruning, whichever limit is reached first;
- failed-work metadata: 3 days.

Every threshold is explicitly configurable, but no category defaults to unlimited retention or disabled purge. Cache pruning is age-and-quota based, serialized by a lock, and never follows symlinks. Reports, backups, and SQLite state are not cache.

## Consequences

Positive:

- configuration, durable state, reconstructible cache, and disposable runtime work have unambiguous ownership and cleanup semantics;
- private modes, ownership checks, and no-follow cleanup reduce local disclosure and path-substitution risk;
- finite defaults prevent silent accumulation while preserving a useful one-year decision/outcome trail;
- age and quota both bound the largest reconstructible cache.

Negative or deferred:

- long-running installations and stale recovery require reliable PID start-time and SQLite cross-checks rather than age alone;
- a one-year history may still consume material space on a busy system and needs observable purge behavior;
- users who need longer audit history must configure it explicitly or export it outside AURoscope's authoritative store;
- exact purge transaction scheduling and user-visible cleanup diagnostics remain implementation details to test in the execution lane.

## Evidence

- XDG Base Directory Specification: <https://specifications.freedesktop.org/basedir-spec/latest>
- Grounded proposal and security/cleanup analysis: [`docs/architecture/design-proposals.md`](../architecture/design-proposals.md), section 7.
- Accepted amended proposal: issue [#12 comment 1114](https://git.2027a.net/2027a/auroscope/issues/12#issuecomment-1114).
- Authorization: Mathieu's exact `GO DECISION` in [issue #12 comment 1161](https://git.2027a.net/2027a/auroscope/issues/12#issuecomment-1161).

## Unresolved risks

- A malicious process running as the same UID can still race paths between validations unless operations remain descriptor-relative throughout.
- Crash residue created before its SQLite record is durable must be discoverable without trusting marker content alone.
- Retention must not delete identities or evidence still referenced by a live transaction, approval, or retained build/install outcome.
