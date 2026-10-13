#!/bin/sh
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
image=${1:-bundledock-toolbox:poc}
consumer=${2:-mcr.microsoft.com/devcontainers/go:2-1.27-trixie}
printf '%s\n' 'Toolbox smoke stage: inspect carrier image' >&2
image=$(docker image inspect --format '{{.Id}}' "$image")
printf '%s\n' 'Toolbox smoke stage: create shared volume' >&2
volume=$(docker volume create)
cleanup() { docker volume rm "$volume" >/dev/null; }
trap cleanup EXIT

printf '%s\n' 'Toolbox smoke stage: initialize shared volume' >&2
docker run --rm --network none --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$image"
# Repeated initialization must validate and preserve the same content.
docker run --rm --network none --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$image"

check() {
    docker run --rm --network none --user 1000:1000 \
        --tmpfs /home/bundledock:exec,uid=1000,gid=1000,mode=0700 \
        --mount "type=volume,src=$volume,dst=/opt/bdock/toolbox,readonly,volume-nocopy" \
        --mount "type=bind,src=$script_dir/quality-security-smoke.sh,dst=/quality-security-smoke.sh,readonly" \
        --mount "type=bind,src=$script_dir/quality-utilities-smoke.sh,dst=/quality-utilities-smoke.sh,readonly" \
        --mount "type=bind,src=$script_dir/mise-smoke.sh,dst=/mise-smoke.sh,readonly" \
        --mount "type=bind,src=$script_dir/utilities-smoke.sh,dst=/utilities-smoke.sh,readonly" \
        --mount "type=bind,src=$script_dir/helix-smoke.sh,dst=/helix-smoke.sh,readonly" \
        --mount "type=bind,src=$script_dir/vcs-versions-smoke.sh,dst=/vcs-versions-smoke.sh,readonly" \
        --mount "type=bind,src=$script_dir/leaf-smoke.sh,dst=/leaf-smoke.sh,readonly" \
        --mount "type=bind,src=$script_dir/jujutsu-smoke.sh,dst=/jujutsu-smoke.sh,readonly" \
        --mount "type=bind,src=$script_dir/hugo-smoke.sh,dst=/hugo-smoke.sh,readonly" \
        --mount "type=bind,src=$script_dir/process-monitors-smoke.sh,dst=/process-monitors-smoke.sh,readonly" \
        --mount "type=bind,src=$script_dir/git-trim-smoke.sh,dst=/git-trim-smoke.sh,readonly" \
        --env HOME=/home/bundledock --env DISABLE_UPDATES=1 --env DISABLE_AUTOUPDATER=1 \
        --entrypoint /bin/sh "$consumer" -ec '
            stage() { printf "Toolbox consumer smoke stage: %s\n" "$1" >&2; }
            stage "ready manifest and standalone agent exclusion"
            test -f /opt/bdock/toolbox/.ready
            for agent in codex codex-code-mode-host agy claude; do
                test ! -e "/opt/bdock/toolbox/bin/$agent"
            done
            test ! -e /opt/bdock/toolbox/codex-resources
            test ! -e /opt/bdock/toolbox/codex-path
            test ! -e /opt/bdock/toolbox/codex-package.json
            export PATH="/opt/bdock/toolbox/bin:$PATH"
            stage "GitHub CLI"
            gh --version
            stage "herdr"
            herdr --version
            stage "fish commands and completion"
            fish --version
            test "$(fish --no-config -c "math \"6*7\"")" = 42
            fish --no-config -c "printf \"%s\\n\" \"{\\\"answer\\\":42}\" | jq -e \".answer == 42\""
            fish --no-config -c "type -q fish_prompt; and complete -C \"fish --no\" | string match --quiet \"*--no-config*\""
            stage "Nushell structured commands"
            nu --version
            test "$(nu --no-config-file --no-history -c "[{answer: 41} {answer: 42}] | where answer > 41 | get answer | first")" = 42
            stage "tmux session and shell"
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
            stage "Starship prompt"
            starship --version
            export TERM=xterm-256color
            export STARSHIP_CONFIG="$HOME/starship.toml" STARSHIP_CACHE="$HOME/.cache/starship"
            printf "%s\n" "add_newline = false" "format = \"\$character\"" "[character]" "success_symbol = \">\"" > "$STARSHIP_CONFIG"
            test "$(starship prompt --path "$HOME" --status 0)" = "> "
            fish --no-config -c "starship init fish | source; functions -q fish_prompt"
            stage "jq JSON"
            jq --version
            printf "%s\n" "{\"answer\":42}" | jq -e ".answer == 42"
            stage "ripgrep"
            rg --version
            printf "%s\n" "bundledock smoke" > "$HOME/tool-smoke.txt"
            rg -qx "bundledock smoke" "$HOME/tool-smoke.txt"
            stage "fd and fdfind"
            fd --version
            fdfind --version
            test "$(fd --type f --max-depth 1 "^tool-smoke[.]txt$" "$HOME")" = "$HOME/tool-smoke.txt"
            test "$(fdfind --type f --max-depth 1 "^tool-smoke[.]txt$" "$HOME")" = "$HOME/tool-smoke.txt"
            stage "ast-grep syntax search"
            ast-grep --version
            printf "%s\n" "package main" "func main() {" "println(42)" "}" > "$HOME/tool-smoke.go"
            test "$(ast-grep run --lang go --pattern "func main() { println(42) }" --files-with-matches "$HOME/tool-smoke.go")" = "$HOME/tool-smoke.go"
            stage "yq YAML"
            yq --version
            printf "%s\n" "answer: 42" | yq -e ".answer == 42"
            export UV_CACHE_DIR="$HOME/.cache/uv" UV_PYTHON_DOWNLOADS=never
            mkdir -p "$UV_CACHE_DIR"
            stage "uv and uvx"
            uv --version
            uvx --version
            # First use resolves packages online; offline smoke checks staging only.
            test -x /opt/bdock/toolbox/bin/specify
            test "$(uv --offline cache dir)" = "$UV_CACHE_DIR"
            stage "mise version"
            sh /mise-smoke.sh
            stage "fzf bat and registry utilities"
            sh /utilities-smoke.sh
            stage "shell formatting recipes Python diffs watching benchmarks and encryption"
            sh /quality-utilities-smoke.sh
            stage "Helix editor"
            sh /helix-smoke.sh
            stage "noninteractive VCS versions"
            sh /vcs-versions-smoke.sh
            stage "Jujutsu isolated Git-backed workspace"
            sh /jujutsu-smoke.sh
            stage "Leaf Markdown preview"
            sh /leaf-smoke.sh
            stage "Hugo"
            sh /hugo-smoke.sh
            stage "quality and security tools"
            sh /quality-security-smoke.sh
            stage "process monitors"
            sh /process-monitors-smoke.sh
            stage "git-trim isolated dry-run"
            sh /git-trim-smoke.sh
            stage "read-only toolbox volume"
            if touch /opt/bdock/toolbox/write-test 2>/dev/null; then
                echo "Unexpected write to tool volume" >&2
                exit 1
            fi
        '
}
printf '%s\n' 'Toolbox smoke stage: parallel consumer checks' >&2
check &
first=$!
check &
second=$!
result=0
wait "$first" || result=1
wait "$second" || result=1
if [ "$result" != 0 ]; then
    echo "Toolbox smoke failed: parallel consumer checks (see the last named consumer stages above)." >&2
    exit "$result"
fi

printf '%s\n' 'Toolbox smoke stage: reject mismatched, incomplete and locked volumes' >&2
# A changed manifest must be rejected, not overwritten by initialization.
docker run --rm --network none --entrypoint /bin/sh \
    --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$consumer" \
    -ec 'printf "invalid\n" > /bundle/manifest.json'
if docker run --rm --network none --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$image"; then
    echo 'Unexpected success for a mismatched volume' >&2
    exit 1
fi
docker run --rm --network none --entrypoint /bin/sh \
    --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$consumer" \
    -ec 'rm /bundle/.ready'
if docker run --rm --network none --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$image"; then
    echo 'Unexpected success for an incomplete volume' >&2
    exit 1
fi
docker run --rm --network none --entrypoint /bin/sh \
    --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$consumer" \
    -ec 'mkdir /bundle/.initializing'
if docker run --rm --network none --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$image"; then
    echo 'Unexpected success for a locked volume' >&2
    exit 1
fi
echo 'Shared-volume smoke test passed'
