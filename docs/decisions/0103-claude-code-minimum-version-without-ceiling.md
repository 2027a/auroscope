# ADR-0103: Claude Code minimum version without a ceiling

## Status

Accepted on 2026-11-16 in [decision issue #103](https://git.2027a.net/2027a/auroscope/issues/103),
by Mathieu's exact standalone [GO DECISION, comment 4256](https://git.2027a.net/2027a/auroscope/issues/103#issuecomment-4256),
accepting the proposal in the issue description. The implementation and its
Arch delivery (package r35) are in PR #102, execution issue #101.

This amends ADR-0066 for Claude Code version admission only, mirroring what
ADR-0046 does for Codex. Admission remains distinct from qualification.

## Context

The `claude-code` provider (ADR-0066, issue #68) ran whatever `claude` executable
it found. Codex, by contrast, rejects versions below 0.150.1 (ADR-0046). The
Claude audit contract depends on recent flags: `--json-schema` with the
`structured_output` envelope (issue #101), `--safe-mode`, `--setting-sources ''`,
`--disallowedTools`, `--strict-mcp-config`. An older CLI may reject or interpret
one of them differently, failing opaquely at best, or re-enabling tools or
settings at worst. Claude Code 2.1.325 is the version qualified in issue #101.

## Decision

Mirror ADR-0046:

- Before each audit, run `claude --version` with empty stdin, bounded output,
  the audit timeout and signal handling; hide its diagnostics like the audit's.
- Admit only the strict banner `MAJOR.MINOR.PATCH (Claude Code)` (one optional
  trailing newline), with a numerically compared triplet of at least `2.1.325`.
- Impose no upper bound, including on future major versions.
- Reject older versions, prereleases and malformed or unexpected banners. The
  refusal is an audit failure (retry, skip or cancel), never an approval or a
  bypass, and happens before any bundle is sent.
- Distinguish **admitted by policy** from **qualified by tests**. Only 2.1.325
  is qualified, with the native default model and the `haiku` alias.
- Leave the rest of the invocation, its protections and strict local report
  validation unchanged.

## Options not selected

1. An exact list: Claude Code updates very often; each update would block audits
   until a new AURoscope package.
2. A major-version ceiling: an administrative refusal at the next major release,
   with no protection against breaking minor releases.

## Consequences

Routine Claude Code updates keep working without an AURoscope release, while
clearly older CLIs fail before receiving a bundle. An admitted future version can
still change behaviour; strict local validation remains the last barrier. The
minimum is neither a security nor a compatibility guarantee.
