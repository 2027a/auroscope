# AURoscope

**Review AUR recipes before building them with Paru.**

AURoscope is a terminal wrapper around [Paru](https://github.com/Morganamilo/paru).
It asks an LLM to review each AUR recipe and its changes, then lets you decide
whether to proceed. Paru still handles search, dependencies, builds and installation.
Official repository packages pass through without an audit.

An audit is advice, **not a security guarantee**. AURoscope reviews packaging, not
the safety of upstream software or downloaded binaries. It does not sandbox builds.
A final identity check rejects recipes that changed after approval, but does not
protect against a compromised local account or all later modifications.

## License

AURoscope is licensed under the [MIT License](LICENSE), copyright Mathieu.
Third-party dependencies retain their own licenses; this grant does not relicense them.

## Install

You need Arch Linux x86_64, `base-devel`, `git`, a compatible Paru, and an audit
provider. Codex CLI is the default; install and authenticate it using its own
instructions. Claude Code and API providers are also available.

**Stable Paru 2.1.0 is incompatible.** The tested Paru commit is
`9ac3578807a87858651e81a02586ceb947686e7c`; install `paru-git` before AURoscope.
If you do not have Paru, build its [AUR recipe](https://aur.archlinux.org/packages/paru-git)
first. See the [package guide](packaging/aur/README.md) for details and diagnostics.

Review the recipes, then run as your normal user, not root:

```sh
paru -S paru-git
git clone https://git.2027a.net/2027a/auroscope.git
cd auroscope/packaging/aur
makepkg -si
pacman -Q auroscope
```

The package is hosted in this repository, not on `aur.archlinux.org`.
`makepkg` builds the immutable source snapshot pinned in `PKGBUILD`, not the
current checkout. Do not use AURoscope to bootstrap its own installation.

## Use

```sh
auroscope                 # official updates, then audited AUR updates
auroscope -Syu            # the same update flow
auroscope visual studio   # Paru's interactive search and selection
auroscope -S paru-git     # install an AUR package after review
auroscope -S git          # official package: native installation, no audit
auroscope -Q              # native installed-package query
```

For each AUR package base, choose `approve`, `inspect`, `edit`, `skip` or `cancel`.
The menu also accepts initials and numbers. Editing triggers a new audit.
Provider and audit-preparation errors offer retry, skip or cancel—never an automatic
bypass or fallback. Tracked symlinks are reviewed as target text, not followed.
Paru retains its final installation confirmation. Skipping a dependency may prevent
the remaining installation; cancelling AUR work does not undo completed official
updates. Local recipe builds (`-B`, `--build`, local paths) are outside scope.

Each AUR operation uses private temporary clones, kept until Paru and its
installation finish, then removed on success, failure or cancellation. Handled
signals also end the operation and clean it. After SIGKILL or a crash, abandoned
operations are collected on the next AUR operation, only after their processes
have stopped. Concurrent operations use separate clones. SQLite history remains.

`AUROSCOPE_CLONE_DIR` selects the temporary root (by default
`${TMPDIR:-/tmp}/auroscope-clones`). For audited installs, `--clonedir` and
`--builddir` likewise select a root, not a shared clone. This is not a durable
archive cache, on tmpfs or disk. Legacy clones and independently configured
Paru/makepkg caches are not purged. Recipe edits survive through re-audit and
build, not between operations; if an edited baseline is no longer available,
the next audit reviews the full current recipe with a warning, as before.

## Configure

With no configuration file, AURoscope uses Codex CLI and requests `gpt-5.6-luna`.
Codex must report a stable `codex-cli MAJOR.MINOR.PATCH` version of at least
`0.150.1`; admission does not guarantee compatibility with every later version.

To choose Codex, Claude Code, Anthropic API, OpenAI API or an OpenAI-compatible
service, create `${XDG_CONFIG_HOME:-$HOME/.config}/auroscope/config.json`.
For example:

```json
{"provider": "claude-code"}
```

CLI model selection is optional; APIs require an explicit model. API keys are
referenced by environment variable name, never stored in the file. A selected
remote service receives the recipe audit bundle. See [configuration](docs/configuration.md)
for exact fields, examples and qualification limits. On the first AUR audit, a
missing file is created with `provider`, `model`, `thinking` (`medium`) and the
full default `prompt`. Edit these fields to customize the audit; existing files
and custom prompts survive upgrades. `thinking` is sent to the selected provider
as its effort parameter; supported levels depend on the service and model.
Official-only operations do not create or read it.

## Update and help

From your existing repository root:

```sh
git pull --ff-only
cd packaging/aur
makepkg -si
pacman -Q auroscope
```

`auroscope --version` reports **Paru's** version; use `pacman -Q auroscope` for
AURoscope's package version. Forcing an old recipe to rebuild does not update its pin.

- [Configuration and provider contracts](docs/configuration.md)
- [Package installation and verification](packaging/aur/README.md)
- [Technical documentation and history](docs/README.md)
- [Report a problem](https://git.2027a.net/2027a/auroscope/issues): include the command,
  package/CLI versions, provider and error, but no credentials or private data.
