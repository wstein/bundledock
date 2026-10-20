# BundleDock gsync and gsync-all: one POSIX sh script, run by sh from a function
# in bash, zsh, fish and Nushell, so all four shells behave the same.
# Usage: sh -c <this script> bdock-gsync sync|all   (neither takes arguments)
# The text is embedded into every generated shell file in a quoted string (a
# Nushell raw string r#...#), so it must not contain a single quote directly
# followed by a hash sign; a test enforces that.

NL="
"
say() { printf "%s$NL" "$*"; }
fail() { printf "gsync: %s$NL" "$*" >&2; exit 1; }

in_work_tree() { [ "$(git -C "${1:-.}" rev-parse --is-inside-work-tree 2>/dev/null)" = true ]; }

# Refuse while a merge, rebase, cherry-pick or revert is unfinished.
check_idle() {
    for pair in rebase-merge:rebase rebase-apply:rebase MERGE_HEAD:merge CHERRY_PICK_HEAD:cherry-pick REVERT_HEAD:revert; do
        if [ -e "$(git rev-parse --git-path "${pair%%:*}")" ]; then
            fail "${pair##*:} in progress: finish or abort it first (git ${pair##*:} --continue or --abort)"
        fi
    done
}

# Print a word so that pasting it into a shell yields the same word: bare when
# it is plain, else POSIX single-quoted with each embedded quote closed,
# escaped and reopened.
shq() {
    case $1 in
    "" | *[!A-Za-z0-9._/-]*) printf "%s" "'$(printf "%s" "$1" | sed "s/'/'\\\\''/g")'" ;;
    *) printf "%s" "$1" ;;
    esac
}

# gsync: fetch with pruning, then rebase the current branch onto its upstream.
sync_one() {
    [ $# -eq 0 ] || fail "usage: gsync (takes no arguments)"
    in_work_tree || fail "not inside a Git work tree"
    check_idle
    if ! git symbolic-ref --quiet HEAD >/dev/null 2>&1; then
        git fetch --prune --all || fail "fetch failed"
        printf "gsync: detached HEAD: fetched and pruned, but not pulling; switch to a branch first$NL" >&2
        exit 2
    fi
    git fetch --prune --all || fail "fetch failed"
    git pull --rebase --autostash
}

# gsync-all: one status line per repository, fetch --prune only.
report_one() {
    dir=$1
    if [ "$dir" != . ] && ! [ -e "$dir/.git" ]; then
        say "$dir: not checked out (git submodule update --init $(shq "$dir"))"
        return 0
    fi
    if ! in_work_tree "$dir"; then
        say "$dir: not a Git work tree"
        status=1
        return 0
    fi
    note=
    git -C "$dir" fetch --prune || { note=" (fetch failed)"; status=1; }
    branch=$(git -C "$dir" symbolic-ref --quiet --short HEAD 2>/dev/null) ||
        branch="detached HEAD at $(git -C "$dir" rev-parse --short HEAD 2>/dev/null)"
    upstream=$(git -C "$dir" rev-parse --abbrev-ref "@{upstream}" 2>/dev/null) || upstream=
    behind=0
    if [ -n "$upstream" ]; then
        counts=$(git -C "$dir" rev-list --left-right --count "HEAD...@{upstream}")
        ahead=${counts%%[[:space:]]*}
        behind=${counts##*[[:space:]]}
        position="ahead $ahead/behind $behind of $upstream"
    else
        if [ -n "$(git -C "$dir" config --get "branch.$branch.merge" 2>/dev/null)" ]; then
            position="upstream gone"
        else
            position="no upstream"
        fi
    fi
    if [ -n "$(git -C "$dir" status --porcelain 2>/dev/null)" ]; then
        state=dirty
    else
        state=clean
    fi
    say "$dir: $branch, $position, $state$note"
    if [ "$behind" -gt 0 ]; then
        say "  behind: run  git -C $(shq "$dir") pull --rebase --autostash"
    fi
}

# A .gitmodules path is data from the repository, so it is confined to the
# root: no absolute path, no leading dash, no ".." component, and a real path
# that stays under the root (a symlink cannot lead out).
confined() {
    case $1 in
    "" | /* | -*) return 1 ;;
    esac
    case "/$1/" in
    */../*) return 1 ;;
    esac
    [ -e "$1" ] || return 0
    real=$(cd "$1" 2>/dev/null && pwd -P) || return 1
    case "$real/" in
    "$rootreal"/*) return 0 ;;
    esac
    return 1
}

sync_all() {
    [ $# -eq 0 ] || fail "usage: gsync-all (takes no arguments)"
    in_work_tree || fail "not inside a Git work tree"
    root=$(git rev-parse --show-toplevel) || fail "cannot find the work tree root"
    cd "$root" || fail "cannot enter $root"
    rootreal=$(pwd -P)
    say "gsync-all: workspace root $root"
    status=0
    report_one .
    if [ -f .gitmodules ]; then
        paths=$(git config --file .gitmodules --name-only --get-regexp "[.]path$")
        while read -r line; do
            [ -n "$line" ] || continue
            path=$(git config --file .gitmodules --get "$line") || continue
            if confined "$path"; then
                report_one "$path"
            else
                say "$path: skipped (path is outside the workspace root)"
                status=1
            fi
        done <<PATHS
$paths
PATHS
    fi
    return "$status"
}

mode=$1
shift
case $mode in
sync) sync_one "$@" ;;
all) sync_all "$@" ;;
*) fail "unknown mode $mode" ;;
esac
