# Install the self-hosted Arch package

AURoscope v1 targets Arch Linux on `x86_64`. The package is self-hosted in this repository and is not published to `aur.archlinux.org`.

## Prerequisites

Install these AUR dependencies before building AURoscope:

- `paru-git>=2.1.0.r67.g9ac3578`: the tagged `paru` 2.1.0 package lacks the supported interactive-output fix;
- `openai-codex-bin>=0.150.1` and `<0.152.0`: AURoscope validates the exact Codex CLI contract and currently supports 0.150.1 and 0.151.0;
- the standard `base-devel` group, plus `go>=1.24.4` for building AURoscope.

AUR packages are untrusted user-produced content. Review their PKGBUILDs and tracked files before building them. Bootstrap these dependencies with an already trusted helper or with your normal manual AUR review process; AURoscope cannot install its own prerequisites.

Authenticate Codex before the first AUR audit:

```console
codex login
codex --version
```

## Build and install

From a trusted checkout of this repository:

```console
cd packaging/arch
makepkg --cleanbuild --clean --syncdeps --install
```

The PKGBUILD pins the reviewed AURoscope source commit and SHA-256 digest. `makepkg` runs the Go test suite and builds a PIE with CGO, `-trimpath`, readonly module metadata, and the pinned `github.com/mattn/go-sqlite3` module.

## First checks

```console
auroscope --version
auroscope -S --repo <official-package>
```

`auroscope --version` is transparently delegated to the installed Paru. Official-only operations do not invoke Codex. Any transaction that resolves an AUR package base requires a Codex audit and an explicit per-package decision before Paru builds it.

AURoscope writes its SQLite state to `${XDG_STATE_HOME:-$HOME/.local/state}/auroscope/state.sqlite3` by default and keeps Paru recipe clones under `${XDG_CACHE_HOME:-$HOME/.cache}/auroscope/aur` unless the corresponding `AUROSCOPE_*` development overrides are set.

## Remove

```console
sudo pacman -Rns auroscope
```

Removing the package does not delete user state or Paru recipe clones.
