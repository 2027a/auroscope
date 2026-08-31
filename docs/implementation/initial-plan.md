# Initial implementation plan

**Goal:** deliver the smallest useful AURoscope v1: a transparent Paru/Pacman wrapper that requires a Codex CLI audit before each AUR package base reaches Paru's build path.

**Fixed boundary:** Paru owns search, selection, resolution, worktrees, makepkg, and Pacman. AURoscope owns recipe collection, Codex audit, the user's per-`pkgbase` decision, two SQLite concepts, and one final identity guard.

Implementation starts only after Mathieu accepts this plan and authorizes production code in a separate execution issue.

## 1. Transparent command shell and pinned Paru contract

**Files:** `cmd/auroscope/main.go`, `internal/app/run.go`, `internal/app/paru.go`, tests in `internal/app/`.

- Forward signals, terminal streams, and child exit status.
- Pass non-building commands straight to Paru.
- For bare update, run the native official phase first with `paru -Syu --repo`; stop on failure or cancellation.
- Implement only the pinned issue-21 surfaces: native interactive selection, `-P --order` records, and `-G` acquisition. Reject incompatible output/version behavior.

**Gate:** PTY tests preserve native prompts; official-only fixture commands make no Codex call; Paru contract fixtures cover selected stdout, status `1`, `REPO`/`AUR`, conflict, and missing records.

## 2. One approved AUR package end to end

**Files:** `internal/app/recipe.go`, `internal/app/codex.go`, `internal/app/review.go`, `internal/app/guard.go`.

- Resolve through Paru, acquire every resolved AUR `pkgbase` with `-G`, and read tracked recipe bytes without sourcing them.
- Build a bounded first-use bundle with commit, complete manifest digest, recipe files, committed `.SRCINFO`, and untrusted-data labels.
- Invoke pinned Codex CLI outside the recipe worktree and accept only bounded validated audit JSON.
- Show the report/diff, collect `approve` or `cancel`, write a mode-`0600` transaction file, then relaunch Paru with `--skipreview`.
- Install a transaction-specific `PreBuildCommand` that accepts only the approved `pkgbase`, worktree commit, and manifest digest.

**Gate:** fake-Codex integration proves `select → acquire → audit → approve → guard → final Paru`; malformed/failed Codex and identity drift abort before recipe execution.

## 3. Differential audit with only two SQLite concepts

**File:** `internal/app/state.go` plus tests.

Create only:

```text
packages(pkgbase, last_successful_commit, last_manifest_digest, updated_at)
audits(pkgbase, commit, previous_commit, codex_json, decision, created_at)
```

- First use sends the full recipe; later use sends the diff from the successful baseline plus complete changed files.
- Record each completed audit/decision.
- Advance `packages` only after the final Paru process succeeds.

**Gate:** tests cover first audit, differential audit, failed Paru completion, and transaction rollback. No session, approval-lifecycle, event, artifact, or resolver tables exist.

## 4. Complete human decision menu

**Files:** extend `internal/app/review.go`, `recipe.go`, and focused tests.

- `inspect` redisplays report and diff.
- `edit` opens the cache worktree, snapshots all recipe changes as a local Git commit, recomputes identity, reruns Codex, and asks again.
- `skip` omits that AUR target from the final command; `cancel` stops the AUR phase.
- Relaunch Paru with selected official targets and approved AUR targets only. A new or changed AUR dependency is rejected by the guard and reported for a new user-driven audit run.

**Gate:** edit uses the same worktree/commit at the guard; skipped targets disappear from a fresh Paru resolution; cancellation starts no build.

## 5. Bare updates, mixed installs, and multi-package review

**Files:** extend `internal/app/run.go`, `paru.go`, and integration fixtures.

- After a successful official bare-update phase, obtain pending AUR targets through the pinned Paru contract and run the same audit path.
- For search and explicit mixed installs, keep official targets unaudited in the final native Paru transaction.
- Review every resolved AUR package base, including AUR dependencies, one at a time before final execution.

**Gate:** fixtures cover bare update, search selection, explicit AUR install, mixed official/AUR install, AUR dependency, provider/conflict failure, skip, and official-only passthrough.

## 6. Supported-version contracts and disposable-Arch E2E

**Files:** test fixtures/scripts only where needed; update user documentation after behavior passes.

- Pin and test the Codex CLI JSON contract; keep deterministic fake-Codex tests for normal CI.
- Run unit tests and Paru contract tests without touching workstation Pacman state.
- Run one disposable-Arch E2E reproducing issue #21: search, exact worktree, mandatory audit, edit/re-audit, matching guard, drift rejection, skip/fresh resolution, final Paru build/install, and audit-free official update/install.

**Gate:** `go test ./...`, repository formatting/static checks, contract fixtures, and the disposable E2E all pass; `git diff --check` is clean.

## Explicit non-goals

No deterministic rule engine, HTTP/backend abstraction, resolver, direct makepkg/Pacman replacement, durable approval protocol, autonomous implementation loop, plugin system, daemon, artifact cache, or same-UID hardening enters v1.

## Issue #24 implementation status

Implemented on branch `execution/issue-24-auroscope-v1` as one executable and one `internal/app` package.

- Slice 1: terminal-preserving passthrough, signal/status forwarding, official bare-update phase, pinned Paru selection/order/acquisition parsing.
- Slice 2: first-use recipe bundle, Codex CLI JSON validation, approve/cancel review, transaction file, `--skipreview` final handoff, guard identity check.
- Slice 3: SQLite `packages` and `audits` only; differential bundles use the last successful baseline; baselines advance only after final Paru success.
- Slice 4: inspect, edit and local commit snapshot, re-audit, skip, and cancel decisions.
- Slice 5: bare update pending-AUR discovery, search selection, mixed official/AUR targets, and AUR dependency review through Paru order records.
- Slice 6: deterministic fake-Paru/fake-Codex tests and `scripts/e2e-disposable-arch.sh` guarded by `AUROSCOPE_E2E_DISPOSABLE_ARCH=1`.
