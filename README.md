# AURoscope

AURoscope is a small terminal wrapper around [Paru](https://github.com/Morganamilo/paru) and Pacman. Its purpose is narrow: before Paru builds an AUR recipe, AURoscope asks Codex CLI to audit the exact recipe change, shows the result to the user, and records a per-package decision.

## Intended interface

```console
auroscope                    # native official update, then audited AUR updates
auroscope <search terms>     # Paru-native search and numbered selection
auroscope -S <packages>      # explicit installation
auroscope <other Paru args>  # transparent passthrough when no AUR build occurs
```

## Product flow

1. Official repository updates and installs remain native Paru/Pacman operations. They receive no AURoscope audit.
2. Paru identifies the AUR package bases that would be built.
3. AURoscope compares each exact recipe with the last successfully approved commit, or sends the full recipe on first use.
4. Codex CLI returns a structured audit of the diff and relevant files.
5. The user chooses `approve`, `inspect`, `edit` and re-audit, `skip`, or `cancel` per AUR package base.
6. AURoscope relaunches Paru with the official targets and approved AUR targets. Paru resolves dependencies, calls `makepkg`, and installs through Pacman normally.
7. A minimal `PreBuildCommand` rejects a recipe whose identity differs from the audited one.

AURoscope does not replace Paru's search UI, resolver, build machinery, or Pacman. It does not audit official packages.

## V1 shape

- Go executable for Arch Linux `linux/amd64`.
- Codex CLI is the only LLM backend.
- SQLite stores only the last successful recipe baseline and audit history.
- One internal Go package initially; split only when demonstrated behavior requires it.
- No deterministic rule engine, plugin system, build sandbox, cached artifact reuse, or same-UID security protocol.

The accepted architecture is in [`docs/architecture/minimal-v1.md`](docs/architecture/minimal-v1.md) and [`ADR-0015`](docs/decisions/0015-minimal-llm-first-wrapper.md). A narrow disposable-Arch Paru worktree spike must be completed before the new implementation plan is written.

## Current implementation

The issue #24 implementation provides the minimal v1 wrapper in `cmd/auroscope` and `internal/app`.

```console
CGO_ENABLED=1 go test ./...
go vet ./...
AUROSCOPE_E2E_DISPOSABLE_ARCH=1 scripts/e2e-disposable-arch.sh
```

The E2E script requires Docker and refuses to run unless the disposable guard variable is set. Its package-manager paths are private `/tmp/auroscope-e2e-*` paths inside the container.

## Previous design

The earlier architecture was intentionally superseded because it made the LLM optional and accumulated resolver, approval, state, recovery, and test machinery outside the product's purpose. Historical material remains under [`docs/archive/pre-llm-first/`](docs/archive/pre-llm-first/).
