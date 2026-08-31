# ADR-0011: Deterministic scanner and advisory LLM contracts

## Status

Superseded by [ADR-0019](0019-minimal-llm-first-wrapper.md) on 2026-08-30.

## Context

AURoscope must preserve the difference between locally observed recipe indicators, model interpretation, and the user's decision. Package content and metadata may be hostile or may attempt prompt injection. A model can be wrong, unavailable, or configured through different local and remote inference paths, but none of those conditions may rewrite deterministic evidence or decide whether a recipe is installed.

The original proposal allowed only a narrow OpenAI-compatible HTTP adapter. Mathieu requested Codex CLI as the default local path when no complete API backend is configured, a configurable Codex model with `5.6-luna` as its default, and pragmatic exposure reduction rather than an artificial guarantee that Codex has no normal agent capabilities.

## Options considered

### A. HTTP inference only

Use an explicitly configured OpenAI, OpenRouter, or custom OpenAI-compatible endpoint.

- Advantages: smallest and easiest-to-bound inference surface; portable and explicit.
- Costs: cannot reuse an existing Codex login and requires separate endpoint, key, and model configuration.

### B. Codex CLI only

Use the installed Codex executable and its existing authentication.

- Advantages: convenient local default and reuses the operator's Codex connection.
- Costs: adds an executable/authentication contract and a broader agentic runtime; cannot cover operators who prefer an API or local OpenAI-compatible endpoint.

### C. Configurable adapters with deterministic selection

Support explicit `codex`, supported API, and `disabled` modes plus an automatic mode that selects a complete API configuration before Codex.

- Advantages: honors explicit operator choice, supports local and remote inference, and makes fallback behavior testable.
- Costs: both adapters need contract tests and distinct diagnostics; Codex isolation is best effort rather than a sandbox guarantee.

## Decision

Choose option C while keeping the deterministic scanner authoritative for its own evidence.

### Deterministic scanner

Each finding is immutable and versioned. Its stable identity covers the rule identifier and version, recipe location, and evidence digest rather than mutable explanatory prose. A finding records its severity, byte-safe path representation, line and/or byte range where available, bounded evidence, and evidence digest. Aggregate advisory signal is computed separately and cannot alter, suppress, upgrade, or downgrade the underlying finding.

The v1 catalogue covers collection integrity; downloaded or decoded execution and dynamic shell behavior; runtime network activity; checksums, signatures, mutable sources, and `SKIP`; privilege and persistence mechanisms; sensitive-data access; package semantics including `.install`, `provides`, `conflicts`, and `replaces`; and obfuscation or control/bidirectional characters. Rules state what was observed, not malicious intent, and inspection never executes or sources package content.

Context selection always identifies the manifest, metadata, deterministic findings, changed files, diff, and executable recipe context that was included. Initial ceilings are 256 KiB per file, 2 MiB aggregate text, 512 KiB diff, and 200 files; they must be benchmarked before v1 and any incompatible adjustment versions the scanner/context contract. Binary files receive bounded metadata, type, and hashes unless a dedicated non-executing parser exists. Relevant truncation or omission yields a visible `partial` inspection and `unknown` or `caution` signal, and pauses for human review; it never silently becomes `clear`.

### LLM contract

The LLM is optional, advisory, and untrusted. Its bounded request contains only selected recipe data, immutable deterministic findings, and the context manifest, with package text explicitly delimited as hostile data. Unnecessary host paths, usernames, environment values, credentials, and unrelated local content are excluded.

The locally validated response contains only a contract version, advisory level, bounded summary, change explanations, attention items, and uncertainties. It has no approval, allow, deny, install, or action field. Unknown fields, invalid enums, oversized values, unknown paths, impossible line references, malformed UTF-8, or other schema/semantic violations reject the assessment. Model output can neither replace deterministic findings nor trigger an action.

Backend selection is deterministic:

1. An explicitly configured backend always wins. Supported explicit modes are `codex`, an OpenAI/OpenRouter/custom OpenAI-compatible API backend, and `disabled`.
2. In automatic mode, a complete API configuration—provider or endpoint, API-key reference, and model—is preferred.
3. Without a complete API configuration, automatic mode uses Codex CLI when it is installed and authenticated.
4. The Codex model is configurable and defaults to exactly `5.6-luna` when omitted.
5. An explicitly selected but incomplete backend, Codex installed but unauthenticated, timeout, transport failure, provider refusal, or invalid output produces a visible remedial error, LLM signal `unknown`, and a human pause. AURoscope never silently changes backend or model.

Codex runs outside the recipe repository in a private temporary directory containing only the bounded, redacted input required for the assessment. AURoscope does not deliberately pass business secrets or unnecessary host paths. Codex may retain its normal tool capabilities; this adapter is exposure reduction, not a sandbox or a guarantee of no shell/filesystem access. Its output remains untrusted and is accepted only after the same local validation as HTTP output. The HTTP adapter remains a narrower inference path.

Timeout, backend, authentication, schema, and truncation failures are evidence about assessment completeness, not autonomous vetoes. The user may explicitly approve after the limitation is shown and recorded; failure is neither approval nor rejection.

Secrets are configured by environment-variable reference rather than stored in TOML or SQLite. Request/response hashes and bounded redacted metadata may be logged. Retained raw model material uses private permissions and the accepted retention policy; privacy mode may omit it entirely.

## Consequences

- Reports and persistence keep deterministic findings, aggregate signal, LLM assessment, technical status, and human decision as distinct records.
- Scanner and response contract versions are part of inspection identity and approval evidence.
- The implementation needs contract tests for backend precedence, explicit and automatic modes, Codex authentication/model selection, HTTP provider configuration, no implicit fallback, strict response validation, prompt injection, and visible failure.
- Codex CLI is an optional external executable contract, not a direct Go module or LLM framework dependency. ADR-0008's dependency-minimization decision remains in force; its HTTP-only client wording is amended by this ADR.
- Neither backend establishes that a recipe is safe. Model advice may be wrong or manipulated, and Codex's normal capabilities are outside any isolation guarantee.

## Evidence

- Initial proposal and scanner contract: issue [#11](https://git.2027a.net/2027a/auroscope/issues/11).
- Mathieu requested Codex CLI, configurable API providers/endpoints, and explicit failure in [comment 1089](https://git.2027a.net/2027a/auroscope/issues/11#issuecomment-1089).
- Mathieu required a configurable Codex model and relaxed the strict no-tools guarantee in [comment 1159](https://git.2027a.net/2027a/auroscope/issues/11#issuecomment-1159).
- The accepted concrete backend precedence, `5.6-luna` default, best-effort isolation, validation, and no-fallback behavior are stated in [comment 1199](https://git.2027a.net/2027a/auroscope/issues/11#issuecomment-1199).
- Authorization is Mathieu's exact standalone `GO DECISION` in [comment 1229](https://git.2027a.net/2027a/auroscope/issues/11#issuecomment-1229).

## Unresolved risks

- Codex CLI's non-interactive invocation, authentication diagnostics, model flag, output mode, and behavior must be grounded against the exact supported installed version during implementation.
- The initial context ceilings require corpus benchmarks; lowering or raising them changes cost, coverage, and denial-of-service behavior.
- Lexical rules cannot understand every shell construct. Any parser added for richer shell semantics requires separate justification and must never execute input.
- OpenAI-compatible endpoints differ in details despite sharing a protocol shape; each supported provider/profile needs a bounded capability contract.