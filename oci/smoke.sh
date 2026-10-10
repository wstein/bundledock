#!/bin/sh
set -eu
image=${1:-anchorage/tools:poc}
consumer=${2:-mcr.microsoft.com/devcontainers/go:2-1.27-trixie}
image=$(docker image inspect --format '{{.Id}}' "$image")
volume=$(docker volume create)
cleanup() { docker volume rm "$volume" >/dev/null; }
trap cleanup EXIT

docker run --rm --network none --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$image"
# Repeated initialization must validate and preserve the same content.
docker run --rm --network none --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$image"

check() {
    docker run --rm --network none --user 1000:1000 \
        --tmpfs /home/anchorage:exec,uid=1000,gid=1000,mode=0700 \
        --mount "type=volume,src=$volume,dst=/opt/anchorage,readonly,volume-nocopy" \
        --env HOME=/home/anchorage --env DISABLE_UPDATES=1 --env DISABLE_AUTOUPDATER=1 \
        --entrypoint /bin/sh "$consumer" -ec '
            test -f /opt/anchorage/.ready
            export PATH="/opt/anchorage/bin:$PATH"
            codex --version
            timeout 45 codex app-server daemon start
            codex app-server daemon version
            codex app-server daemon stop
            claude --version
            agy --version
            gh --version
            sqlite3 --version
            test "$(sqlite3 :memory: "SELECT 6 * 7;")" = 42
            jq --version
            printf "%s\n" "{\"answer\":42}" | jq -e ".answer == 42"
            rg --version
            printf "%s\n" "anchorage smoke" > "$HOME/tool-smoke.txt"
            rg -qx "anchorage smoke" "$HOME/tool-smoke.txt"
            fd --version
            fdfind --version
            test "$(fd --type f --max-depth 1 "^tool-smoke[.]txt$" "$HOME")" = "$HOME/tool-smoke.txt"
            test "$(fdfind --type f --max-depth 1 "^tool-smoke[.]txt$" "$HOME")" = "$HOME/tool-smoke.txt"
            ast-grep --version
            printf "%s\n" "package main" "func main() {" "println(42)" "}" > "$HOME/tool-smoke.go"
            test "$(ast-grep run --lang go --pattern "func main() { println(42) }" --files-with-matches "$HOME/tool-smoke.go")" = "$HOME/tool-smoke.go"
            yq --version
            printf "%s\n" "answer: 42" | yq -e ".answer == 42"
            export UV_CACHE_DIR="$HOME/.cache/uv" UV_PYTHON_DOWNLOADS=never
            mkdir -p "$UV_CACHE_DIR"
            uv --version
            uvx --version
            test "$(uv --offline cache dir)" = "$UV_CACHE_DIR"
            if touch /opt/anchorage/write-test 2>/dev/null; then
                echo "Unexpected write to tool volume" >&2
                exit 1
            fi
        '
}
check &
first=$!
check &
second=$!
result=0
wait "$first" || result=1
wait "$second" || result=1
test "$result" = 0

# A changed manifest must be rejected, not overwritten by initialization.
docker run --rm --network none --entrypoint /bin/sh \
    --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$image" \
    -ec 'printf "invalid\n" > /bundle/manifest.json'
if docker run --rm --network none --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$image"; then
    echo 'Unexpected success for a mismatched volume' >&2
    exit 1
fi
docker run --rm --network none --entrypoint /bin/sh \
    --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$image" \
    -ec 'rm /bundle/.ready'
if docker run --rm --network none --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$image"; then
    echo 'Unexpected success for an incomplete volume' >&2
    exit 1
fi
docker run --rm --network none --entrypoint /bin/sh \
    --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$image" \
    -ec 'mkdir /bundle/.initializing'
if docker run --rm --network none --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$image"; then
    echo 'Unexpected success for a locked volume' >&2
    exit 1
fi
echo 'Shared-volume smoke test passed'
