#!/bin/sh
set -eu

IMAGE='archlinux:base-devel@sha256:68bfc3b0d277b08a99101dc9b94aaa03e5ae70cf1b4fb965c03b2b87b915760d'
ROOT='/tmp/auroscope-e2e-root'
DB='/tmp/auroscope-e2e-db'
CACHE='/tmp/auroscope-e2e-cache'

case "${AUROSCOPE_E2E_DISPOSABLE_ARCH:-}" in
  1) ;;
  *)
    printf '%s\n' 'Refusing to run without AUROSCOPE_E2E_DISPOSABLE_ARCH=1.' >&2
    exit 64
    ;;
esac

if ! command -v docker >/dev/null 2>&1; then
  printf '%s\n' 'Docker is not available; disposable Arch E2E cannot run here.' >&2
  exit 69
fi

case "$ROOT:$DB:$CACHE" in
  /tmp/auroscope-e2e-root:/tmp/auroscope-e2e-db:/tmp/auroscope-e2e-cache) ;;
  *)
    printf '%s\n' 'Internal private pacman paths changed unexpectedly; aborting.' >&2
    exit 70
    ;;
esac

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

docker run --rm -i \
  -e AUROSCOPE_E2E_INSIDE=1 \
  -v "$repo_root:/src:ro" \
  "$IMAGE" \
  /bin/bash <<'BASH'
set -euo pipefail

if [[ "${AUROSCOPE_E2E_INSIDE:-}" != 1 ]]; then
  echo 'not inside disposable container; aborting' >&2
  exit 70
fi

ROOT=/tmp/auroscope-e2e-root
DB=/tmp/auroscope-e2e-db
CACHE=/tmp/auroscope-e2e-cache
case "$ROOT:$DB:$CACHE" in
  /tmp/auroscope-e2e-root:/tmp/auroscope-e2e-db:/tmp/auroscope-e2e-cache) ;;
  *) echo 'private pacman path guard failed' >&2; exit 70 ;;
esac

mkdir -p "$ROOT" "$DB" "$CACHE" /work
pacman --noconfirm --needed -Sy git go

cp -a /src /work/auroscope
cd /work/auroscope
GOCACHE=/tmp/auroscope-gocache GOMODCACHE=/tmp/auroscope-gomodcache CGO_ENABLED=1 go test ./...
GOCACHE=/tmp/auroscope-gocache GOMODCACHE=/tmp/auroscope-gomodcache CGO_ENABLED=1 go build -o /tmp/auroscope ./cmd/auroscope

cat > /tmp/fake-codex <<'EOF'
#!/bin/sh
if test "$1" = "--version"; then
  printf 'codex-cli 0.150.1\n'
  exit 0
fi
test "$PWD" != /home/builder/aur/hello
test -f bundle.json
out=''
while test "$#" -gt 0; do
  if test "$1" = "--output-last-message"; then
    shift
    out="$1"
  fi
  shift || true
done
test -n "$out"
printf '{"event":"jsonl stdout ignored by AURoscope"}\n'
printf '{"summary":"disposable audit","risk":"low","findings":[],"uncertainty":"","inspect":["PKGBUILD"]}' > "$out"
EOF
chmod +x /tmp/fake-codex

cat > /tmp/fake-paru <<'EOF'
#!/bin/sh
set -eu
printf '%s\n' "$*" >> /tmp/auroscope-e2e-paru-calls
if test "$1 $2" = "-Syu --repo"; then
  exit 0
fi
if test "$1 $2" = "-Qua --quiet"; then
  printf 'hello\n'
  exit 0
fi
if test "$1 $2" = "-P --order"; then
  printf 'AUR TARGET hello hello\n'
  exit 0
fi
if test "$1" = "-G"; then
  mkdir -p "$PWD/$2"
  cd "$PWD/$2"
  git init >/dev/null
  git config user.name E2E
  git config user.email e2e@example.invalid
  printf 'pkgname=hello\npkgver=1\npkgrel=1\n' > PKGBUILD
  printf 'pkgbase = hello\n' > .SRCINFO
  git add .
  git commit -m initial >/dev/null
  exit 0
fi
if test "$1" = "-S"; then
  test -n "${PARU_CONF:-}"
  if printf '%s\n' "$*" | grep -q -- '--config'; then
    exit 88
  fi
  grep -q '^CloneDir = /tmp/auroscope-e2e-clones$' "$PARU_CONF"
  grep -q '^\[bin\]$' "$PARU_CONF"
  hook=$(sed -n 's/^PreBuildCommand = //p' "$PARU_CONF")
  cd /tmp/auroscope-e2e-clones/hello
  PKGBASE=hello sh -c "$hook"
  exit 0
fi
exit 0
EOF
chmod +x /tmp/fake-paru

rm -rf /tmp/auroscope-e2e-clones /tmp/auroscope-e2e-state.sqlite3
printf 'approve\n' | env \
  AUROSCOPE_PARU=/tmp/fake-paru \
  AUROSCOPE_CODEX=/tmp/fake-codex \
  AUROSCOPE_STATE=/tmp/auroscope-e2e-state.sqlite3 \
  AUROSCOPE_CLONE_DIR=/tmp/auroscope-e2e-clones \
  /tmp/auroscope -S hello \
  >/tmp/auroscope-e2e-stdout \
  2>/tmp/auroscope-e2e-stderr \
  || { cat /tmp/auroscope-e2e-stderr >&2; exit 1; }

grep -q -- '-S --skipreview hello' /tmp/auroscope-e2e-paru-calls
grep -q 'AUR audit: hello' /tmp/auroscope-e2e-stdout

echo 'disposable Arch fake-Paru integration smoke passed; real supported-version Paru E2E remains unresolved'
BASH
