# BundleDock shared shell layer. Generated from internal/shelllayer/shortcuts.tsv; do not edit.
# Loaded automatically when <toolbox>/share is in XDG_DATA_DIRS.
# Opt out: BDOCK_SHORTCUTS=0 (abbreviations), BDOCK_STARSHIP=0 (prompt), BDOCK_FASTFETCH=0 (startup greeting), BDOCK_ZOXIDE=0 (cd/cdi), BDOCK_UMASK=0, BDOCK_EDITOR=0.
status is-interactive; or return 0

if test "$BDOCK_UMASK" != 0
    switch (umask)
        case 0000 000
            umask 022
    end
end

if test "$BDOCK_EDITOR" != 0; and test -z "$EDITOR"; and test "$TERM_PROGRAM" != vscode; and test -z "$VSCODE_IPC_HOOK_CLI"; and test "$TERMINAL_EMULATOR" != JetBrains-JediTerm; and test -z "$JETBRAINS_REMOTE_RUN"; and test -z "$IDEA_INITIAL_DIRECTORY"; and command -q hx
    set -gx EDITOR hx
end

if test "$BDOCK_FASTFETCH" != 0; and test "$__bdock_fastfetch_pid" != "$fish_pid"; and command -q fastfetch
    set -g __bdock_fastfetch_pid $fish_pid
    command fastfetch; or true
end

if test "$BDOCK_STARSHIP" != 0; and not set -q STARSHIP_SHELL; and command -q starship
    starship init fish | source
end

# Capture output before sourcing: a failing initializer may print partial code.
if test "$BDOCK_ZOXIDE" != 0; and not functions -q __zoxide_z; and not functions -q __zoxide_hook; and command -q zoxide
    set -l __bdock_zoxide_init (command zoxide init fish --cmd cd)
    if test $status -eq 0
        printf '%s\n' $__bdock_zoxide_init | source
    end
end

test "$BDOCK_SHORTCUTS" != 0; or return 0

function bdock-git-branch -d 'Current Git branch; fails on a detached HEAD'
    command git symbolic-ref --quiet --short HEAD; and return 0
    echo 'bdock: not on a branch (detached HEAD)' >&2
    return 1
end

function bdock-git-default-branch -d 'origin/HEAD, else main, else master'
    if set -l ref (command git symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null)
        string replace -r '^origin/' '' -- $ref
    else if command git show-ref --quiet --verify refs/heads/main
        echo main
    else
        echo master
    end
end

abbr --add --global -- g 'git'
abbr --add --global -- ga 'git add'
abbr --add --global -- gaa 'git add --all'
abbr --add --global -- gapa 'git add --patch'
abbr --add --global -- gau 'git add --update'
abbr --add --global -- gb 'git branch -vv'
abbr --add --global -- gba 'git branch -a -v'
abbr --add --global -- gbd 'git branch -d'
abbr --add --global -- gbl 'git blame -w'
abbr --add --global -- gbs 'git bisect'
abbr --add --global -- gc 'git commit -v'
abbr --add --global -- gca 'git commit -v -a'
abbr --add --global -- gcam 'git commit -a -m'
abbr --add --global -- gcm 'git commit -m'
abbr --add --global -- gcn 'git commit --no-edit --amend'
abbr --add --global -- gcf 'git config --list'
abbr --add --global -- gcl 'git clone --recurse-submodules'
abbr --add --global -- gcp 'git cherry-pick'
abbr --add --global -- gcpa 'git cherry-pick --abort'
abbr --add --global -- gcpc 'git cherry-pick --continue'
abbr --add --global -- gd 'git diff'
abbr --add --global -- gdca 'git diff --cached'
abbr --add --global -- gds 'git diff --stat'
abbr --add --global -- gdw 'git diff --word-diff'
abbr --add --global -- gsh 'git show'
abbr --add --global -- gss 'git status -s'
abbr --add --global -- gsb 'git status -sb'
abbr --add --global -- gst 'git status'
abbr --add --global -- gf 'git fetch'
abbr --add --global -- gfa 'git fetch --all --prune'
abbr --add --global -- gfo 'git fetch origin'
abbr --add --global -- gl 'git pull'
abbr --add --global -- glr 'git pull --rebase'
abbr --add --global -- gp 'git push'
abbr --add --global -- gpf 'git push --force-with-lease'
function gpsup --wraps 'git push'
    set -l __bdock_b (bdock-git-branch); or return 1
    command git push --set-upstream origin $__bdock_b $argv
end
abbr --add --global -- gr 'git remote -v'
abbr --add --global -- glg 'git log --stat'
abbr --add --global -- glo 'git log --oneline --decorate'
abbr --add --global -- glog 'git log --oneline --decorate --graph'
abbr --add --global -- gloga 'git log --oneline --decorate --graph --all'
abbr --add --global -- glol 'git log --graph --pretty=\'%Cred%h%Creset -%C(auto)%d%Creset %s %Cgreen(%ar) %C(bold blue)<%an>%Creset\''
abbr --add --global -- gloo 'git log --pretty=format:\'%C(yellow)%h %Cred%ad %Cblue%an%Cgreen%d %Creset%s\' --date=short'
abbr --add --global -- gm 'git merge'
abbr --add --global -- gma 'git merge --abort'
abbr --add --global -- grb 'git rebase'
abbr --add --global -- grba 'git rebase --abort'
abbr --add --global -- grbc 'git rebase --continue'
abbr --add --global -- grbi 'git rebase --interactive'
function grbm --wraps 'git rebase'
    set -l __bdock_d (bdock-git-default-branch); or return 1
    command git rebase $__bdock_d $argv
end
abbr --add --global -- grs 'git restore'
abbr --add --global -- grst 'git restore --staged'
abbr --add --global -- gsta 'git stash push'
abbr --add --global -- gstl 'git stash list'
abbr --add --global -- gstp 'git stash pop'
abbr --add --global -- gsw 'git switch'
abbr --add --global -- gswc 'git switch --create'
function gswm --wraps 'git switch'
    set -l __bdock_d (bdock-git-default-branch); or return 1
    command git switch $__bdock_d $argv
end
abbr --add --global -- gwt 'git worktree'
abbr --add --global -- gwta 'git worktree add'
abbr --add --global -- gwtls 'git worktree list'
abbr --add --global -- gwtrm 'git worktree remove'
abbr --add --global -- j 'jj'
abbr --add --global -- jab 'jj abandon'
abbr --add --global -- jbl 'jj bookmark list'
abbr --add --global -- jbs 'jj bookmark set'
abbr --add --global -- jc 'jj commit'
abbr --add --global -- jd 'jj diff'
abbr --add --global -- jdesc 'jj describe'
abbr --add --global -- je 'jj edit'
abbr --add --global -- jgf 'jj git fetch'
abbr --add --global -- jgp 'jj git push'
abbr --add --global -- jl 'jj log'
abbr --add --global -- jn 'jj new'
abbr --add --global -- jop 'jj op log'
abbr --add --global -- jrb 'jj rebase'
abbr --add --global -- jsp 'jj split'
abbr --add --global -- jsq 'jj squash'
abbr --add --global -- jst 'jj status'
abbr --add --global -- jun 'jj undo'
abbr --add --global -- ll 'lsd -l --color auto'
abbr --add --global -- la 'lsd -A --color auto'
abbr --add --global -- lla 'lsd -lA --color auto'
abbr --add --global -- lt 'lsd -lt --color auto'
abbr --add --global -- ltr 'lsd -ltr --color auto'
abbr --add --global -- l1 'lsd -1 --color auto'
abbr --add --global -- ltree 'lsd --tree --color auto'
abbr --add --global -- llg 'lsd -l --group-dirs first --color auto'
abbr --add --global -- rgi 'rg -i'
abbr --add --global -- rgl 'rg -l'
abbr --add --global -- rgf 'rg --files'
abbr --add --global -- fdh 'fd --hidden'
abbr --add --global -- dfh 'df -h'
abbr --add --global -- duh 'du -h -d 1'
abbr --add --global -- fr 'free -h'
abbr --add --global -- md 'mkdir -p'
abbr --add --global -- rd 'rmdir'

# gsync and gsync-all (gsync.sh): resync after GitHub rebase-merges rewrote SHAs.
function bdock-gsync-sh
    command sh -c '# BundleDock gsync and gsync-all: one POSIX sh script, run by sh from a function
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
    "" | *[!A-Za-z0-9._/-]*) printf "%s" "\'$(printf "%s" "$1" | sed "s/\'/\'\\\\\\\\\'\'/g")\'" ;;
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
' bdock-gsync $argv
end
function gsync -d 'fetch --prune --all, then pull --rebase --autostash'
    bdock-gsync-sh sync $argv
end
function gsync-all -d 'one status line per repository and submodule; fetch --prune only'
    bdock-gsync-sh all $argv
end

# Navigation names this shell does not run natively.
abbr --add --global -- ... 'cd ../..'
abbr --add --global -- .... 'cd ../../..'
abbr --add --global -- - 'cd -'
