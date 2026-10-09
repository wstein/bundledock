#!/bin/sh
set -eu

source=/opt/anchorage
target=/bundle
test -d "$target" || { echo 'Mount a named volume at /bundle' >&2; exit 1; }
# Atomic directory creation serializes writers. A failed initialization remains
# visibly incomplete and must use a new volume rather than overwriting files.
if ! mkdir "$target/.initializing" 2>/dev/null; then
    echo 'Volume initialization already running or previously failed' >&2
    exit 1
fi
trap 'rmdir "$target/.initializing"' EXIT
if test -f "$target/.ready"; then
    cmp "$source/manifest.json" "$target/manifest.json"
    cmp "$source/SHA256SUMS" "$target/SHA256SUMS"
    (cd "$target" && sha256sum -c SHA256SUMS)
    exit 0
fi
test "$(ls -A "$target")" = '.initializing' || {
    echo 'Refusing to initialize a nonempty volume' >&2
    exit 1
}
cp -a "$source/." "$target/"
(cd "$target" && sha256sum -c SHA256SUMS)
touch "$target/.ready"
echo 'Tool volume initialized'
