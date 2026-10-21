# Dependency note — Paru 2.1.0 and post-2.1 integration contract

## Context

AURoscope must preserve Paru's native selection and resolver while obtaining a machine-readable selected target list and enforcing an exact-recipe guard before build.

## Verified baseline

- Stable upstream tag: `v2.1.0`, commit `70f66dc9eddb40e264ee6c9197541262b7792c9c`.
- Current AUR recipe inspected: `paru 2.1.0-2`, AUR commit `329be2113c590046cb29858c23d9b96a8d7bd586`.
- Current upstream `master` inspected: `9ac3578807a87858651e81a02586ceb947686e7c`.
- Current Arch image used by spikes: Pacman `7.1.0`, libalpm `16.0.1`.

## Verified findings

1. Bare Paru becomes `-Syu`; bare search terms become sync interactive selection.
2. `paru -Ssaq --interactive TERM` is intended to display Paru's menu and emit selected names for another tool. The sync-search path returns exit status `1` even when a package was selected.
3. Released `v2.1.0` contains a file-descriptor redirection bug: menu text and the selected name both remain on stdout. This makes robust selection capture impossible without parsing human UI. Commit `d1dfbc48895a2a3b3fadc14569c3f2ffb5112b2e` fixes the redirection; no stable tag contains it at the time of inspection.
4. A current AUR-style build of `v2.1.0` must run `cargo update alpm alpm-utils` to support libalpm 16. Building the unmodified locked tag against Pacman 7.1 fails because `alpm 4.0.3` supports libalpm 15 only.
5. `PreBuildCommand` runs via `sh -c`, with `PKGBASE`, `VERSION`, and cwd set to the recipe directory. It runs for each planned package base even if Paru later reuses an already-built package.
6. In `v2.1.0`, all pre-build commands run after recipe download but before Paru's own review. Therefore AURoscope must invoke execution with Paru review disabled after AURoscope's review; otherwise Paru's file-manager review can mutate the recipe after the guard.
7. Paru may reuse a same-version package artifact from cache after the guard. Recipe approval alone does not authenticate that binary artifact.
8. `paru -P --order` is not a stable machine protocol today: `v2.1.0` documents/prints `INSTALL ...` repository records, while post-release `master` commit `9ac3578` prints `REPO ...` from `src/order.rs` and its man page still documents `INSTALL`. The adapter must be version/capability-gated and reject unknown records.
9. `paru -B` calls makepkg `--printsrcinfo` while constructing its local PKGBUILD repository, before `PreBuildCommand`. Configured PKGBUILD repositories may likewise generate `.SRCINFO` when absent or forced. Paru also special-cases any sync target beginning `./` before normal mode handling. V1 must reject local/path-like targets before starting Paru and reset config mode with trusted CLI flags.
10. A generated Paru config can include the original effective config and then override `PreBuildCommand` in a final `[bin]` section; a disposable build spike confirmed the later command won.
11. Pacman 7.1 cannot replace installed `paru 2.1.0` with the absent AUR-only `paru-git` provider while installing an AURoscope package that both depends on `paru` and conflicts with `paru<=2.1.0`: accepting removal leaves the dependency unsatisfied and aborts the transaction. The provider replacement must be a separate AUR-helper transaction. The self-hosted recipe checks this exact installed package in `prepare()` so `makepkg -si` fails before the Go build with an actionable remediation, while package metadata retains the conflict as the final installation guard.

## Disposable verification

- Built the exact tag in `archlinux:base-devel`: locked build failed against libalpm 16 as expected; applying the AUR recipe's `cargo update alpm alpm-utils` produced `paru v2.1.0 - libalpm v16.0.1`.
- With `v2.1.0`, `printf '1\n' | paru -Ssaq --interactive paru` produced the menu and `paru` together on stdout, stderr empty, exit `1`.
- Built upstream `master` commit `9ac3578`; the same command produced only `paru` on stdout, menu on stderr, exit `1`.
- `paru -P --order paru` at `9ac3578` emitted `REPO ...` and `AUR ...`; with synced databases it exited `0`, while absent repository metadata produced `MISSING ...` records and exit `1`.
- A benign local `paru -B` with layered config confirmed the final `PreBuildCommand` override, but Paru generated `.SRCINFO` through makepkg before that hook.
- A configured marker-writing PKGBUILD repository with `PkgbuildsOnly` and `GenerateSrcinfo` remained unexecuted when trusted CLI flags `--repo --mode=aur` were injected; the marker was absent, confirming config mode was reset then limited to repo+AUR on commit `9ac3578`.
- In the pinned Arch package gate, an installed disposable `paru 2.1.0-1` satisfied `depends=('paru')`; the AURoscope recipe then rejected it during `prepare()` before creating an archive. After a separate replacement with a disposable `paru-git` package providing `paru`, the same recipe built and installed successfully.

## Current use under ADR-0015

### Selected packages already encountered as dependencies (issue #99)

Pinned Paru's `src/order.rs` prints each resolver package's stored role.
In `aur-depends 5.0.0`, `resolve_repo_pkg` returns immediately for a name
already visited. A package first encountered as another target's dependency
can therefore remain `DEP` or `MAKE` when explicitly selected later.
`TARGET` is not a complete inventory of the user's selections.

A private official repository reproduces the reported relationship with
actual pinned Paru: selecting `protonmail-bridge`, `proton-vpn-gtk-app` and
`proton-vpn-daemon` emits the daemon only as `REPO DEP extra`.
The old adapter's deterministic regression fails with the exact reported
unclassified-target error. The corrected plan matches each requested name
against all `REPO` and `AUR` package records, independently of that role.
Selected dependencies remain final targets, including when another AUR
target is skipped. Unselected dependencies do not become explicit targets.
Absent names, provider aliases and a pkgbase without its named package
still fail closed; every resolved AUR pkgbase still requires audit.

Source tests and vet pass. The disposable Arch selection gate passes with
actual pinned Paru, including native numbered multi-selection and explicit
targets against the private repository. Its final install is intercepted:
the test checks all selected names, native status and absence of audit state,
without installing Proton packages. The regression also covers mixed AUR
approval/skip and is included in the installed-artifact release gate.
The subsequent r33 package pins source
`a16d32aa4fd86d86e3bfa90ef60e543f40396c63` and installs as
`auroscope 0.1.0.r33.ga16d32a-1`. Full Go race tests and vet pass.
Both the package/upgrade gate and the installed-artifact supported-Paru gate
exit 0. The r32 archive remains present during the upgrade without `--force`.
The installed regression covers native search, plus explicit targets in both
application-first and dependency-first order. Mixed AUR approval/skip,
refusal of missing names and native exit statuses remain covered.
The complete gate also builds and installs the benign AUR fixture, exercises
edit/re-audit and drift refusal/retry, and keeps official installs audit-free.
Codex is a deterministic fixture in this integration gate, not a live audit.
Verified logs and package archives are retained under the profile's
`work/artifacts/auroscope/issue-99/`; gate containers were removed afterward.
No installation was performed on Mathieu's workstation.

### Option operands and target boundaries (issue #87)

The operand rules were checked against `src/command_line.rs::parse_arg`,
`takes_value` and `src/args.rs` at pinned Paru `9ac3578`. Required long values
consume the next argument or the text after `=`; optional values require `=`.
Short `-b` and `-r` consume the remaining cluster or the next argument.
The first unconsumed `--` ends option parsing for all subsequent arguments.

`parseParuArguments` now supplies one representation for target extraction,
dispatch and final target replacement. Operand text cannot enable a preview,
an official-only operation or a system upgrade. Unknown options and missing
required operands are refused rather than guessed. The final handoff retains
option spelling, repetitions and values, even if a value equals a skipped
target. The planning call also uses `--` to protect literal target text.
Actual local recipe targets remain rejected before Paru starts.

The original spaced `--ignore linux` and `--config /tmp/pacman.conf` failures
were reproduced by `TestOptionOperandsAreNotTargets` before the fix. The new
tests cover spaced and attached values, relative paths, repetitions, target
collisions, the separator, refusal paths and native passthrough. Both approval
and skip retain exact arguments; only real targets enter the planning call.

Go tests, vet and checkout-binary regressions passed. The complete source-only
Arch gate passed with actual pinned Paru, including two audited installations
using both operand spellings and a relative configuration path. Existing
selection, edit/re-audit, drift refusal, retry and official paths also passed.
Evidence is retained in `build/issue-87/`, including `reproduction.log`,
`tests.log`, `binary-options.log` and `source-paru.log`; named container
`issue87-source-paru` exited 0 and is retained. No workstation installation
occurred. This is not yet package qualification: the immutable source checkpoint
and both installed-package gates remain required.

Recovery after main advanced to merged PR #91 preserved all candidate edits
and fast-forwarded the issue branch without creating a commit. Main's r25
source and official-install audit-state boundary remain included. The combined
test suite initially failed only because that new boundary test expected the
old planning arguments. Its expectations now include the protective `--`;
all resource, stream and exit-status assertions remain intact. Full Go tests,
vet and checkout-binary operand and audit-state regressions passed again.
Evidence: `build/issue-87/recovered-tests-fixed.log` and
`build/issue-87/recovered-binary.log`. The earlier source-only Arch result does
not qualify this combined snapshot; installed-package gates must exercise it
after the wrapper publishes the immutable source checkpoint.

This fixes operand/target separation for explicit installs, not general option
forwarding through every internal Paru call. In particular the existing system
upgrade reconstruction described below is unchanged; custom resolution settings
are still not forwarded to the preliminary `-P --order` call. Paru remains the
resolver, and this parser does not interpret configuration or ignore semantics.

### Upgrade previews (issue #86)

The upgrade dispatch previously preceded `commandMayBuildAUR`, replacing
`-Syu --print`, `-Sup` and `--sync --sysupgrade --print` with `-Syu --repo`.
The regression reproduced that replacement before the fix, including native
status 17. No real installation was attempted in this reproduction.

Non-building classification now precedes upgrade orchestration. The original
argument vector goes directly to Paru, including both `--print-format VALUE`
and `--print-format=VALUE`. The same ordering preserves the existing info,
search and repo-only paths rather than adding a separate preview parser.
Paru still validates argument combinations and owns their native semantics.

`TestUpgradePreviewPassesThrough` checks exact arguments, a single invocation,
stdin/stdout/stderr, success and failure status, and absence of Codex, clone,
state and provider-configuration side effects. The installed-artifact gate runs
the same test against the packaged executable. The ordinary upgrade test also
covers explicit short/long sysupgrade: official phase first, then acquisition,
Codex audit, approval and final AUR handoff. Package verification remains a
separate delivery requirement; checkout tests alone do not satisfy it.

The adjacent option-loss review found a broader existing limitation, not fixed
by this preview change: building upgrades still reconstruct the official phase
as `-Syu --repo`, and `auditedInstallArgs` reconstructs AUR installs as `-S` plus
selected targets. Options such as `--needed`, `--noconfirm`, `--ignore`,
`--config` and `--dbpath`, repeated refresh/upgrade flags and explicit extra
targets are not faithfully carried through those reconstructed phases.
`--aur` likewise does not suppress the existing official-first phase. These
paths require their own grounded option-forwarding work; the preview regression
does not establish that they work. This fix adds no general CLI parser and
does not change the accepted official-first architecture.

### Empty AUR updates and native warnings

At the pinned `9ac3578807a87858651e81a02586ceb947686e7c`,
`src/query.rs::print_upgrade_list` returns 1 when it prints no upgrades.
An installed package missing from AUR contributes no target; an out-of-date
flag does not by itself imply an available version upgrade. The previous
adapter rejected this normal empty result as a child failure (issue #84).

Status 1 with empty stdout is not sufficient proof of success: a network or
runtime failure can have the same shape. The candidate lets native Paru
confirm an empty result with `-Su --mode=aur --skipreview`, using the original
terminal streams. Paru prints its own missing/out-of-date warnings and
"nothing to do" text; AURoscope does not parse or reconstruct that UI.
Native failure and interruption remain failures. No second official upgrade
or configured local PKGBUILD repository is entered by this AUR-only command.

The confirmation uses the existing transaction hook with an empty approval
list. A newly discovered recipe is refused before its code executes, rather
than using a stale empty query as permission to build. This conservative race
case requires a fresh invocation to query and audit the newly available update.
Updates already found by the query retain the existing audited target flow.

Verified in disposable Arch: `TestEmptyUpdateRealParu` supplies a private
libalpm database and local RPC response to the actual pinned Paru. The empty
query returns 1; the corrected flow returns 0 and retains both native warnings.
`TestEmptyAURUpdate` checks native streams and input, nonzero statuses, partial
failed output, the empty approval guard, and absence of provider configuration.
The full source-only supported-Paru gate also passed, including real build,
edit/re-audit, drift refusal, retry and audit-free official installation.

Source evidence: `build/issue-84/source-paru.log`. The subsequent r23 package
pins `ee992223ee02dea214b5bb2c162c614ff250edd4`. Both package gates exited 0;
the r22-to-r23 upgrade retained the old archives without forcing a rebuild.
The same regression tests passed against installed `/usr/bin/auroscope`,
including actual pinned Paru and both native warnings. Package and installed
integration evidence: `build/issue-84/package-r23.log` and
`build/issue-84/paru-r23.log`. No workstation installation was performed.

These findings are historical evidence, not a complete active adapter contract. The first executable task must revalidate one supported Paru version and prove the exact native search/selection, AUR worktree, edit/re-audit, skip/exclusion, final build, and `PreBuildCommand` path required by [ADR-0015](../decisions/0015-minimal-llm-first-wrapper.md). Do not carry forward the transitional human-menu parser, broad mode classifier, or closure machinery merely because they were explored here.

## Sources consulted

- `man/paru.8`, `man/paru.conf.5`
- `src/lib.rs`, `src/install.rs`, `src/util.rs`, `src/exec.rs`
- AUR `paru/PKGBUILD`
- commits `70f66dc9`, `d1dfbc48`, `9ac35788`
