#!/bin/sh
set -eu
# Offline, noninteractive checks use only a private temporary workspace.
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM
shellcheck --version
shfmt --version
just --version
ruff --version
delta --version
watchexec --version
hyperfine --version
age --version
age-keygen --version
test "$(printf '%s\n' '#!/bin/sh' 'echo smoke' | shellcheck --norc -s sh -)" = ''
test "$(printf '%s\n' 'echo  smoke' | shfmt -ln posix)" = 'echo smoke'
printf '%s\n' 'smoke:' '    @echo recipe-smoke' > "$work/justfile"
test "$(just --no-dotenv --justfile "$work/justfile" --working-directory "$work" smoke)" = recipe-smoke
test "$(printf '%s\n' 'x=1' | ruff format --isolated --no-cache --stdin-filename smoke.py -)" = 'x = 1'
printf '%s\n' '--- a/smoke' '+++ b/smoke' '@@ -1 +1 @@' '-before-smoke' '+after-smoke' | delta --no-gitconfig --paging=never > "$work/diff"
# Preserve the renderer; normalize intraline SGR and erase-line escapes only
# in the comparison copy. Piped rendering must still exit zero.
escape=$(printf '\033')
sed "s/${escape}\[[0-9;]*[mK]//g" "$work/diff" > "$work/diff-text"
grep -qx before-smoke "$work/diff-text"
grep -qx after-smoke "$work/diff-text"
test "$(watchexec -1 --shell=none --workdir "$work" -- echo watch-smoke)" = watch-smoke
hyperfine --warmup 0 --runs 1 --export-json "$work/benchmark.json" true
jq -e '
    .schema_version == 2 and (.results | length) == 1 and
    .results[0].command == "true" and
    (.results[0].measurements | length) == 1 and
    (.results[0].measurements[0] |
        .exit_code == 0 and .time_wall_clock.unit == "second" and
        (.time_wall_clock.value | type) == "number" and
        .time_wall_clock.value >= 0)
' "$work/benchmark.json" > /dev/null
age-keygen -o "$work/key.txt"
recipient=$(age-keygen -y "$work/key.txt")
case "$recipient" in age1*) ;; *) exit 1;; esac
printf '%s\n' encryption-smoke > "$work/plain"
age -r "$recipient" -o "$work/encrypted" "$work/plain"
age -d -i "$work/key.txt" -o "$work/decrypted" "$work/encrypted"
cmp "$work/plain" "$work/decrypted"
