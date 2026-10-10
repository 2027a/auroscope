# Dependency note — Codex CLI audit contract

## Context

AURoscope v1 uses Codex CLI as its mandatory audit backend and runs it outside the hostile recipe worktree.

## Verified baselines

Version evidence below is deliberately separate from admission policy. [ADR-0046](../decisions/0046-codex-minimum-version-without-ceiling.md) accepts stable `codex-cli MAJOR.MINOR.PATCH` versions numerically at least `0.150.1`, without a ceiling, including future majors. Older versions, prereleases, and malformed banners are rejected. Execution issue #45 implements this admission policy in the candidate source and package. The real 0.153.4 audit, direct sandbox enforcement check and installed-package gate passed as detailed below; those observations qualify only the tested behaviors and environment. Neither 0.153.4 nor future versions are qualified by admission alone. Keep a pinned contract-test matrix and preserve all invocation, process, and validation protections below. Valid JSON alone does not establish unchanged sandbox/execution semantics.

- Codex CLI: `codex-cli 0.150.1` and `codex-cli 0.151.0`.
- `0.150.1` was verified on 2026-08-31 with the installed executable and a live isolated audit.
- `0.151.0` was verified on 2026-08-31 from the checksum-matched upstream `x86_64-unknown-linux-musl` release artifact inside disposable Arch; its `exec --help` retains every invocation flag used by AURoscope.

## Verified contract

The supported invocation is:

```text
codex exec --json --ephemeral --sandbox read-only --skip-git-repo-check --output-schema <private-schema-path> --output-last-message <private-report-path> <prompt>
```

- `--json` writes JSONL lifecycle events to stdout; stdout is not the audit response.
- `--output-last-message` writes the final assistant message as raw text. AURoscope reads and strictly validates that file as its bounded audit JSON.
- Codex runs in a private non-Git directory outside the recipe worktree.
  It contains the output schema, not a recipe file; the bounded bundle arrives
  through private stdin. `--skip-git-repo-check` remains required.
- `--sandbox read-only` permits reading the bounded bundle while preventing model-driven workspace writes. `workspace-read` is not a valid sandbox value in either supported version.
- `--ephemeral` avoids retaining a Codex session for the package audit.
- `--output-schema` constrains the final response before local validation. Codex/OpenAI structured output requires every object property to appear in `required`; optional finding fields therefore use nullable types and local decoding maps `null` to their zero values.
- `codex exec --help` documents that piped stdin is appended to a positional
  prompt. AURoscope supplies a finite private reader of the complete JSON
  bundle, never the interactive Paru/review stream. The version check still
  receives empty stdin. Earlier qualification used empty audit stdin and a
  local file; issue #95 replaces that delivery mechanism.

The live contract smoke returned JSONL events on stdout and wrote this schema-valid final message to the requested file:

```json
{"summary":"contract smoke","risk":"low","findings":[],"uncertainty":"","inspect":["PKGBUILD"]}
```

## Project usage

### Direct bundle delivery, issue #95

A valid report saying the recipe was inaccessible is not evidence of an audit.
The old client wrote `bundle.json` but left audit stdin empty, relying on the
model to open the file. An inert live probe with Codex 0.156.1 reproduced an
`unknown` report with exit zero when recipe-reading tools were disabled.
The same probe received a random recipe version through stdin without those
tools. This reproduces the failure condition, not Mathieu's exact desktop
configuration.

The client now sends the complete JSON bundle through private stdin. No local
`bundle.json` is written. A fixed transport instruction is appended to both
default and custom prompts; stored user configuration remains unchanged.
Recipe text is not placed in argv, executed, or treated as instructions.
Read-only sandboxing, output validation, review and final guard remain intact.
This ensures delivery, not model accuracy or immunity to prompt injection.

`TestCodexBundleDirectInput` is red against the old client and green against
the candidate, including existing custom prompts and preserved user approval.
The auxiliary-file regression receives full, diff and unchanged bundles from
stdin. The installed-Paru gate runs these tests against `/usr/bin/auroscope`.

The opt-in production-client test passed with real Codex 0.156.1 and
`gpt-5.6-luna`, effort `medium`, with recipe-reading tools disabled.
It requires the exact random version from an inert recipe in the summary; a
nonempty summary alone does not qualify this check.

```sh
AUROSCOPE_TEST_REAL_CODEX_INPUT=/absolute/path/to/codex \
  go test ./internal/app -run '^TestCodexRealDirectInputContract$' -count=1 -v
```

The npm launcher on this host needs Node on PATH. The installed platform
package also contains the real native `bin/codex` and its sibling helpers;
the successful probe invoked that binary directly without copying credentials.
The version-matched help and feature schema were consulted:

<https://raw.githubusercontent.com/openai/codex/rust-v0.156.1/codex-rs/core/config.schema.json>

### Explicit initial configuration and thinking

Issue #77 requires all initial audit settings to be visible in `config.json`:
Codex, `gpt-5.6-luna`, `medium` and the complete unchanged prompt. Existing files
remain untouched; an omitted `thinking` leaves Codex's native setting intact.
An explicit string becomes one `--config` argument whose value is
`model_reasoning_effort=<quoted string>`. JSON string encoding quotes the value
as a TOML basic string, rather than interpolating raw configuration or shell code.

The version-matched 0.153.4 schema declares `model_reasoning_effort` as a nonempty
string. Its shared CLI parser accepts global `--config key=value` overrides and
parses values as TOML. AURoscope does not maintain its own list of model levels.
Issue #78 leaves string validity to the user and Codex: explicit empty, unknown,
whitespace and escaped control-character strings are forwarded literally.
Omission sends no override. Null and non-string JSON values remain invalid.
The initial configuration template from #77 still explicitly writes `medium`;
removing the field restores the native Codex default without rewriting the file.
Sources consulted:

- <https://github.com/openai/codex/blob/rust-v0.153.4/codex-rs/core/config.schema.json>
- <https://github.com/openai/codex/blob/rust-v0.153.4/codex-rs/utils/cli/src/config_override.rs>

Deterministic tests cover initial fields, argv transmission, existing-file
preservation and invalid configuration. The lifecycle test also supports the
installed artifact through `AUROSCOPE_TEST_BINARY`. These contracts do not
qualify live model access, model-specific thinking levels or inference quality.

### Explicit audit model

The issue #65 source candidate adds `--model <literal-id>` to the audit argv.
Issue #76 corrects the default to exactly `gpt-5.6-luna`; the optional AURoscope
`config.json` can override it. Configuration is read only on the audit path,
not for official operations.
No shell interpolation, alias translation, or fallback model is introduced.

Recovery reconciles this candidate with the merged provider implementation:
`loadAuditConfig` is the single reader and supplies `gpt-5.6-luna` only for Codex.
Claude Code retains its native default; API providers require an explicit model.
The complete auxiliary-file prompt from #63 is preserved without modification.

The version-matched Codex 0.153.4 source exposes the shared `model` argument in
`ExecSharedCliOptions` and marks it global in `mark_exec_global_args`:
<https://github.com/openai/codex/blob/rust-v0.153.4/codex-rs/exec/src/cli.rs>

Fake-CLI tests verify the default and a literal override as one argv value.
The local Codex launcher cannot start because `node` is absent; this is not
real-model qualification or proof that an account accepts `gpt-5.6-luna`.
The candidate still needs its immutable package pin and installed-artifact gates.

AURoscope checks the strict stable banner and numeric minimum before each
audit, bounds JSONL/stderr diagnostics and the final-message file, rejects
unknown fields and trailing data, and validates file/line references against
the bundle. Codex runs in a separate process group with private bundle stdin,
a five-minute timeout, visible progress, signal forwarding, descendant cleanup,
and a fresh private directory for every retry. Command, version, transport or
validation failure enters retry/skip/cancel; there is no audit bypass.

## Verification

- `codex --version` → `codex-cli 0.150.1` for the live isolated audit.
- Checksum-matched Codex `0.151.0` release in disposable Arch → exact version plus all required `exec` flags present.
- Live isolated `0.150.1` invocation with the flags above → exit `0`; JSONL stdout and raw final-message file observed separately.
- Fake-Codex tests reproduce the same argv/output-file contract deterministically for both accepted exact versions.
- The issue #45 regression matrix admits the inclusive minimum, 0.153.4, future minor/major versions, and decimal components beyond machine integer range; it rejects old, prerelease, suffixed, noncanonical, and multiline banners. LF and CRLF line terminators are accepted; other surrounding whitespace is not. Numeric comparison uses canonical decimal length then value, without integer overflow.
- Live `brave-bin` first audit at commit `7b85edf5694c59dd523fabfdec2cedbd525b5e37` with npm-installed Codex 0.150.1 → visible progress at 15-second intervals and a locally schema-valid report after about 45 seconds; Codex consumed no interactive decision input.

## Issue #45 qualification attempt (2026-09-06)

Official release: <https://github.com/openai/codex/releases/tag/rust-v0.153.4>.
The SHA-256 hashes below matched the release API asset digests before execution:

- `codex-x86_64-unknown-linux-musl.tar.gz`: `f479424eca092484dc40d87ae28c44f4cc40234a60045d6131e493800d814a30`.
- `codex-package-x86_64-unknown-linux-musl.tar.gz`: `a822187e1a2420c61c5926721bfbd878701ed95547c9bb0d4de4498a16ba1821`.

The standalone executable reports `codex-cli 0.153.4`; its `exec --help` retains all production flags and the documented stdin append behavior. Its audit attempt also reported a missing sibling `codex-code-mode-host`. The complete official package includes that helper (plus runtime resources); retrying with its `bin/codex` removed that helper error. Do not install only the standalone binary for this contract smoke.

The initial full-package live test reached the production invocation but failed after 16.86 seconds with CLI exit 1 / HTTP 401 in the unauthenticated profile-isolated environment. This is historical, not a current authentication blocker. Mathieu's [comment 2164](https://git.2027a.net/2027a/auroscope/issues/45#issuecomment-2164) identified the authenticated host environment. With `HOME=/home/mathieu` and `CODEX_HOME=/home/mathieu/.codex`, `login status` succeeded and the upstream verification lane obtained a locally validated report in 20.79 seconds ([evidence](https://git.2027a.net/2027a/auroscope/issues/45#issuecomment-2170)). The delivery lane independently reran `TestCodexReal01534AuditContract` on source `235916f34adfac0d12fcf7efd7c84ab477a77dd9`: PASS, CLI success and a locally validated report in 26.81 seconds. It used the actual production invocation, including read-only sandbox, isolated empty stdin, private directory, output file and schema. No token contents were read, copied or published; Forgejo identity remained Hephaistos.

### Separate sandbox observations

On the same checksum-verified 0.153.4 package and host, a direct `codex sandbox -c 'sandbox_mode="read-only"' -- /usr/bin/python3 -c '<probe>'` check ran in a disposable private directory. The inert probe first read `input.txt`, then attempted `Path("write-probe.txt").write_text("inert probe")`. Outside the sandbox the identical probe exited 0 and created the marker; the control marker was removed before the sandbox run. Inside the sandbox it printed the input, exited 1 with `OSError: [Errno 30] Read-only file system`, and created no marker. This demonstrates actual filesystem enforcement for that direct sandbox path, not merely a model's description of it. No recipe or package-supplied code was executed.

Version-matched source consulted: [`cli/src/lib.rs`](https://github.com/openai/codex/blob/rust-v0.153.4/codex-rs/cli/src/lib.rs) (`LandlockCommand`) and [`cli/src/debug_sandbox.rs`](https://github.com/openai/codex/blob/rust-v0.153.4/codex-rs/cli/src/debug_sandbox.rs) (`load_config`, explicit legacy sandbox-mode override). Set the subprocess working directory rather than adding `-C`: this version's `sandbox -C` requires a named permission profile. The initial probe with `-C` exited 2 before execution and was not counted as an enforcement test.

A separate model-driven probe was inconclusive: the real `exec` command exited 0 with no marker and an assistant message claiming a write denial, but its JSONL contained only `agent_message` items and no `command_execution` event. That self-report is not execution evidence and is excluded from the qualification claim. The initial scratch assertion accepted text alone; review rejected that false-positive criterion. The successful production-client audit plus direct sandbox enforcement check are bounded evidence, not exhaustive qualification of all model-driven tools, network restrictions, configuration combinations or future versions.

The opt-in test is reproducible with an authenticated environment and a verified full release package:

```console
AUROSCOPE_TEST_REAL_CODEX=/absolute/path/to/codex-package/bin/codex CGO_ENABLED=1 go test ./internal/app -run '^TestCodexReal01534AuditContract$' -count=1 -v
```

It sends only inert fixture metadata, calls the real production client, and requires a locally validated report. It executes no recipe and makes no Paru/Pacman transaction. Without the environment variable, the live test explicitly skips rather than pretending qualification. Successful execution is still not a general sandbox guarantee; preserve the separate process/stdin/timeout and local-validation regression suite.

### Installed-package regression and authorized Arch gates

The generated Kanban task initially prohibited all PKGBUILD execution, beyond the product's audit-time prohibition. Mathieu explicitly authorized the two disposable-Arch gates in [comment 2187](https://git.2027a.net/2027a/auroscope/issues/45#issuecomment-2187). No recipe was executed during audit or on the host.

The package test was changed first to present fake Codex 0.153.4 to the installed binary. RED: old package `0.1.0.r4.gd0ad113-3` built and installed, then rejected that banner with the exact reported allowlist error; container exit 1. GREEN after changing only the immutable source identity/version/checksum and generated `.SRCINFO`: package `0.1.0.r5.gd42be9f-1` built, ran its Go `check()`, installed, and passed the same test. The installed package admitted the fake 0.153.4 and reached the validated review UI. The test also preserved early incompatible-stable-Paru rejection, compatible-provider installation, npm-style external Codex support, ownership/mode, native passthrough and guard-failure checks. The fake CLI verifies admission and package wiring, not real model behavior; that evidence is the separate authenticated production-client test above.

Source: signed commit `d42be9f2e3b06579e08c37631a8ff17ebc8db2e3`; archive SHA-256 `c363282770c188c24da75f4a8c4ba06dd2d55f5dacf42478b1250371ba64d6b7`. Preserve this source in merge history; do not squash it away while the package pins it. This snapshot does not include the separate, unmerged selection-colour PR #49.

The delivery lane also reran Go tests, vet, fake-Paru Arch, and `scripts/e2e-supported-paru.sh` with real pinned Paru `9ac3578807a87858651e81a02586ceb947686e7c`: all passed. The real-Paru gate built and installed `hello`, exercised edit/re-audit, rejected identity drift, preserved skip without build, and installed official `tree` without another Codex call. It uses fake Codex and a source-built AURoscope; it is distinct from the installed-package gate. All package-manager writes stayed inside disposable containers. PR #48 remains for human review; no merge, tag, AUR publication or workstation upgrade is implied.
