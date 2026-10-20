# BundleDock shared shell layer. Generated from internal/shelllayer/shortcuts.tsv; do not edit.
# Loaded automatically by interactive Nushell when <toolbox>/share is in
# XDG_DATA_DIRS. Opt out with BDOCK_STARSHIP=0 (prompt) or
# BDOCK_FASTFETCH=0 (startup greeting), BDOCK_ZOXIDE=0 (cd/cdi),
# BDOCK_UMASK=0, BDOCK_EDITOR=0. Nushell
# definitions cannot be conditional, so the shortcuts are always defined;
# leave <toolbox>/share out of XDG_DATA_DIRS to skip this file entirely.

if $nu.is-interactive and ($env.BDOCK_UMASK? | default "1") != "0" and (umask) == 'rwxrwxrwx' {
    umask rwxr-xr-x | ignore
}

if $nu.is-interactive and ($env.BDOCK_EDITOR? | default "1") != "0" and ($env.EDITOR? | is-empty) and ($env.TERM_PROGRAM? != "vscode") and ($env.VSCODE_IPC_HOOK_CLI? | is-empty) and ($env.TERMINAL_EMULATOR? != "JetBrains-JediTerm") and ($env.JETBRAINS_REMOTE_RUN? | is-empty) and ($env.IDEA_INITIAL_DIRECTORY? | is-empty) and (which hx | is-not-empty) {
    $env.EDITOR = "hx"
}

if $nu.is-interactive and ($env.BDOCK_FASTFETCH? | default "1") != "0" and ($env.__bdock_fastfetch_pid? | default 0) != $nu.pid and (which fastfetch | is-not-empty) {
    $env.__bdock_fastfetch_pid = $nu.pid
    try { ^fastfetch } catch { null }
}

# Nushell parses string hooks into the persistent session scope. Capture the
# official initializer now, then install it once before the first prompt.
let __bdock_zoxide_hooks = ($env.config?.hooks?.pre_prompt? | default [])
if $nu.is-interactive and ($env.BDOCK_ZOXIDE? | default "1") != "0" and (which __zoxide_z | is-empty) and not ($__bdock_zoxide_hooks | any { try { get __zoxide_hook } catch { false } }) and not ($__bdock_zoxide_hooks | any { try { get __bdock_zoxide_init } catch { false } }) and (which zoxide | is-not-empty) {
    let init = (try { do { ^zoxide init nushell --cmd cd } | complete } catch { null })
    if $init != null and $init.exit_code == 0 {
        $env.config = ($env.config? | default {} | upsert hooks { default {} } | upsert hooks.pre_prompt { default [] })
        $env.config.hooks.pre_prompt = ($env.config.hooks.pre_prompt | append {
            __bdock_zoxide_init: true,
            condition: {||
                # Remove this bootstrap even if an optout or manual initializer
                # appeared after autoload. Preserve every unrelated hook.
                $env.config.hooks.pre_prompt = ($env.config.hooks.pre_prompt | where {|hook| not (try { $hook.__bdock_zoxide_init } catch { false }) })
                $nu.is-interactive and ($env.BDOCK_ZOXIDE? | default "1") != "0" and (which __zoxide_z | is-empty) and not ($env.config.hooks.pre_prompt | any { try { get __zoxide_hook } catch { false } }) and (which zoxide | is-not-empty)
            },
            # The active pre_prompt list is a snapshot: run only the newly
            # installed official hook now so it records the initial directory.
            code: ('alias __bdock_native_cd = cd' + (char nl) + $init.stdout + (char nl) + '$env.config.hooks.pre_prompt | where { try { get __zoxide_hook } catch { false } } | each {|hook| do $hook.code } | ignore')
        })
    }
}

if ($env.BDOCK_STARSHIP? | default "1") != "0" and ($env.STARSHIP_SHELL? | is-empty) and (which starship | is-not-empty) {
    # Keep the existing static equivalent of `starship init nu` here.
    # Generated definitions need persistent parsing through a string hook,
    # as used for zoxide above, rather than direct runtime source evaluation.
    $env.STARSHIP_SHELL = "nu"
    $env.STARSHIP_SESSION_KEY = (random chars -l 16)
    $env.PROMPT_MULTILINE_INDICATOR = (^starship prompt --continuation)
    $env.PROMPT_INDICATOR = ""
    $env.PROMPT_COMMAND = {||
        let duration = if $env.CMD_DURATION_MS == "0823" { 0 } else { $env.CMD_DURATION_MS }
        ^starship prompt --cmd-duration $duration $"--status=($env.LAST_EXIT_CODE)" --terminal-width (term size).columns
    }
    $env.PROMPT_COMMAND_RIGHT = {||
        let duration = if $env.CMD_DURATION_MS == "0823" { 0 } else { $env.CMD_DURATION_MS }
        ^starship prompt --right --cmd-duration $duration $"--status=($env.LAST_EXIT_CODE)" --terminal-width (term size).columns
    }
    $env.config = ($env.config? | default {} | merge {render_right_prompt_on_last_line: true})
}

# Current Git branch; fails on a detached HEAD.
def bdock-git-branch []: nothing -> string {
    let r = (do { ^git symbolic-ref --quiet --short HEAD } | complete)
    if $r.exit_code != 0 {
        error make --unspanned {msg: "bdock: not on a branch (detached HEAD)"}
    }
    $r.stdout | str trim
}

# origin/HEAD, else main, else master.
def bdock-git-default-branch []: nothing -> string {
    let r = (do { ^git symbolic-ref --quiet --short refs/remotes/origin/HEAD } | complete)
    if $r.exit_code == 0 {
        return ($r.stdout | str trim | str replace --regex '^origin/' '')
    }
    let main = (do { ^git show-ref --quiet --verify refs/heads/main } | complete)
    if $main.exit_code == 0 { "main" } else { "master" }
}

alias g = ^git
alias ga = ^git add
alias gaa = ^git add --all
alias gapa = ^git add --patch
alias gau = ^git add --update
alias gb = ^git branch -vv
alias gba = ^git branch -a -v
alias gbd = ^git branch -d
alias gbl = ^git blame -w
alias gbs = ^git bisect
alias gc = ^git commit -v
alias gca = ^git commit -v -a
alias gcam = ^git commit -a -m
alias gcm = ^git commit -m
alias gcn = ^git commit --no-edit --amend
alias gcf = ^git config --list
alias gcl = ^git clone --recurse-submodules
alias gcp = ^git cherry-pick
alias gcpa = ^git cherry-pick --abort
alias gcpc = ^git cherry-pick --continue
alias gd = ^git diff
alias gdca = ^git diff --cached
alias gds = ^git diff --stat
alias gdw = ^git diff --word-diff
alias gsh = ^git show
alias gss = ^git status -s
alias gsb = ^git status -sb
alias gst = ^git status
alias gf = ^git fetch
alias gfa = ^git fetch --all --prune
alias gfo = ^git fetch origin
alias gl = ^git pull
alias glr = ^git pull --rebase
alias gp = ^git push
alias gpf = ^git push --force-with-lease
def --wrapped gpsup [...rest] { ^git push --set-upstream origin (bdock-git-branch) ...$rest }
alias gr = ^git remote -v
alias glg = ^git log --stat
alias glo = ^git log --oneline --decorate
alias glog = ^git log --oneline --decorate --graph
alias gloga = ^git log --oneline --decorate --graph --all
alias glol = ^git log --graph --pretty='%Cred%h%Creset -%C(auto)%d%Creset %s %Cgreen(%ar) %C(bold blue)<%an>%Creset'
alias gloo = ^git log --pretty=format:'%C(yellow)%h %Cred%ad %Cblue%an%Cgreen%d %Creset%s' --date=short
alias gm = ^git merge
alias gma = ^git merge --abort
alias grb = ^git rebase
alias grba = ^git rebase --abort
alias grbc = ^git rebase --continue
alias grbi = ^git rebase --interactive
def --wrapped grbm [...rest] { ^git rebase (bdock-git-default-branch) ...$rest }
alias grs = ^git restore
alias grst = ^git restore --staged
alias gsta = ^git stash push
alias gstl = ^git stash list
alias gstp = ^git stash pop
alias gsw = ^git switch
alias gswc = ^git switch --create
def --wrapped gswm [...rest] { ^git switch (bdock-git-default-branch) ...$rest }
alias gwt = ^git worktree
alias gwta = ^git worktree add
alias gwtls = ^git worktree list
alias gwtrm = ^git worktree remove
alias j = ^jj
alias jab = ^jj abandon
alias jbl = ^jj bookmark list
alias jbs = ^jj bookmark set
alias jc = ^jj commit
alias jd = ^jj diff
alias jdesc = ^jj describe
alias je = ^jj edit
alias jgf = ^jj git fetch
alias jgp = ^jj git push
alias jl = ^jj log
alias jn = ^jj new
alias jop = ^jj op log
alias jrb = ^jj rebase
alias jsp = ^jj split
alias jsq = ^jj squash
alias jst = ^jj status
alias jun = ^jj undo
alias ll = ^lsd -l --color auto
alias la = ^lsd -A --color auto
alias lla = ^lsd -lA --color auto
alias lt = ^lsd -lt --color auto
alias ltr = ^lsd -ltr --color auto
alias l1 = ^lsd -1 --color auto
alias ltree = ^lsd --tree --color auto
alias llg = ^lsd -l --group-dirs first --color auto
alias rgi = ^rg -i
alias rgl = ^rg -l
alias rgf = ^rg --files
alias fdh = ^fd --hidden
alias dfh = ^df -h
alias duh = ^du -h -d 1
alias fr = ^free -h
alias md = ^mkdir -p
alias rd = ^rmdir

# gsync and gsync-all (gsync.sh): resync after GitHub rebase-merges rewrote SHAs.
def --wrapped bdock-gsync-sh [...rest] { ^sh -c r#'
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
'# bdock-gsync ...$rest }
def --wrapped gsync [...rest] { bdock-gsync-sh sync ...$rest }
def --wrapped gsync-all [...rest] { bdock-gsync-sh all ...$rest }

# Navigation: .., ... and .... are native implicit cd in Nushell; - cannot be
# supported (the parser treats a lone - as a string), so none is defined here.
