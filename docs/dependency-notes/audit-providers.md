# Audit provider contracts — issue #68

## Authority and delivery state

ADR-0066 was accepted by Mathieu's exact `GO DECISION`, comment 3051 of #66.
His comment 3065, `PR de documentation fusionnée, passe à l'implémentation`,
authorizes the execution formalized in #68. This candidate uses one internal
package and a fixed provider switch, not a backend framework or plugin system.

The wrapper preserved source commit `ad96e164679bbdd0a2b474d9712f238cc3a06679`
on remote `loop/subject-68`. Candidate package `0.1.0.r11.gad96e16-1` now pins that
source with a verified archive checksum and non-root generated Arch `.SRCINFO`.
Freshness, Go tests/vet, the r10-to-r11 upgrade with cached archives and the full
installed supported-Paru gate passed. See `packaging/aur/README.md` for exact
package evidence. Publication and human review/merge remain pending; no deployment
or live-provider qualification is claimed here.

## Shared configuration

Read the root README for exact examples. The only user file is
`${XDG_CONFIG_HOME:-$HOME/.config}/auroscope/config.json`.
It accepts `provider`, `model`, `base_url`, and `api_key_env`. Missing file/provider
keeps Codex and its native model. Existing files are never rewritten. Reads occur
at each AUR audit attempt, not on official-only paths. Unknown fields, duplicate
keys, null fields, invalid UTF-8, trailing JSON and oversized files are rejected.
Errors never quote user-supplied configuration values.

The explicit API model is necessary to construct a valid request; optional CLI
`model` maps to `--model`. This does not settle #65's requested default model.
Prompt configuration and automatic file creation remain #64, in this same file.
No extra model/prompt file or competing environment configuration was introduced.

## Codex CLI

Default invocation, version admission, read-only sandbox, empty stdin and native
credentials remain as recorded in `codex-cli-contract.md`. The optional model is
passed as one argument. The process-group runner is reused for Claude without
changing default Codex progress text. All providers retain local validation.
Final-message file reading is now bounded before allocation, not after it.

## Claude Code

Official documentation consulted:

- <https://code.claude.com/docs/en/cli-reference>
- <https://code.claude.com/docs/en/headless>
- <https://code.claude.com/docs/en/settings-reference#disableallhooks>

### Current contract: structured output (2.1.325)

Production uses `--print --output-format json`, a fresh private `0700` directory,
`--no-session-persistence`, `--safe-mode` and an explicit session name. `--tools ''`,
MCP denial, strict empty MCP configuration, disabled setting sources, ordinary
hooks and slash commands remove package execution capabilities. The bounded
bundle is stdin, never the user's terminal input. The executable is `claude` on
`PATH`, or `AUROSCOPE_CLAUDE`, mirroring `AUROSCOPE_CODEX`.

Version admission (decision issue #103, modelled on ADR-0046): before each audit,
`claude --version` runs with empty stdin, bounded output, the audit timeout and
signal handling. Only the strict banner `MAJOR.MINOR.PATCH (Claude Code)`, one
optional trailing newline, and a numerically compared triplet of at least
`2.1.325` are admitted, with no ceiling. Older versions, prereleases and
malformed banners fail the audit before any bundle is sent; the version probe's
diagnostics stay hidden like the audit's. Admitted means eligible for an audit
attempt; only 2.1.325 is qualified below.

Like Codex `--output-schema`, the audit schema is passed with `--json-schema`.
Claude Code then offers the model exactly one tool, `StructuredOutput`, which only
records the answer, and returns it as the envelope's `structured_output` object.
Only that object is validated locally as the report; the free-text `result` is
ignored. A missing, null, non-object or invalid object fails the audit with
retry/skip/cancel. The local validation reason is shown, terminal-escaped; CLI
diagnostics remain hidden because native auth helpers can print secrets.

Why: the previous text-only contract (schema in the system prompt, report parsed
from `result`) failed intermittently with real inference. On 2026-11-15, 1 of 6
live audits of an inert fixture with Claude Code 2.1.325 wrapped the otherwise
valid report in a Markdown `json` fence, which strict validation rightly rejected
and the generic error hid. The structured-output contract removes that formatting
dependency instead of loosening local JSON validation.

Real executable checked: `2.1.325 (Claude Code)`, native Linux x64. The installed
binary's SHA-256 `0dc4f4a9a165fc60746d7ae8489712e5fc352f1e7ad4eb22e7141bd0812eb0cf`
equals the `package/claude` file of the npm archive, whose integrity matched:

- <https://registry.npmjs.org/@anthropic-ai/claude-code-linux-x64/2.1.325>
- <https://registry.npmjs.org/@anthropic-ai/claude-code-linux-x64/-/claude-code-linux-x64-2.1.325.tgz>

```text
sha512-BqZzMvP/zgLLL34S+7OzbG+cyoFp7gwWHU5koVwsyMlTAz+0zZAmbY+UkzvtN2RXxp6AcMenbYIjQJxoXfUY3w==
```

`TestProviderClaudeRealCLIContract` passed with that executable in the pinned
`archlinux:base-devel` image under `docker run --network none`, as a non-root user
with an empty HOME and an inert credential. A loopback server played the model:

- all production flags accepted by the actual executable;
- stdin bundle reached the server; interactive input was not inherited;
- exactly one model tool advertised, `StructuredOutput`, carrying the audit schema;
- the fixture answered through that tool; one request was made;
- the envelope's `structured_output` was decoded and validated locally.

`TestProviderLiveAudit` with `AUROSCOPE_TEST_LIVE_PROVIDER=claude-code` then used
real inference through Mathieu's native login on the inert fixture metadata, with
no `model` (the CLI's native default, reported as `claude-opus-5-5`): 8 of 8
consecutive structured-output audits validated locally.

The same test with `AUROSCOPE_TEST_LIVE_MODEL=haiku`, the cheapest model alias
offered by Claude Code, passed 8 of 8; every envelope reported `claude-haiku-5-5`,
a `structured_output` object and 2 or 3 turns. A temporary, uncommitted variant
sent an inert hostile fixture as text only (a `post_install` running
`curl … | sh` and `chmod 4755`, plus `sha256sums=('SKIP')`): 6 of 6 Haiku reports
validated locally, all rated `critical` and cited both install-script lines
correctly. In 4 of the 6, at least one PKGBUILD line number was wrong (usually
off by two, for instance `source=` cited as line 3 instead of 5); local
validation only checks that cited lines exist, so line accuracy is a model-quality
limit, not a contract guarantee.

This qualifies the invocation for that CLI version with its default model and
`haiku`, not other versions or models, and not audit quality on real recipes.
Unsupported flags fail without fallback.

`--bare` is not interchangeable: it disables native OAuth/keychain authentication.
Safe mode preserves native login while suppressing customizations. Managed
administrator settings can still apply; this is not an OS sandbox or a guarantee
against a compromised CLI/administrator. User settings such as custom
authentication helpers are not loaded in this invocation. An `ANTHROPIC_API_KEY`
inherited from the environment may take precedence over the subscription login.

### Earlier text-result contract (2.1.269, superseded)

The first implementation deliberately avoided the structured-output tool and put
the schema in the system prompt. It was checked with the native 2.1.269 archive
(`sha512-Ti+9oKbf2p9pJuMMj1Fv+6YzljREpy9cx6SN7NV7Zik/vaaXPxC8u+cDiGF7HnPPJ5HsLbmwKoh3BnE3IhdAeQ==`)
in disposable Arch, with zero tools advertised. An initial probe observed an extra
title-generation request; `--name 'AURoscope audit'` removed it and remains.

## HTTP contracts

### Effort forwarding requested in #77

The shared `thinking` field maps to Claude Code `--effort`, Anthropic
`output_config.effort`, and OpenAI-compatible `reasoning_effort`. Codex retains
its quoted `model_reasoning_effort` override. An omitted field adds nothing;
explicit values are literal and provider/model compatibility remains external.
Anthropic documents effort as applicable with or without extended thinking;
this change does not enable a separate thinking mode or change report parsing.

Sources consulted for these mappings:

- <https://code.claude.com/docs/en/cli-reference>
- <https://platform.claude.com/docs/en/build-with-claude/effort>
- <https://developers.openai.com/api/docs/guides/reasoning>

`TestProviderHTTPThinking` checks all API mappings, omission, preservation of
the output schema and unsupported-level refusal without retry. Claude argv and
failure without build are exercised by `TestProviderClaudeThinking`, including
installed-artifact mode. The installed HTTP test checks the configured effort.
These deterministic contracts do not qualify live models or native CLI versions.

Authoritative sources consulted:

- <https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create>
- <https://github.com/openai/openai-go/blob/main/chatcompletion.go>
- <https://github.com/anthropics/anthropic-sdk-go/blob/main/message.go>
- <https://platform.claude.com/docs/en/build-with-claude/structured-outputs>

These were current online sources, not version-pinned SDK dependencies. Production
uses Go's standard HTTP client, not either SDK. Endpoint/model compatibility still
requires qualification against the actual service.

OpenAI and compatible providers use `/chat/completions` below the configured root,
non-streaming messages, and strict `response_format.json_schema`. Only one completed
assistant choice with `finish_reason: stop`, no refusal and no tool call is accepted.
A service lacking that structured-output contract fails visibly; no schema downgrade
or alternate endpoint is attempted.

Anthropic uses `/v1/messages`, `anthropic-version: 2023-06-01`, `x-api-key`, a system
prompt, an explicit model, 8192 output tokens and `output_config.format`. Its wire
schema omits unsupported `maxLength`, `maxItems` and `minimum`; the original bounds
remain in the prompt and local validation. This follows Anthropic's documented
schema restrictions, not an error-triggered fallback. Only one text block with
`stop_reason: end_turn` is accepted. Tool, refusal and truncated responses fail.

Keys come only from the referenced environment variable and are sent only in the
HTTP authentication header. API URLs require HTTPS without userinfo/query/fragment;
redirects are refused even on the same host. Error bodies and transport diagnostics
are never printed. A valid report reflecting the selected key is discarded before
printing or SQLite persistence. This is exact-key protection, not a universal DLP
system for arbitrary transformed secrets from a malicious remote service.

Requests have cancellation, a timeout, bounded response reads and low-rate progress.
No tools, automatic request retries, provider fallback or approval bypass exist.
The local JSON validator additionally rejects duplicate keys, invalid UTF-8,
missing required top-level fields, null required fields and oversized reports.
Existing file/path/line and terminal-escaping checks remain active.

## Verification and limits

`go test ./...`, `go vet ./...` and race-enabled provider tests passed on the
checkout. Local TLS servers exercise all three API selections, headers, model,
schema, valid JSON, HTTP errors, redirect refusal, cancellation and invalid output.
A subprocess test exercises the built binary's compatible-API configuration,
approval, failed-audit skip and absence of keys in SQLite. The same test supports
`AUROSCOPE_TEST_BINARY=/usr/bin/auroscope` for the installed Arch release gate.

Fake-Claude end-to-end tests prove explicit selection, approval, retry, skip,
cancellation, no Codex fallback and no final build after a failed audit. They also
support the installed artifact. Existing Codex, Paru, drift and auxiliary-script
regressions remain in the full suite. Tests use an isolated XDG configuration root;
they must not read a developer's real provider configuration.

The worker environment has no Anthropic, OpenAI or OpenRouter API key variables;
Claude is not installed there. No personal credentials were searched or copied.
The native Claude test above uses only a downloaded, checksum-matched executable
inside disposable Arch. API simulations do not qualify those live services.

For a separately credentialed live check, use `TestProviderLiveAudit` with:

```sh
AUROSCOPE_TEST_LIVE_PROVIDER=anthropic \
AUROSCOPE_TEST_LIVE_MODEL=YOUR_ACTUAL_MODEL_ID \
CGO_ENABLED=1 go test ./internal/app -run '^TestProviderLiveAudit$' -count=1 -v
```

Use the matching provider and native authentication/environment key. Compatible
APIs also require `AUROSCOPE_TEST_LIVE_BASE_URL` and `AUROSCOPE_TEST_LIVE_KEY_ENV`.
The key variable contains a variable name, never the key itself. The test sends
only inert fixture metadata through the actual production client. Without explicit
opt-in it skips, rather than pretending qualification. Record the exact CLI,
service, endpoint and model after any live run; a provider name is not that proof.
