#!/usr/bin/env bash
# Run only the official stable prebuilt Hugo release; never compile Hugo.
set -euo pipefail
version=0.167.0
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cache=${HUGO_CACHE_DIR:-$root/.cache/hugo}
case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) arch=amd64 ;;
  Linux-aarch64|Linux-arm64) arch=arm64 ;;
  *) echo 'hugo.sh: supported hosts are Linux AMD64 and ARM64' >&2; exit 1 ;;
esac
asset=hugo_${version}_linux-${arch}.tar.gz
sum=$(awk -v asset="$asset" '$2 == asset {print}' "$root/.config/hugo.sha256")
[ -n "$sum" ] || { echo "hugo.sh: missing checksum for $asset" >&2; exit 1; }
mkdir -p "$cache"
tmp=$(mktemp -d "$cache/tmp.XXXXXX")
trap 'rm -rf "$tmp"' EXIT
if [ ! -f "$cache/$asset" ]; then
  curl --proto '=https' --proto-redir '=https' --tlsv1.2 --fail --location \
    --silent --show-error --connect-timeout 10 --max-time 120 --retry 2 \
    "https://github.com/gohugoio/hugo/releases/download/v$version/$asset" -o "$tmp/$asset"
  (cd "$tmp" && printf '%s\n' "$sum" | sha256sum --check --status) || {
    echo "hugo.sh: downloaded archive checksum failed: $asset" >&2; exit 1;
  }
  mv "$tmp/$asset" "$cache/$asset"
fi
# Verify cached bytes on every run and extract afresh, rather than trust a cached executable.
(cd "$cache" && printf '%s\n' "$sum" | sha256sum --check --status) || {
  echo "hugo.sh: cached archive checksum failed; remove $cache/$asset and retry" >&2; exit 1;
}
tar -xzf "$cache/$asset" -C "$tmp" hugo
"$tmp/hugo" "$@"
