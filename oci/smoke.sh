#!/bin/sh
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
image=${1:-bundledock-toolbox:poc}
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
        --tmpfs /home/bundledock:exec,uid=1000,gid=1000,mode=0700 \
        --mount "type=volume,src=$volume,dst=/opt/workharbor,readonly,volume-nocopy" \
        --mount "type=bind,src=$script_dir/quality-security-smoke.sh,dst=/quality-security-smoke.sh,readonly" \
        --mount "type=bind,src=$script_dir/process-monitors-smoke.sh,dst=/process-monitors-smoke.sh,readonly" \
        --env HOME=/home/bundledock --env DISABLE_UPDATES=1 --env DISABLE_AUTOUPDATER=1 \
        --entrypoint /bin/sh "$consumer" -ec '
            test -f /opt/workharbor/.ready
            export PATH="/opt/workharbor/bin:$PATH"
            codex --version
            timeout 45 codex app-server daemon start
            codex app-server daemon version
            codex app-server daemon stop
            claude --version
            agy --version
            gh --version
            herdr --version
            fish --version
            test "$(fish --no-config -c "math \"6*7\"")" = 42
            fish --no-config -c "printf \"%s\\n\" \"{\\\"answer\\\":42}\" | jq -e \".answer == 42\""
            fish --no-config -c "type -q fish_prompt; and complete -C \"fish --no\" | string match --quiet \"*--no-config*\""
            nu --version
            test "$(nu --no-config-file --no-history -c "[{answer: 41} {answer: 42}] | where answer > 41 | get answer | first")" = 42
            tmux -V
            export TERM=xterm-256color
            tmux_socket="$HOME/tool-smoke-tmux.sock"
            trap "tmux -S \"$tmux_socket\" kill-server 2>/dev/null || :" EXIT
            tmux -S "$tmux_socket" -f /dev/null start-server \; \
                set-option -g default-shell /bin/bash \; \
                new-session -d -s bundledock-smoke "exec /bin/bash --noprofile --norc -c \"printf 42 > $HOME/tmux-answer; exec sleep 30\""
            test "$(tmux -S "$tmux_socket" show-options -gv default-shell)" = /bin/bash
            attempts=0
            until test -f "$HOME/tmux-answer"; do
                attempts=$((attempts + 1))
                test "$attempts" -lt 50
                sleep 0.1
            done
            test "$(cat "$HOME/tmux-answer")" = 42
            tmux -S "$tmux_socket" kill-server
            trap - EXIT
            starship --version
            export TERM=xterm-256color
            export STARSHIP_CONFIG="$HOME/starship.toml" STARSHIP_CACHE="$HOME/.cache/starship"
            printf "%s\n" "add_newline = false" "format = \"\$character\"" "[character]" "success_symbol = \">\"" > "$STARSHIP_CONFIG"
            test "$(starship prompt --path "$HOME" --status 0)" = "> "
            fish --no-config -c "starship init fish | source; functions -q fish_prompt"
            jq --version
            printf "%s\n" "{\"answer\":42}" | jq -e ".answer == 42"
            rg --version
            printf "%s\n" "bundledock smoke" > "$HOME/tool-smoke.txt"
            rg -qx "bundledock smoke" "$HOME/tool-smoke.txt"
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
            sh /quality-security-smoke.sh
            sh /process-monitors-smoke.sh
            if touch /opt/workharbor/write-test 2>/dev/null; then
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
