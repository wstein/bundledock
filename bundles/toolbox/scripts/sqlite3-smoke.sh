#!/bin/sh
set -eu
# Optional $1: the version the lock pins, which `sqlite3 --version` must report.
# Runs in memory only, with a scrubbed environment and no terminal: never reads or
# writes a user database, rc file or history.
expected=${1:-}
home=$(mktemp -d "${TMPDIR:-/tmp}/bundledock-sqlite3.XXXXXX")
trap 'rm -rf "$home"' EXIT HUP INT TERM
fail() { printf 'sqlite3 smoke: %s\n' "$1" >&2; exit 1; }
# run [VAR=value ...] command ...: PATH, an empty HOME and TMPDIR only, plus the given variables.
run() {
    env -i PATH="$PATH" HOME="$home" TMPDIR="$home" "$@"
}
q() { run sqlite3 :memory: "$1" </dev/null; }

version=$(run sqlite3 --version) || fail "--version failed"
printf '%s\n' "$version"
printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+ [0-9]{4}-[0-9]{2}-[0-9]{2} ' || fail "unexpected version line"
if [ -n "$expected" ]; then
    [ "${version%% *}" = "$expected" ] || fail "version ${version%% *} differs from the locked $expected"
fi

[ "$(q 'create table t(a); insert into t values(41); select a+1 from t;')" = 42 ] || fail "create, insert and select"
options=$(q 'pragma compile_options;') || fail "compile_options"
for option in THREADSAFE=1 ENABLE_FTS5 ENABLE_RTREE ENABLE_DBSTAT_VTAB ENABLE_MATH_FUNCTIONS OMIT_LOAD_EXTENSION; do
    printf '%s\n' "$options" | grep -Fxq "$option" || fail "compile option $option is missing"
done
[ "$(q "create virtual table f using fts5(x); insert into f values('hello world'); select count(*) from f where f match 'hello';")" = 1 ] || fail "FTS5"
[ "$(q 'create virtual table r using rtree(id, x0, x1); insert into r values(1, 0, 1); select count(*) from r;')" = 1 ] || fail "RTREE"
[ "$(q 'select sqrt(16);')" = 4.0 ] || fail "math functions"
[ "$(q 'create table t(a); select count(*) > 0 from dbstat;')" = 1 ] || fail "dbstat"
# Extension loading is compiled out: the function must not exist.
if q "select load_extension('/nonexistent');" >/dev/null 2>&1; then fail "load_extension is available"; fi

# No terminal: the line editor must not be needed for piped input, with TERM unset or dumb.
[ "$(printf 'select 1+1;\n' | run sqlite3 :memory:)" = 2 ] || fail "piped input without TERM"
[ "$(printf 'select 1+1;\n' | run TERM=dumb sqlite3 :memory:)" = 2 ] || fail "piped input with TERM=dumb"
run TERM=dumb sqlite3 :memory: 'select 1' </dev/null >/dev/null || fail "statement argument with TERM=dumb"
