#!/bin/sh
set -eu
# Offline, noninteractive checks use only a private temporary workspace.
# Labels describe operations only: never trace arguments or captured values.
smoke_stage='temporary workspace'
stage() {
    smoke_stage=$1
    printf 'Quality utilities smoke stage: %s\n' "$smoke_stage" >&2
}
finish() {
    status=$?
    if test "$status" -ne 0; then
        printf 'Quality utilities smoke failed: %s (exit %s)\n' "$smoke_stage" "$status" >&2
    fi
    if test -n "${work:-}"; then rm -rf "$work"; fi
    exit "$status"
}
trap finish EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
stage 'temporary workspace'
work=$(mktemp -d)
stage 'shellcheck version'
shellcheck --version
stage 'shfmt version'
shfmt --version
stage 'just version'
just --version
stage 'ruff version'
ruff --version
stage 'delta version'
delta --version
stage 'watchexec version'
watchexec --version
stage 'hyperfine version'
hyperfine --version
stage 'age version'
age --version
stage 'age-keygen version'
age-keygen --version
stage 'shell fixture'
printf '%s\n' '#!/bin/sh' 'echo smoke' > "$work/smoke.sh"
stage 'shellcheck analysis'
output=$(shellcheck --norc -s sh - < "$work/smoke.sh")
stage 'shellcheck output assertion'
test "$output" = ''
stage 'shfmt fixture'
printf '%s\n' 'echo  smoke' > "$work/unformatted.sh"
stage 'shfmt formatting'
output=$(shfmt -ln posix < "$work/unformatted.sh")
stage 'shfmt output assertion'
test "$output" = 'echo smoke'
stage 'just fixture'
printf '%s\n' 'smoke:' '    @echo recipe-smoke' > "$work/justfile"
# just 1.58.0 --quiet suppresses recipe stdout; @ already hides command echo.
stage 'just recipe'
output=$(just --no-dotenv --justfile "$work/justfile" --working-directory "$work" smoke)
stage 'just output assertion'
test "$output" = recipe-smoke
stage 'ruff fixture'
printf '%s\n' 'x=1' > "$work/smoke.py"
stage 'ruff formatting'
output=$(ruff format --isolated --no-cache --stdin-filename smoke.py - < "$work/smoke.py")
stage 'ruff output assertion'
test "$output" = 'x = 1'
stage 'delta fixture'
printf '%s\n' '--- a/smoke' '+++ b/smoke' '@@ -1 +1 @@' '-before-smoke' '+after-smoke' > "$work/input.diff"
stage 'delta rendering'
delta --no-gitconfig --paging=never < "$work/input.diff" > "$work/diff"
# Delta 0.20.1 inserts SGR escapes inside words during intraline highlighting.
# Keep the real renderer and its exit status; remove SGR/erase-line sequences
# only from the comparison copy, not by using --raw or accepting diff status 1.
stage 'delta escape character'
escape=$(printf '\033')
stage 'delta comparison normalization'
sed "s/${escape}\[[0-9;]*[mK]//g" "$work/diff" > "$work/diff-text"
stage 'delta removed text assertion'
grep -qx before-smoke "$work/diff-text"
stage 'delta added text assertion'
grep -qx after-smoke "$work/diff-text"
# The pinned watchexec exposes its testing-only run-once mode as -1.
stage 'watchexec command'
output=$(watchexec -1 --shell=none --workdir "$work" -- echo watch-smoke)
stage 'watchexec output assertion'
test "$output" = watch-smoke
stage 'hyperfine benchmark'
hyperfine --warmup 0 --runs 1 --export-json "$work/benchmark.json" true
stage 'hyperfine JSON assertion'
jq -e '
    .schema_version == 2 and (.results | length) == 1 and
    .results[0].command == "true" and
    (.results[0].measurements | length) == 1 and
    (.results[0].measurements[0] |
        .exit_code == 0 and .time_wall_clock.unit == "second" and
        (.time_wall_clock.value | type) == "number" and
        .time_wall_clock.value >= 0)
' "$work/benchmark.json" > /dev/null
stage 'age key generation'
age-keygen -o "$work/key.txt"
stage 'age recipient derivation'
recipient=$(age-keygen -y "$work/key.txt")
stage 'age recipient assertion'
case "$recipient" in age1*) ;; *) exit 1;; esac
stage 'age plaintext fixture'
printf '%s\n' encryption-smoke > "$work/plain"
stage 'age encryption'
age -r "$recipient" -o "$work/encrypted" "$work/plain"
stage 'age decryption'
age -d -i "$work/key.txt" -o "$work/decrypted" "$work/encrypted"
stage 'age roundtrip assertion'
cmp "$work/plain" "$work/decrypted"
