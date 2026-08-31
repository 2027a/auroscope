# Dependency note — Codex CLI 0.150.1 audit contract

## Context

AURoscope v1 requires `codex-cli 0.150.1` as its only audit engine. Recipe data is hostile input and the audit must not expose tools or the recipe worktree to Codex.

## Verified contract

- Require the exact `codex --version` output `codex-cli 0.150.1`.
- Invoke `codex exec` from a private temporary directory with `-C` pointing to a private empty workspace, `--sandbox read-only`, `--ephemeral`, ignored user config/rules, strict config, no Git-repository requirement, JSONL output, a strict output schema, and an explicit model.
- Pass the bounded recipe bundle inline on stdin with the final positional argument `-`; do not ask a tool-disabled model to read a side file.
- Disable shell, unified execution, web search, browser/computer use, apps/plugins/MCP, multi-agent, memory, image/artifact and related tool features. Set `web_search="disabled"` explicitly.
- Accept only known JSONL lifecycle events, `reasoning`/`agent_message` items, and a terminal `turn.completed`; reject errors, failures, unknown events and tool items.
- Treat `--output-last-message` as untrusted output: require a fresh regular non-symlink file within the byte bound, reject duplicate/unknown JSON fields, and validate every path and line range against the supplied manifest.

`workspace-read` is not a valid sandbox value in this release; `read-only` is accepted by the live parser.

## Exact sources consulted

Pinned source: `NousResearch/codex` tag `rust-v0.150.1`, commit `90854393966b21e9ebfd21b122334eb09a20c93d`.

- `codex-rs/exec/src/cli.rs` — `exec` options, prompt argument and output paths.
- `codex-rs/protocol/src/config_types.rs` and CLI parser sources — supported sandbox values.
- `codex-rs/exec/src/exec_events.rs` and event-processing tests — JSONL event and item shapes.
- Feature registry/config sources, checked together with `codex features list` from the installed 0.150.1 binary — exact feature-disable names.

## Project implementation and tests

- Runtime: `internal/app/codex.go`
- Contract and adversarial tests: `internal/app/slice2_contract_test.go`
- Integration fixtures: `internal/app/integration_test.go`

Verified commands:

```console
codex --version
codex exec --help
codex exec --sandbox read-only --help
codex exec --sandbox workspace-read --help  # must fail parser validation
CGO_ENABLED=1 go test -count=1 ./...
CGO_ENABLED=1 go test -race -count=1 ./...
go vet ./...
git diff --check
```

The parser checks require no authenticated model call. They verify argument compatibility only; they do not claim a successful live audit.

## Reuse rule

Pin the CLI and its parser/event contract together. A Codex upgrade requires re-grounding the argv, feature names, JSONL events and sandbox spelling before changing the accepted version.
