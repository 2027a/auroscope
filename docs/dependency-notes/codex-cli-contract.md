# Dependency note — Codex CLI audit contract

## Context

AURoscope v1 uses Codex CLI as its mandatory audit backend and runs it outside the hostile recipe worktree.

## Verified baselines

- Codex CLI: `codex-cli 0.150.1` and `codex-cli 0.151.0`.
- `0.150.1` was verified on 2026-08-31 with the installed executable and a live isolated audit.
- `0.151.0` was verified on 2026-08-31 from the checksum-matched upstream `x86_64-unknown-linux-musl` release artifact inside disposable Arch; its `exec --help` retains every invocation flag used by AURoscope.

## Verified contract

The supported invocation is:

```text
codex exec --json --ephemeral --sandbox read-only --skip-git-repo-check --output-last-message <private-report-path> <prompt>
```

- `--json` writes JSONL lifecycle events to stdout; stdout is not the audit response.
- `--output-last-message` writes the final assistant message as raw text. AURoscope reads and strictly validates that file as its bounded audit JSON.
- A private non-Git working directory is intentionally used to contain only `bundle.json`; `--skip-git-repo-check` is therefore required. Without it, Codex exits nonzero with `Not inside a trusted directory`.
- `--sandbox read-only` permits reading the bounded bundle while preventing model-driven workspace writes. `workspace-read` is not a valid sandbox value in either supported version.
- `--ephemeral` avoids retaining a Codex session for the package audit.

The live contract smoke returned JSONL events on stdout and wrote this schema-valid final message to the requested file:

```json
{"summary":"contract smoke","risk":"low","findings":[],"uncertainty":"","inspect":["PKGBUILD"]}
```

## Project usage

AURoscope checks the exact version string before each audit, ignores JSONL stdout, bounds the final-message file, rejects unknown fields and trailing data, and validates every file/line reference against the recipe bundle. Any command, version, transport, or validation failure enters the explicit retry/skip/cancel path; it never bypasses the audit silently.

## Verification

- `codex --version` → `codex-cli 0.150.1` for the live isolated audit.
- Checksum-matched Codex `0.151.0` release in disposable Arch → exact version plus all required `exec` flags present.
- Live isolated `0.150.1` invocation with the flags above → exit `0`; JSONL stdout and raw final-message file observed separately.
- Fake-Codex tests reproduce the same argv/output-file contract deterministically for both accepted exact versions.
