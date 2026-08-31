# Dependency notes

Version-specific findings gathered from upstream sources and disposable spikes.

- [`paru-contract.md`](paru-contract.md) — Paru 2.1.0/post-2.1 selection, planning, pre-build, cache, and libalpm behavior. Historical findings must be revalidated by the ADR-0019 spike before they become implementation requirements.
- [`pacman-makepkg-contract.md`](pacman-makepkg-contract.md) — Pacman/makepkg 7.1 transaction and PKGBUILD execution boundaries.
- [`sqlite-go-driver.md`](sqlite-go-driver.md) — SQLite driver comparison retained by active ADR-0007.

The former TOML/shellwords dependency selection is archived under [`../archive/pre-llm-first/go-direct-dependencies.md`](../archive/pre-llm-first/go-direct-dependencies.md).

These notes preserve evidence, not active architecture. Revalidate exact versions and commands before implementation.
