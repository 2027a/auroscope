# AURoscope Arch package

Self-hosted AUR-style package recipe for AURoscope. Publication on `aur.archlinux.org` is intentionally deferred.

## Prerequisites

- Arch Linux `x86_64`;
- a package providing `paru` with Paru's post-2.1 interactive-output fix and machine order records (currently `paru-git`);
- an audit provider: authenticated Codex CLI by default, or a configured alternative
  as described in [configuration](../../docs/configuration.md).

AURoscope's currently tested Paru surface is commit `9ac3578807a87858651e81a02586ceb947686e7c`. Stable Paru 2.1.0 is not compatible because it mixes the interactive human menu and selected targets on stdout. The package depends on the virtual `paru` capability so an already installed compatible provider such as `paru-git` satisfies `makepkg`, and conflicts with the known-incompatible stable package version `paru<=2.1.0` so it cannot install into the silent-search failure state. Pacman cannot fetch an absent AUR provider or replace stable `paru` with `paru-git` while installing the already-built AURoscope archive. The recipe therefore stops in `prepare()` with the remediation below when that stable package is installed, before compiling AURoscope. AURoscope also validates the required selection and order behavior when those paths run and fails closed on incompatible output; the package name and `paru --version` string alone do not prove compatibility.

AURoscope admits strict stable Codex CLI versions at least `0.150.1`, with no upper ceiling, including future major versions. Admission is not qualification: the real 0.153.4 audit and direct sandbox checks are documented with their limits in the repository's Codex dependency note; future versions are not automatically tested. Codex is deliberately **not** a Pacman dependency: an installation from npm, an Arch package, or another method works equally as long as `codex --version` reports an admitted version and the user is authenticated.

Examples:

```console
npm install -g @openai/codex
# or, if preferred:
paru -S openai-codex-bin
```

## Install

Bootstrap the package without asking AURoscope to audit itself:

```console
paru -S paru-git
git clone https://git.2027a.net/2027a/auroscope.git
cd auroscope/packaging/aur
makepkg -si
```

If a previous `makepkg -si` offered to remove stable `paru` and then reported that AURoscope's `paru` dependency could not be satisfied, no package was installed. Run `paru -S paru-git` as a separate transaction, then rerun `makepkg -si`.

Then verify the installed paths:

```console
command -v auroscope
codex --version
paru --version
```

## Use

```console
auroscope                    # official update, then audited AUR updates
auroscope <search terms>     # Paru-native interactive selection
auroscope -S <packages>      # explicit package installation
auroscope --version          # transparent Paru passthrough
```

State defaults to `$HOME/.local/state/auroscope/state.sqlite3`; override it with
`AUROSCOPE_STATE`. The current implementation does not read `XDG_STATE_HOME`.
See [configuration](../../docs/configuration.md) for CLI/API providers and their
native authentication or environment-key requirements. Codex is the default,
not a requirement when another provider is configured.

During the first desktop trial, keep invoking `auroscope` explicitly rather than replacing `paru` with an alias.

## Package verification

From `packaging/aur` on a machine with Docker:

```console
scripts/test-package.sh
```

The test checks source freshness, builds the previous and current packages in
Arch, and upgrades with the old archives still present. It validates dependency
metadata and installed files, then exercises native Paru passthrough, guard
failure, and review layout with fake Codex 0.153.4. The repository root carries
the full pinned-Paru integration gate and the opt-in real Codex audit test.

## Upstream snapshot

### Verified r32: recovery after the r31 verification

The later host check exposed a test assumption: `TMPDIR` was labelled as disk
even when it was tmpfs. The test now exercises the default temporary root on
either filesystem; explicit disk and tmpfs roots still check their actual type.
The supported-Paru gate selects both roots explicitly.

A disposable reproduction also exposed a process-exit race: Linux can report
`ESRCH`, not only `ENOENT`, when a process disappears while reading its procfs
status. Cleanup treats both as a stopped process, while other read errors still
retain the operation instead of risking deletion of active work.

Package: `0.1.0.r32.ged0c148-1`.
Immutable source: `ed0c148fe5140107f9ba48456385f172657379b4`.
Archive SHA-256:
`5477c645c10a0612e9482afe3555b2b0f060e5cd1d6013a911e0d8d23f109554`.
The checkpoint is present in remote history and includes current main at
`318f0bb0cdf4ba0072410e47833a1198f0e615f4`.
Preserve it and earlier source pins without squash.

Non-root Arch metadata generation and exact comparison, authenticated archive
checksums, freshness, namcap, Go tests and vet passed. The package gate
upgraded r31 to r32 with the old package and debug archives present, without
`--force`, and preserved custom configuration. Both disposable gates exited 0
and reported `auroscope 0.1.0.r32.ged0c148-1`.

The installed artifact passed repeated cleanup on explicitly verified disk and
tmpfs roots, acquisition/build failure, cancel, EOF, skip, handled signals,
SIGKILL recovery and concurrent-operation protection. The real supported Paru
gate repeated installations on tmpfs without retained clones or archives and
preserved edit/re-audit/build, full audit after historical clone loss, SQLite
history, identity-drift refusal, retry and audit-free official installs.
Host tests and the race detector also passed; no gate assertion was removed.

Evidence and previous/current package archives are retained under
`build/issue-97/r32`, with a checksum-verified evidence archive.
Test containers are removed after capture.
Providers are deterministic, not a new live-model qualification.
Legacy clones and external caches are not purged.
No workstation installation, deployment or merge was performed.

### Retained r31: operation-owned temporary cleanup

Issue #97 gives each AUR operation private clones. Downloads, built archives,
audit scratch and transaction files remain until Paru and installation finish,
then only that operation is removed. Success, failure, cancellation and
handled signals are covered. After SIGKILL, the next AUR operation collects
identified abandoned work only when its subprocess leases and groups are idle.
Concurrent work, legacy clones, arbitrary paths and external caches are spared.
This is temporary storage, not a durable cache, on either tmpfs or disk.

Package: `0.1.0.r31.ge7b30ea-1`.
Immutable source: `e7b30eaa69959bb4e73016b5f6a5af040e195939`.
Archive SHA-256:
`dc55f81c57f1db30e5a6d2dad4730eb65d530930a0c1bd418bfe40300adc976f`.
This remote checkpoint includes main at
`318f0bb0cdf4ba0072410e47833a1198f0e615f4`.
It includes direct Codex bundle input.
Preserve the checkpoint and earlier pins in merge history without squash.

Non-root Arch metadata generation and comparison, authenticated archive
checksums, freshness, namcap, Go tests and vet passed. The package gate moved
from r30 to r31 with previous archives still present, without `--force`,
and preserved custom configuration. Both disposable gates exited 0 and reported
`auroscope 0.1.0.r31.ge7b30ea-1`.

Installed `/usr/bin/auroscope` passed cleanup on disk and tmpfs after repeated
success, acquisition/build failure, cancel, EOF, skip and a handled signal.
SIGKILL left a live orphan protected; its work was collected after it stopped.
Concurrency, ownership, symlink boundaries and all four handled review signals
were tested. The real supported Paru gate repeated installations on tmpfs,
retained edited recipes through re-audit/build, and re-audited after cleanup
made the historical local commit unavailable, preserving #89.
Identity-drift refusal, retry, native options and audit-free official installs
also passed. SQLite baselines and audit history remain outside cleanup.

Evidence is retained under `build/issue-97`, including package and Paru logs.
The Docker caller timed out during the Paru gate; the same container completed
with exit 0 and its full log was recovered, without repeating the build.
Providers are deterministic, not a new live-model qualification. No workstation
installation, deployment or merge was performed.

### Verified r29: MIT licensing

Package `0.1.0.r29.gb922b4c-1` pins source
`b922b4c970c62b607cd07fee5d80b5e00d79dea4`, preserved on the licensing branch.
Archive SHA-256:
`d69e5b5a3f273717e02d1bbef1dc1c6a0da50ca3c08008b8e96949a4a092e0ad`.
Preserve this source checkpoint in merge history, without squash.

Non-root Arch `.SRCINFO` generation and exact comparison, source checksums,
namcap, Go tests/vet, freshness, and the package gate passed. The real r28-to-r29
upgrade kept both previous archives present, used no `--force`, and preserved
custom configuration. Pacman reported `auroscope 0.1.0.r29.gb922b4c-1`.
The installed MIT notice is package-owned, identical to the pinned source,
and mode 0644; Pacman reports MIT. Passthrough, guard failure, review, model
selection and custom-prompt checks passed with deterministic test providers.

No production code, Go dependency or runtime test changed from main's r28.
The supported-Paru integration gate was therefore not rerun for this licensing
change; the package gate is not a new real-Paru or live-model qualification.
Local evidence is retained under Hephaistos artifacts `auroscope-mit-evidence`.
No workstation installation, merge, tag, release or AUR publication is included.

### Verified r28: audit after historical clone loss

Issue #89 recovers a full current-recipe audit when the previous successful
commit is absent from a recreated clone. A visible warning explains the missing
comparison; the old identity and audit history remain recorded. A valid new
audit, explicit approval and the final identity check remain mandatory. Other
Git errors still fail preparation, and cancellation or failure keeps the baseline.

Package: `0.1.0.r28.gfce9185-1`.
Immutable source: `fce9185734c52e726cc5ee74afed0f947658637f`.
Archive SHA-256:
`c1133667b5973cc5e159653fc6ebdb6197daeb2222db276edf0755b2fba24a1c`.
This remote checkpoint includes main at
`040e879f9377b9898b0bd03495c3e47d4e8428a3`, including the native option fix.
Preserve its history without squash. Earlier r27 gates passed on an older
snapshot; r28 replaces that candidate rather than reusing its package identity.

Non-root Arch metadata generation and comparison, authenticated checksums,
freshness, namcap, Go tests and vet passed. The package gate upgraded r26 to
r28 with old archives present, without `--force`, preserving configuration.
Both named gates exited 0 and reported `auroscope 0.1.0.r28.gfce9185-1`.

The installed executable passed approval, inspection, cancel, EOF, skip, invalid
audit, failed final Paru and identity-drift cases with a missing historical commit.
Real supported Paru also rebuilt after loss of its edited clone, requiring a new
audit and approval. Existing native options, official paths and guard tests passed.
Source tests cover removed upstream history, preserved diffs and other Git errors.
Providers were deterministic; this does not qualify a new live model.

Evidence: `build/issue-89/package-r28-docker.log` and
`build/issue-89/paru-r28-docker.log`. Containers `issue89-package-r28`,
`issue89-paru-r28` and `issue89-metadata-r28` are retained. The original Paru
container completed after the caller timeout; its exit code and logs were read
back without rerunning the gate. Credentials stayed on the host; no workstation
installation occurred.

### Verified r26: native option operands

Issue #87 separates option operands from package targets and reuses that parsing
for dispatch and the final install. Both spellings of `--config` and `--ignore`,
relative values, repeated options and `--` retain their boundaries. Unknown forms
fail explicitly; actual local recipe targets remain outside v1.

Package: `0.1.0.r26.g949c75d-1`.
Immutable source: `949c75db8e21cc6b1e632830894cac1cb4765c8d`.
Archive SHA-256:
`e267f2866e35e433281d5ac5ae8706604f57466e4dcd2e44301d7ef02cb28d66`.
The remote checkpoint includes main at
`38b260dedba8e0f411b0a4a4140a6ae81cd5d60a`; preserve its history without squash.

Non-root Arch metadata generation and comparison, authenticated checksums,
freshness, namcap, Go tests and vet passed. The package gate upgraded r25 to
r26 with old archives present, without `--force`, preserving configuration.
Both named gates exited 0 and reported `auroscope 0.1.0.r26.g949c75d-1`.

The installed executable passed operand preservation, approve/skip handoffs,
separator handling, native status preservation and invalid/local target refusal.
Real supported Paru accepted separate and attached operands, relative config,
repeated ignores and the separator in audited installations. Full AUR builds,
edit/re-audit, drift refusal, retry and audit-free official installs also passed.
Providers were deterministic; this does not qualify a new live model.

Evidence: `build/issue-87/package-r26.log` and `build/issue-87/paru-r26.log`.
Containers `issue87-package-r26`, `issue87-paru-r26` and `issue87-metadata-r26`
are retained. The Docker caller timed out, but the original containers completed;
their exit codes and full logs were recovered, without rerunning the gates.
Credentials stayed on the host. No workstation installation occurred.

### Verified r25: official installs independent of audit state

Issue #88 moves audit resource preparation after Paru classifies the targets.
Explicit official installs, official search selections and empty selections
do not open SQLite or prepare clone directories. Unusable AUR audit resources
still stop the AUR path; existing corrupt files are never repaired or replaced.

Package: `0.1.0.r25.g0944841-1`.
Immutable source: `0944841179160d0aa130758b2fd671775bf2c97a`.
Archive SHA-256:
`8adc85e8b5935480a765e457fbcb9e6fe16cdde1d905ac221b2a7a2853ea054c`.
The remote checkpoint includes main at
`fa68e25167929a152a61161fa9acc209dc8ba30e`; preserve its history without squash.

Non-root Arch metadata generation and comparison, authenticated checksums,
freshness, namcap, Go tests and vet passed. The package gate upgraded r24 to
r25 with the previous archives present, without `--force`, preserving custom
configuration. Both gates exited 0 and reported `auroscope 0.1.0.r25.g0944841-1`.

The installed executable passed the audit-resource matrix: missing resources,
corrupt SQLite, invalid state parents and invalid clone paths; native arguments,
input, output and success/failure statuses remained unchanged. AUR failures
remained closed, without provider calls or destructive state recovery.
The full supported-Paru gate also passed real official selection and explicit
installation with corrupt state and an unusable clone path, without Codex.
Terminal colors, AUR builds, edit/re-audit, drift refusal and retry passed too.
Deterministic providers do not establish new live-model qualification.

Evidence: `build/issue-88/package-r25.log` and `build/issue-88/paru-r25.log`.
Named containers `issue88-package-r25`, `issue88-paru-r25` and
`issue88-metadata-r25` are retained. Credentials stayed on the host; containers
received checksum-verified archives only. No workstation installation occurred.

### Verified r24: native upgrade previews

Issue #86 keeps upgrade previews on the native Paru path, with their exact
arguments, streams and exit status. Preview commands no longer become an
official installation, and do not prepare recipes or invoke an audit provider.
Ordinary upgrades retain the official phase followed by the AUR audit.

Package: `0.1.0.r24.gc380a5b-1`.
Immutable source: `c380a5b2a0689c503eb7a746755a00e6ec8df3ea`.
Archive SHA-256:
`78c4f683657b1aab8490559f2becdb0ab4096f14bebf30501ba5b0471685d0d5`.
The remote checkpoint includes current main at
`30b53a270c3e28024a74762fc94c5ddb6f541f38`; preserve its history without squash.

Non-root Arch metadata generation and comparison, authenticated checksums,
freshness, namcap, Go tests and vet passed. The package gate upgraded r23 to
r24 with the previous archives present, without `--force`, preserving custom
configuration. Both gates exited 0 and reported `auroscope 0.1.0.r24.gc380a5b-1`.

The installed executable passed all three reported preview forms and both
`--print-format` forms, preserving success and failure statuses, terminal streams
and argument boundaries without audit side effects. The full supported-Paru gate
also passed real AUR builds, edit/re-audit, drift refusal, retry, native colors
and audit-free official installation. Preview dispatch uses deterministic Paru
responses; the integration gate uses actual pinned Paru for its other paths.

Other option loss in building upgrades was examined, not fixed by this change;
see the issue #86 section in the Paru dependency note. Provider qualification
is unchanged. No package was installed on the workstation.

Evidence: `build/issue-86/package-r24.log` and `build/issue-86/paru-r24.log`.
Named containers `issue86-package-r24`, `issue86-paru-r24` and
`issue86-metadata-r24` are retained. Credentials stayed on the host; the
containers received checksum-verified source archives only.

### Verified r23: empty AUR updates and native warnings

Issue #84 fixes the false failure when Paru's quiet AUR query returns no
updates with status 1. Native Paru confirms the empty update and displays its
missing-package and out-of-date warnings. Actual failures retain their status.
The final hook approves no recipe: a newly discovered update cannot bypass
the audit and requires a fresh invocation.

Package: `0.1.0.r23.gee99222-1`.
Immutable source: `ee992223ee02dea214b5bb2c162c614ff250edd4`.
Archive SHA-256:
`08e1b3f85d98c18f88003a5c53478cfd6e09e522e1b7e9bd67b7a5cff6d00813`.
The remote source checkpoint includes main at
`8da35d6a6a4cf4c35e991ae3b3f745a841142711`. Preserve it without squash.

Non-root Arch metadata generation and comparison, authenticated checksums,
freshness, namcap, Go tests and vet passed. The package gate upgraded r22 to
r23 with the old archives present, without `--force`, preserving configuration.
Both disposable gates exited 0 and reported `auroscope 0.1.0.r23.gee99222-1`.

The installed artifact passed the empty-update regression with actual pinned
Paru, a private package database and local AUR RPC fixture. Both native warnings
remained visible. Deterministic tests also checked streams, input, error status
and refusal of unapproved recipes. The full gate retained native colors, real
AUR builds, edit/re-audit, drift refusal, retry and audit-free official installs.

Evidence: `build/issue-84/package-r23.log` and `build/issue-84/paru-r23.log`.
Containers were removed; no credentials entered them. No workstation package
was installed. Providers were deterministic, not a new live-model qualification.

### Verified r22: literal Codex thinking on reconciled main

Issue #78 preserves Codex's native effort when `thinking` is omitted from an
existing configuration. Explicit strings reach Codex literally, without a local
list of levels or whitespace normalization. The initial complete configuration
still writes `medium`; provider effort forwarding from merged PR #81 is retained.

Package: `0.1.0.r22.gb630719-1`.
Immutable source: `b630719c9c180fb61135b8e69a58a6e249c4f8fd`.
Archive SHA-256:
`40682ff9cec4b481f8f87cadc99d3948602b3ca76c8711f42e3035e176ded4e6`.
This remote checkpoint includes main at
`1c75904a01289c0c0bb57f6beacfa2c7b3696a2d` and preserves earlier source pins.
Keep those histories reachable without squash.

Non-root Arch metadata generation and exact comparison, archive verification,
freshness, namcap, Go tests and vet passed. The package gate upgraded main's r21
to r22 with the previous archives present, without `--force`, preserving custom
configuration. Both disposable gates exited 0 and reported
`auroscope 0.1.0.r22.gb630719-1`.

The full supported-Paru gate exercised installed `/usr/bin/auroscope`: literal
and omitted Codex effort, complete configuration and customization, Claude and
HTTP effort, provider failure without bypass, auxiliary recipe context, native
colors, real AUR builds, edit/re-audit, identity drift refusal, retry, skip and
audit-free official installations. Deterministic providers do not qualify live
model access or model-specific effort support.

Evidence: `build/issue-78/package-r22.log` and `build/issue-78/paru-r22.log`.
Test containers were removed. No credentials entered them, and no workstation
installation, deployment or merge was performed.

### Verified r21: complete configuration and provider effort

Issue #77 and Mathieu's comment 3253 on PR #81 are covered by the combined
source: the first AUR audit writes `provider`, `model`, `thinking` and the full
prompt. Customized configuration is preserved. Thinking reaches Codex, Claude
Code, Anthropic, OpenAI and OpenAI-compatible providers through their respective
effort parameters. Unsupported values fail without fallback or an audit bypass.

Package: `0.1.0.r21.g92232fe-1`.
Immutable source: `92232fedb1b902f7f42416573dcb491d997706bb`.
Archive SHA-256:
`1768adb86a837d250e1f97b0e48fad0a290d04a14d240f1884de5d6feb5991ed`.
The source checkpoint is remotely reachable and contains current main at
`edffd014ed14c284d93bcfb7d028d0bf572bd07d`, including merged PR #80's model fix.
Preserve this checkpoint and both parent histories without squash. Revision r20
already exists on the separate issue #78 branch; this package advances to r21.

Non-root Arch `.SRCINFO` generation and byte comparison, authenticated archive
checksums, freshness, namcap, Go tests and vet passed. The unchanged package gate
upgraded the actual r19 package to r21 with its old archives still present and
without `--force`. A direct upgrade from main's r18 was not separately exercised.
Both disposable gates exited 0 and reported `auroscope 0.1.0.r21.g92232fe-1`.

The full supported-Paru gate exercised installed `/usr/bin/auroscope`: complete
configuration creation, customization and preservation, Codex and Claude effort,
configured HTTP effort and failure without bypass, complete auxiliary files,
preparation recovery, native colors, real AUR builds, edit/re-audit, identity
drift refusal, explicit retry, skip and audit-free official installations.
The API unit contracts cover effort forwarding for all three API choices.
These deterministic provider checks do not qualify live models, account access
or model-specific effort support.

Evidence: `build/issue-77/package-r21.log` and `build/issue-77/paru-r21.log`.
Test containers were removed; no credentials entered them. No workstation
installation, deployment or merge was performed.

### Verified r19: complete initial audit configuration

Issue #77's candidate writes `provider`, `model`, `thinking` and the full prompt
on the first AUR audit. The installed artifact transmits the initial model and
`medium` thinking setting to Codex, then preserves and uses customized values.
Existing configuration remains untouched; official operations do not create it.
Thinking currently applies only to Codex. These deterministic tests do not
qualify live model access or model-specific reasoning levels.

Package: `0.1.0.r19.g3d0c08a-1`.
Immutable source: `3d0c08abd313af09e84b3b078dd10aa59d7c9bf3`.
Archive SHA-256:
`46ce18978515fe431d50869259e42bf70dca562968e46a68f1f935b461fdfb20`.
Preserve this remote source checkpoint and earlier pins without squash.

Non-root Arch `.SRCINFO` generation and exact comparison, archive checksums,
freshness, namcap, Go tests and vet passed. The package gate upgraded r17 to r19
with the old archives present and without `--force`. Both disposable gates
reported `auroscope 0.1.0.r19.g3d0c08a-1` and exited successfully.
The supported-Paru gate exercised installed `/usr/bin/auroscope`, including
`TestPromptConfigurationLifecycle`, provider failures without bypass, auxiliary
files, preparation recovery, colors, real AUR builds, edit/re-audit, identity
drift refusal, explicit retry, skip and audit-free official installations.

During validation, main advanced to `edffd014ed14c284d93bcfb7d028d0bf572bd07d`
with the separate r18 model correction. Its functional changes are already
included in this source; r19 advances beyond that package. The branch histories
remain separate and must both survive integration. The source archive still
contains the r17 recipe, which is the package gate's tested upgrade baseline.
Logs are retained in `build/issue-77/package-r19.log` and
`build/issue-77/paru-r19.log`. Test containers were automatically removed.
Private archives were authenticated on the host and checksum-verified; no
credentials entered containers. No host installation or merge was performed.
Wrapper publication and human review remain separate from these validations.

### Verified r16: combined prompt and Codex model configuration

Package `0.1.0.r16.g20bdd79-1` combines main's configurable prompt with Codex
model selection (default `luna`), preserving providers and the short English guide.
Source pin: `20bdd799a285c1988d850416265b53023a288a3a`.
SHA-256: `27401cd67aec84ff89533421d0b6699e541d6960e181aae7eb9bb5dd33116982`.
Preserve this checkpoint and all earlier pins in merge history, without squash.

Full Go tests/vet, freshness, non-root Arch `.SRCINFO` generation and exact
comparison, archive checksums, namcap and both package gates passed. The real
r15-to-r16 upgrade retained old archives and used no `--force`. The installed
artifact received both default/explicit models and the preserved custom prompt.
The full supported-Paru gate passed on `/usr/bin/auroscope`: prompt lifecycle,
providers without bypass, auxiliary scripts, native colors, real AUR build/install,
edit/re-audit, drift refusal, explicit retry, skip and audit-free official work.
Both gates reported `auroscope 0.1.0.r16.g20bdd79-1`.

Retained containers `pr73-package-r16` and `pr73-paru-r16` exited 0; full logs
are in Hephaistos artifacts `pr73-main-gates/{package,paru}-r16-docker.log`.
The caller interruption did not stop the named Paru container; its completion
and final success marker were recovered directly from Docker. No assertions
were removed. Private archives used a host-authenticated, checksum-verified
cache without credentials in containers. No workstation installation or agent
merge occurred. Real account access to `luna` and inference quality remain
unqualified; deterministic tests establish argument delivery, not live inference.

### Verified r15: prompt configuration on current main

Package `0.1.0.r15.g6429418-1` combines the editable audit prompt with `main`
commit `70718ea6568170f93681bfcd9c0d1479019f4b59`, including PR #70's short
English README and configuration guide, provider selection and existing fixes.
The shared guide now documents automatic prompt creation and preservation;
the separate #65 / PR #73 default-model change is not included.

Immutable source pin: `6429418b8003a0360e9adb4aa18f6eafe86d393c`.
Archive SHA-256: `1423f8c4050e236e5fe5c6aba4d6a6f36a5cefa726a89562a4c97522ba78f1a7`.
The source checkpoint is a merge commit preserved on remote `loop/subject-64`.
Preserve it and earlier pins when merging; do not squash them away.

Verified: non-root Arch `.SRCINFO` generation and byte comparison, freshness,
checksums, `namcap`, full Go tests and vet, package build/check/install, and the
real r14-to-r15 upgrade with old archives present, without `--force`.
The preserved custom prompt reached Codex unchanged after upgrade.

The full supported-Paru gate passed against installed `/usr/bin/auroscope`,
including prompt lifecycle, auxiliary recipe context, configured providers and
failure without bypass, native colors, real AUR acquisition/build/install,
edit/re-audit, identity-drift refusal, explicit retry, skip and audit-free official
operations. Both gates reported `auroscope 0.1.0.r15.g6429418-1`.

Both named containers (`pr72-package-r15`, `pr72-paru-r15`) completed with exit
code 0 and are retained for recovery. Full logs are retained in the Hephaistos
artifact directory `pr72-main-gates/{package,paru}-r15-docker.log`.
The launcher only replaced Docker's `--rm` with unique container names; no test
assertion or installed-artifact mode was removed. Private archives were fetched
with host credentials into the checksum-verified source cache; no credentials
entered the containers. Anonymous installation and new live-model qualification
are not claimed. No package was installed on the user's machine and no PR merged.

### Previous r14 README delivery

Package `0.1.0.r14.ge5aecca-1` includes the shorter English user README and all
sources from `main` at `23f64e17fbf9c3d477f5a4405fdea0bcf147168a`.
Its immutable source pin is `e5aeccaed203d5c3ce8f045b35d9a25cf9932f6b`;
archive SHA-256: `9f03404701919e28df93e2d4457352519158270668179c44a2ac9729b150c8c7`.
Preserve that commit in remote history rather than squashing it away.

Non-root Arch `.SRCINFO` generation, source freshness, checksums, `namcap`, Go
checks, build and installation passed. The package gate upgraded the actual r11
package to r14 with old archives present, without `--force`. Installed ownership,
executable mode, native passthrough, guard failure, permission regression and
review layout passed. `pacman -Q auroscope` returned:

```text
auroscope 0.1.0.r14.ge5aecca-1
```

The private repository archives were authenticated and checksum-verified on the
host, then supplied through a disposable makepkg source cache. No credentials
were passed to the container and no package gate assertions were removed.
The full supported-Paru rerun has not yet been confirmed; this documentation
change does not alter code or tests relative to the merged provider release.
No installation on the user's machine or PR merge has been performed.

### Verified r13 recovery

Candidate `0.1.0.r13.geb7109a-1` preserves the editable prompt and shared provider
configuration. It pins the remotely verified source checkpoint:
`eb7109aa3d46fb9f689ba68528f8c39423f19f44`

The real authenticated archive has SHA-256:
`2cb12936a01331aa61cd5fd8bc0dddeb2a81afe64c23276b76477e035183f8e2`

The previous wrapper failure came from an anonymous private-archive download
(HTTP 404, `test-64.log` lines 926-936), after a successful package gate.
The checkpoint makes both gates use the same prefetched archive cache, without
a Docker PATH shim or credentials in containers. Checksums remain mandatory.

Non-root Arch `.SRCINFO` generation and byte comparison, freshness, Go tests
and vet passed. The package gate built and upgraded the actual r12 package to
r13 with its old archives present, without `--force`. It verified that the custom
prompt survived the upgrade and reached Codex unchanged. Both gates reported:

```text
auroscope 0.1.0.r13.geb7109a-1
```

The full supported-Paru gate passed on `/usr/bin/auroscope`, including prompt
creation/customization/preservation, auxiliary scripts, provider failure without
bypass, native colors, real AUR build/install, edit/re-audit, identity drift,
explicit retry, skip and audit-free official operations. The complete container
log and exact Docker exit event confirm success with exit code 0. Evidence:

- `logs/recovery-64/package-r13.log`
- `logs/recovery-64/paru-r13-docker.log`
- `logs/recovery-64/paru-r13-docker-exit.json`

The final fetch found `origin/main` at `23f64e1`, included in this source.
Preserve this checkpoint and earlier pins in remote history. Wrapper publication
and human review/merge remain pending. No desktop installation was changed;
deterministic providers do not qualify a new live model. The separate #65 work
and the unrelated `docs/user-readme` PR are not included or modified here.

### Retained r12 verification

Candidate package `0.1.0.r12.g78ad044-1` adds the editable audit prompt from #64
to the shared provider configuration. A missing file receives the complete
default prompt; existing files and customized prompts survive upgrades and audits.
It preserves #63's complete auxiliary recipe context and #68's providers.
The default-model change in #65 remains separate retained work, not delivered here.

The immutable source was read back on remote `loop/subject-64`:
`78ad044b5e627ea98638f0aa2074f42445c4e63b`

The authenticated archive download has SHA-256:
`bb1789647dddd3baa3b58ce806dd3610037b5c218eadc32653719a7053156197`

Non-root Arch `.SRCINFO` generation, freshness, Go tests/vet and both disposable
Arch gates passed. The package gate upgraded r11 with its real archives present,
without `--force`, and confirmed that Codex received the preserved custom prompt.
Both gates reported:

```text
auroscope 0.1.0.r12.g78ad044-1
```

The full supported-Paru gate exercised `/usr/bin/auroscope`: prompt creation,
customization and preservation, auxiliary scripts, provider success/failure,
native colors, real AUR build/install, edit/re-audit, drift refusal, explicit retry,
skip and audit-free official operations passed. Providers were deterministic;
this does not qualify a new live model or change a desktop installation.

Anonymous archive downloads returned HTTP 404 for both r11 and r12 during this
run. The gates therefore used the real archives downloaded with profile credentials
in a shared makepkg `SRCDEST` cache; makepkg still verified their recipe checksums.
No credentials entered the containers. An uncached anonymous bootstrap is not
verified in this access state; authenticated source acquisition is required.

The subsequent wrapper failure was not a prompt regression: its normal package
test lacked the earlier run's Docker cache shim. The repaired package gate was
rerun without that shim, using fresh authenticated r11/r12 archives in the
documented cache. It passed generation and byte comparison of non-root Arch
`.SRCINFO`, checksums, stable-Paru refusal, r11-to-r12 upgrade without `--force`,
and transmission and preservation of the custom prompt. Its process exited 0.

The full installed-artifact supported-Paru gate also exited 0 on this same pin,
including `TestPromptConfigurationLifecycle` and the auxiliary/provider tests.
That unchanged integration script still used the retained archive-cache Docker
shim; its anonymous download path is not newly qualified. Local evidence:
`logs/recovery-64/package-normal.log` and
`logs/recovery-64/paru-revalidated.log`. Both again reported
`auroscope 0.1.0.r12.g78ad044-1`.

Only the external package-test harness changed in this recovery, not an input
consumed by the recipe's build, check or package functions. The existing remote
source pin and candidate package identity therefore remain valid.

The final fetch still found `main` at `23f64e1`, fully included in this source.
Preserve this pin and earlier pins in remote history. Wrapper publication and
human review/merge remain separate from these successful package validations.

### Previous r11 provider verification

Candidate package `0.1.0.r11.gad96e16-1` delivers the explicit audit-provider
selection authorized by #68: Codex remains the default, with Claude Code,
Anthropic API, OpenAI API and configurable OpenAI-compatible API alternatives.
Configuration and bounded qualification evidence are documented in the root
README and `docs/dependency-notes/audit-providers.md`.

The immutable source pin, verified on remote `loop/subject-68`, is:
`ad96e164679bbdd0a2b474d9712f238cc3a06679`

The downloaded archive SHA-256 is:
`ba45ebc42831195e9e474fc9cc715978c205eb888d0b622ff2487e7c43f5fde1`

Non-root Arch `.SRCINFO` generation, freshness, Go tests/vet and both disposable
Arch gates passed. The upgrade test retained the real r10 archives and installed
r11 without `--force`. Both gates reported:

```text
auroscope 0.1.0.r11.gad96e16-1
```

The supported-Paru gate exercised `/usr/bin/auroscope`, including configured HTTP
approval and failed-audit skip, explicit Claude selection, retry/cancel/skip with
no Codex fallback, and complete auxiliary-script audit input. Native colors,
real AUR build/install, edit/re-audit, identity drift refusal, explicit recovery
and audit-free official operations also passed. These deterministic provider
tests do not qualify a live API endpoint/model or native Claude authentication.

The final fetch confirmed that the source includes current `origin/main` at
`da9dca7`; no concurrent source fix is omitted. Preserve the source commit in
remote merge history. Publication and human review/merge remain pending; no
desktop installation was changed.

### Previous r10 verification

Candidate package `0.1.0.r10.g04025fc-1` pins immutable AURoscope commit
`04025fceaaa0df59d357275f4f337b6b6c9693e7`, verified on the remote issue branch.
It retains r9's fixes and supplies every tracked recipe file to each audit,
including unchanged installation, removal and helper scripts. The diff remains
additional context. Binary files retain metadata and hashes; downloaded upstream
sources and untracked build leftovers remain outside this recipe audit.
The real archive SHA-256 is:
`d3774042b9466e2eb03af00acfff3b2fd91e449643b7293681815773d369c5d8`

The disposable Arch package gate passed: non-root generated `.SRCINFO`, archive
checksum, `namcap`, Go tests, and upgrade from r9 with its archives still present,
without `--force`. `pacman -Q auroscope` returned:

```text
auroscope 0.1.0.r10.g04025fc-1
```

The full supported-Paru gate also passed against `/usr/bin/auroscope`.
`TestAuditIncludesAuxiliaryScripts` verified full, differential and unchanged
audits, complete auxiliary text and no execution during collection. The gate
also retained native selection/colors, real AUR build/install, edit/re-audit,
drift refusal, explicit retry, skip and audit-free official installations.
Codex was deterministic in these tests; this is not a new real-model qualification.

The final remote check found `main` at
`1b70f8c67e5afe9239fbccdac9738d942c61e7fc`. Its additional commits change only
unpackaged provider-decision documentation, not shipped inputs. Publication must
preserve those additions and the immutable source pin. Human review and merge
remain required; no desktop installation was changed.

### Previous r9 verification

Candidate package `0.1.0.r9.g3c5f935-1` pins immutable AURoscope commit
`3c5f935e71f88445290522119f4cb2f8b56f9e97`.
It descends from current `main` (`8d9b844c093269286a8e5d2818d77665fecf7e99`),
retains the previous fixes and includes merged PR #61: clone preparation
neither traverses nor chmods unreadable build artifacts below the clone root.
It also makes the disposable gate work from linked Git worktrees by copying
source without host `.git` metadata, with a checkout/worktree regression test.
Its archive SHA-256 is:
`9eff0b666631389abb79f0b38731bab6b4545f6506d86cfcfbfc0f443fe5f3c6`

On 2026-09-12, the disposable Arch package gate built and installed the previous
`0.1.0.r8.gc338567-1` package, left its real archives beside the new recipe, then
ran `makepkg --syncdeps --install --noconfirm` without `--force`. Pacman upgraded
both AURoscope and its debug package to `0.1.0.r9.g3c5f935-1`.
Generated non-root `.SRCINFO`, checksum verification, `namcap PKGBUILD`, Go
`check()`, installed ownership/modes, passthrough, guard failure, and review
layout passed. `pacman -Q auroscope` reported `auroscope 0.1.0.r9.g3c5f935-1`.

The package gate first reproduced #60 on the old installed r8 artifact with
an unreadable `hermes-agent-desktop/pkg` directory. After the upgrade, bare
`/usr/bin/auroscope` completed with deterministic Paru responses for the official
update and empty AUR query, without invoking Codex. It left the artifact mode
at `000` and set only the clone root to `700`.

The full pinned-Paru gate passed against `/usr/bin/auroscope`: native
selection/color behavior, real AUR acquisition/build/install, edit and re-audit,
exact drift refusal, explicit retry with a second audit and approval, skip,
and official installs without a Codex call. The original container completed
successfully (Docker exit code 0); its complete logs were recovered after the
calling agent timed out. Codex was a deterministic test executable; these runs
do not claim a new real-model qualification.

This candidate requires human review and merge before a plain `git pull` on
`main` obtains it. Preserve the pinned source commit in merge history rather
than squashing it away or deleting its only reachable branch. No desktop
installation was changed by these tests.

## Keeping the package current

Follow the delivery rules in the root `AGENTS.md`. Before declaring a change
ready, run:

```console
bash packaging/aur/scripts/check-freshness.sh
packaging/aur/scripts/test-package.sh
AUROSCOPE_E2E_SUPPORTED_PARU=1 scripts/e2e-supported-paru.sh
```

Run these commands from the repository root. Both package-verification entry
points invoke the freshness check before starting Docker; source-only E2E mode
remains explicitly separate. The check compares code, tests, test scripts,
Go dependencies, and the installed root README and LICENSE with the immutable source pin.
It rejects changed tracked inputs and new non-ignored source files. Keep its
input list in sync with the recipe. It is a local release gate, not a configured
server-side branch protection or an automatic release publisher.

Commit source changes first, then advance `_commit`, the `rN` revision in
`pkgver`, its commit suffix, and `sha256sums` together. Generate `.SRCINFO` with
Arch `makepkg --printsrcinfo`. Recipe-only rebuilds increment `pkgrel`.
A new package identity is required whenever delivered contents change; forcing
a rebuild of an old pin is not an update. The package test keeps the actual
previous package archives present to exercise this upgrade path every time.

After merging a package update, from an existing clone:

```console
git pull --ff-only
cd packaging/aur
makepkg -si
pacman -Q auroscope
```

If already in `packaging/aur`, omit the `cd` command. `auroscope --version`
intentionally passes through to Paru; use `pacman -Q auroscope` for this package.
The installed README comes from the immutable source and may retain historical
candidate wording; current recipe metadata identifies the delivered snapshot.

AURoscope is licensed under MIT. The package declares `license=('MIT')` and
installs the upstream notice at `/usr/share/licenses/auroscope/LICENSE` with
mode 0644. Third-party dependencies retain their own licenses.

## Distribution boundary

Forgejo remains canonical; the GitHub mirror follows `main` only. Future release
automation is intended to publish from Forgejo to GitHub, with 0.2 as the first
official release only once readiness is established and publication authorized.
This licensing change does not create a tag or release, change mirror settings,
or publish to the AUR. AUR publication remains deferred until after several
weeks of testing and a separate authorization.
