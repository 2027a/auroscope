# Architecture decision index

## Active for the minimal v1

Read these in order. ADR-0015 controls except for the explicit amendments in
ADR-0046 (Codex admission) and ADR-0066 (accepted provider choice, not yet shipped).

1. [ADR-0015 — minimal LLM-first wrapper](0015-minimal-llm-first-wrapper.md)
2. [ADR-0001 — Go and self-hosted Arch packaging](0001-go-and-self-hosted-arch-packaging.md)
3. [ADR-0004 — official update before AUR review](0004-official-upgrade-before-aur-review.md), amended by ADR-0015
4. [ADR-0005 — final recipe identity guard](0005-recipe-identity-guard-boundary.md), amended by ADR-0015
5. [ADR-0007 — `mattn/go-sqlite3` with CGO](0007-mattn-go-sqlite3-cgo.md), amended by ADR-0015
6. [ADR-0046 — Codex minimum version without a ceiling](0046-codex-minimum-version-without-ceiling.md), accepted policy amendment; issue #45 admission candidate and installed-package gate passed, bounded real-CLI evidence recorded, human PR review pending

The active list also includes
[ADR-0066 — configurable audit provider](0066-configurable-audit-provider.md).
Mathieu accepted the provider extension; separate `MODE: EXECUTION` issue #68
now carries its implementation and Arch delivery. Codex remains the default;
candidate code is not proof of packaged delivery or live-provider qualification.

Proposed, not accepted: [ADR-0103 — Claude Code minimum version without a
ceiling](0103-claude-code-minimum-version-without-ceiling.md), decision issue
#103, awaiting Mathieu's `GO DECISION`; its candidate implementation is in PR #102.

## Superseded

ADR-0002, ADR-0003, ADR-0006, and ADR-0008 through ADR-0014 are historical. Their status sections point to ADR-0015. They must not be used as implementation requirements.

The broad design that produced them is archived under [`../archive/pre-llm-first/`](../archive/pre-llm-first/).
