# Architecture decision index

## Active for the minimal v1

Read these in order; [ADR-0019](0019-minimal-llm-first-wrapper.md) controls on any conflict:

1. [ADR-0019 — minimal LLM-first wrapper](0019-minimal-llm-first-wrapper.md)
2. [ADR-0001 — Go and self-hosted Arch packaging](0001-go-and-self-hosted-arch-packaging.md)
3. [ADR-0004 — official update before AUR review](0004-official-upgrade-before-aur-review.md), amended by ADR-0019
4. [ADR-0005 — final recipe identity guard](0005-recipe-identity-guard-boundary.md), amended by ADR-0019
5. [ADR-0007 — `mattn/go-sqlite3` with CGO](0007-mattn-go-sqlite3-cgo.md), amended by ADR-0019

## Superseded

ADR-0002, ADR-0003, ADR-0006, and ADR-0008 through ADR-0014 are historical. Their status sections point to ADR-0019. They must not be used as implementation requirements.

The broad design that produced them is archived under [`../archive/pre-llm-first/`](../archive/pre-llm-first/).
