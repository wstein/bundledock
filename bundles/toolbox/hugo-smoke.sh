#!/bin/sh
set -eu
# OCI callers enforce --network none and supply writable HOME outside the bundle.
# Use an explicit binary so a consumer's PATH cannot shadow the installed Hugo.
hugo=${1:-/opt/bundledock/toolbox/bin/hugo}
version=$("$hugo" version)
printf '%s\n' "$version"
case "$version" in
    'hugo v'*' linux/'*) ;;
    *) echo 'Expected the official Linux Hugo binary' >&2; exit 1 ;;
esac
case "$version" in
    *extended*) echo 'Expected standard Hugo, not extended' >&2; exit 1 ;;
esac
fixture="$HOME/hugo-smoke"
mkdir -p "$fixture/content" "$fixture/layouts" "$HOME/.cache/hugo"
printf '%s\n' 'baseURL = "https://example.invalid/"' 'title = "BundleDock smoke"' > "$fixture/hugo.toml"
printf '%s\n' '# Bundled Hugo' '' 'Offline static site build.' > "$fixture/content/_index.md"
printf '%s\n' '<!doctype html><html><head><title>{{ .Site.Title }}</title></head><body>{{ .Content }}</body></html>' > "$fixture/layouts/index.html"
"$hugo" --source "$fixture" --destination "$fixture/public" \
    --cacheDir "$HOME/.cache/hugo" --disableKinds taxonomy,term --panicOnWarning
grep -q '<h1 id="bundled-hugo">Bundled Hugo</h1>' "$fixture/public/index.html"
grep -q '<p>Offline static site build.</p>' "$fixture/public/index.html"
