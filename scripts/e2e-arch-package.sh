#!/bin/sh
set -eu

IMAGE='archlinux:base-devel@sha256:68bfc3b0d277b08a99101dc9b94aaa03e5ae70cf1b4fb965c03b2b87b915760d'
PARU_COMMIT='9ac3578807a87858651e81a02586ceb947686e7c'
CODEX_VERSION='0.151.0'
CODEX_SHA256='605b4b183f22c645f5def63a5b7191767407fb66a6feaec4eaf10b5b7e0058f6'

if [ "${AUROSCOPE_E2E_ARCH_PACKAGE:-}" != 1 ]; then
  printf '%s\n' 'Refusing to run without AUROSCOPE_E2E_ARCH_PACKAGE=1.' >&2
  exit 64
fi
if ! command -v docker >/dev/null 2>&1; then
  printf '%s\n' 'Docker is required for the Arch package E2E.' >&2
  exit 69
fi

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

docker run --rm -i \
  -e AUROSCOPE_E2E_INSIDE=1 \
  -e PARU_COMMIT="$PARU_COMMIT" \
  -e CODEX_VERSION="$CODEX_VERSION" \
  -e CODEX_SHA256="$CODEX_SHA256" \
  -v "$repo_root:/src:ro" \
  "$IMAGE" /bin/bash <<'BASH'
set -euo pipefail

[[ "${AUROSCOPE_E2E_INSIDE:-}" == 1 ]]
[[ "$PWD" == / ]]

# All package-manager activity stays inside this disposable container.
pacman --noconfirm --needed -Sy cargo curl git go namcap sudo
useradd --create-home --shell /bin/bash builder
printf 'builder ALL=(ALL:ALL) NOPASSWD: ALL\n' >/etc/sudoers.d/auroscope-package-e2e
chmod 0440 /etc/sudoers.d/auroscope-package-e2e

# Build the exact post-2.1.0 Paru contract without executing an AUR PKGBUILD.
git clone --quiet https://github.com/Morganamilo/paru.git /work/paru
git -C /work/paru checkout --quiet "$PARU_COMMIT"
[[ "$(git -C /work/paru rev-parse HEAD)" == "$PARU_COMMIT" ]]
cd /work/paru
cargo update alpm alpm-utils
cargo build --release
install -Dm755 target/release/paru /usr/local/bin/paru-supported

# Install and verify the exact Codex CLI release accepted by AURoscope.
curl -fsSL \
  "https://github.com/openai/codex/releases/download/rust-v${CODEX_VERSION}/codex-x86_64-unknown-linux-musl.tar.gz" \
  -o /tmp/codex.tar.gz
printf '%s  %s\n' "$CODEX_SHA256" /tmp/codex.tar.gz | sha256sum --check --strict
mkdir -p /tmp/codex
bsdtar -xf /tmp/codex.tar.gz -C /tmp/codex
install -Dm755 /tmp/codex/codex-x86_64-unknown-linux-musl /usr/local/bin/codex-supported
[[ "$(/usr/local/bin/codex-supported --version)" == "codex-cli ${CODEX_VERSION}" ]]

# Build only AURoscope's trusted repository PKGBUILD. Runtime AUR dependencies
# are installed above from pinned upstream source/binary, so makepkg does not
# resolve or execute any third-party PKGBUILD.
cp -a /src/packaging/arch /home/builder/auroscope-package
chown -R builder:builder /home/builder/auroscope-package
sudo -u builder -- bash -lc \
  'cd /home/builder/auroscope-package && makepkg --printsrcinfo >.SRCINFO.generated && cmp .SRCINFO .SRCINFO.generated'
sudo -u builder -- bash -lc \
  'cd /home/builder/auroscope-package && makepkg --cleanbuild --clean --nodeps --noconfirm'

package_file=$(printf '%s\n' /home/builder/auroscope-package/auroscope-*.pkg.tar.zst)
[[ -f "$package_file" ]]
namcap /home/builder/auroscope-package/PKGBUILD >/tmp/namcap-pkgbuild
namcap "$package_file" >/tmp/namcap-package
cat /tmp/namcap-pkgbuild /tmp/namcap-package
if grep -q ' E: ' /tmp/namcap-pkgbuild /tmp/namcap-package; then
  printf '%s\n' 'namcap reported a packaging error' >&2
  exit 1
fi

pacman --noconfirm \
  --assume-installed 'paru-git=2.1.0.r67.g9ac3578' \
  --assume-installed "openai-codex-bin=${CODEX_VERSION}" \
  -U "$package_file"
pacman -Q auroscope
pacman -Qk auroscope
[[ -x /usr/bin/auroscope ]]
readelf -h /usr/bin/auroscope | grep -q 'Type:.*DYN'
ldd /usr/bin/auroscope | grep -q 'libc.so.6'

cat >/usr/local/bin/paru-e2e <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >>/tmp/auroscope-package-paru-calls
exec /usr/local/bin/paru-supported "$@"
EOF
chmod 0755 /usr/local/bin/paru-e2e

cat >/usr/local/bin/codex-must-not-run <<'EOF'
#!/bin/sh
printf '%s\n' 'Codex was invoked for an official-only transaction' >&2
exit 97
EOF
chmod 0755 /usr/local/bin/codex-must-not-run

cat >/usr/local/bin/codex-e2e <<'EOF'
#!/bin/sh
set -eu
if [ "${1:-}" = --version ]; then
  printf '%s\n' 'codex-cli 0.151.0'
  exit 0
fi
out=''
while [ "$#" -gt 0 ]; do
  if [ "$1" = --output-last-message ]; then
    shift
    out=$1
  fi
  shift || true
done
[ -n "$out" ]
[ -f bundle.json ]
printf '%s\n' audit >>/tmp/auroscope-package-codex-calls
if [ "${AUROSCOPE_E2E_DRIFT:-}" = 1 ]; then
  printf '\n# post-audit drift\n' >>/home/builder/aur/hello/PKGBUILD
  git -C /home/builder/aur/hello \
    -c user.name=E2E -c user.email=e2e@example.invalid add PKGBUILD
  git -C /home/builder/aur/hello \
    -c user.name=E2E -c user.email=e2e@example.invalid commit -m drift >/dev/null
fi
printf '%s' '{"summary":"packaged binary audit","risk":"low","findings":[],"uncertainty":"","inspect":["PKGBUILD"]}' >"$out"
EOF
chmod 0755 /usr/local/bin/codex-e2e

install -d -m 0700 -o builder -g builder /home/builder/aur /home/builder/state
: >/tmp/auroscope-package-paru-calls
: >/tmp/auroscope-package-codex-calls
chmod 0666 /tmp/auroscope-package-paru-calls /tmp/auroscope-package-codex-calls

run_as_builder() {
  sudo -u builder -- env \
    HOME=/home/builder \
    PATH=/usr/local/bin:/usr/bin \
    AUROSCOPE_PARU=/usr/local/bin/paru-e2e \
    AUROSCOPE_CLONE_DIR=/home/builder/aur \
    AUROSCOPE_STATE=/home/builder/state/state.sqlite3 \
    "$@"
}

# The installed binary preserves an official-only transaction and never calls Codex.
run_as_builder env AUROSCOPE_CODEX=/usr/local/bin/codex-must-not-run \
  /usr/bin/auroscope -S --repo --noconfirm tree
pacman -Q tree

# The installed binary audits a real AUR worktree and reaches the real Paru guard.
{ printf '1\n'; sleep 2; printf 'approve\n'; } | \
  run_as_builder env AUROSCOPE_CODEX=/usr/local/bin/codex-e2e \
    /usr/bin/auroscope hello
pacman -Q hello
[[ "$(wc -l </tmp/auroscope-package-codex-calls)" -eq 1 ]]
grep -q -- '-S --skipreview -- hello' /tmp/auroscope-package-paru-calls

# A mutation committed after the audit must be rejected by the packaged guard.
if printf 'approve\n' | run_as_builder env \
  AUROSCOPE_CODEX=/usr/local/bin/codex-e2e \
  AUROSCOPE_E2E_DRIFT=1 \
  /usr/bin/auroscope -S --noconfirm --rebuild hello; then
  printf '%s\n' 'packaged guard accepted post-audit recipe drift' >&2
  exit 1
fi

pacman --noconfirm -R auroscope
[[ ! -e /usr/bin/auroscope ]]
[[ -f /home/builder/state/state.sqlite3 ]]

printf '%s\n' 'self-hosted Arch package build/install/passthrough/guard/uninstall E2E passed'
BASH
