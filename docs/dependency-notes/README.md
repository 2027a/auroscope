# Dependency notes

Version-specific findings gathered from upstream sources and disposable spikes.

- [`codex-cli.md`](codex-cli.md) — pinned Codex CLI 0.150.1 argv, sandbox, feature-disable, JSONL, and report-validation contract.
- [`paru-contract.md`](paru-contract.md) — historical Paru 2.1.0/post-2.1 selection, planning, pre-build, cache, and libalpm findings.
- [`paru-issue-21-spike.md`](paru-issue-21-spike.md) — active pinned Paru surface and disposable-Arch proof for search, exact worktree reuse, edit/re-audit, guard, skip, final build, and official-only passthrough.
- [`pacman-makepkg-contract.md`](pacman-makepkg-contract.md) — Pacman/makepkg 7.1 transaction and PKGBUILD execution boundaries.
- [`sqlite-go-driver.md`](sqlite-go-driver.md) — SQLite driver comparison retained by active ADR-0007.

The former TOML/shellwords dependency selection is archived under [`../archive/pre-llm-first/go-direct-dependencies.md`](../archive/pre-llm-first/go-direct-dependencies.md).

These notes preserve evidence, not active architecture. Revalidate exact versions and commands before implementation.
