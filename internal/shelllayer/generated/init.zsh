# BundleDock shared shell layer. Generated from internal/shelllayer/shortcuts.tsv; do not edit.
# Source from an interactive zsh (for example /etc/zshrc or /etc/zsh/zshrc).
# Opt out: BDOCK_SHORTCUTS=0 (aliases), BDOCK_STARSHIP=0 (prompt),
# BDOCK_ZSH_EXPAND=0 (keep aliases, but do not expand them on space),
# BDOCK_FASTFETCH=0 (startup greeting), BDOCK_ZOXIDE=0 (cd/cdi),
# BDOCK_UMASK=0, BDOCK_HISTORY=0, BDOCK_EDITOR=0.
[[ -n ${ZSH_VERSION-} ]] || return 0
[[ -o interactive ]] || return 0

if [[ ${BDOCK_UMASK-1} != 0 ]]; then
    case $(umask) in
        0000 | 000) umask 022 ;;
    esac
fi

if [[ ${BDOCK_HISTORY-1} != 0 ]]; then
    setopt HIST_IGNORE_ALL_DUPS HIST_SAVE_NO_DUPS HIST_IGNORE_SPACE
fi

if [[ ${BDOCK_EDITOR-1} != 0 && -z ${EDITOR-} ]] &&
    [[ ${TERM_PROGRAM-} != vscode && -z ${VSCODE_IPC_HOOK_CLI-} ]] &&
    [[ ${TERMINAL_EMULATOR-} != JetBrains-JediTerm && -z ${JETBRAINS_REMOTE_RUN-} && -z ${IDEA_INITIAL_DIRECTORY-} ]] &&
    (( $+commands[hx] )); then
    export EDITOR=hx
fi

if [[ ${BDOCK_FASTFETCH-1} != 0 && ${__bdock_fastfetch_pid-} != $$ ]] && (( $+commands[fastfetch] )); then
    typeset -g __bdock_fastfetch_pid=$$
    command fastfetch || :
fi

if [[ ${BDOCK_STARSHIP-1} != 0 && -z ${STARSHIP_SHELL-} ]] && (( $+commands[starship] )); then
    eval "$(starship init zsh)"
fi

# Keep manually initialized zoxide in control. Only evaluate successful output.
if [[ ${BDOCK_ZOXIDE-1} != 0 ]] && (( ! $+functions[__zoxide_z] && ! $+functions[__zoxide_hook] && $+commands[zoxide] )); then
    if __bdock_zoxide_init=$(command zoxide init zsh --cmd cd); then
        eval "$__bdock_zoxide_init"
    fi
    unset __bdock_zoxide_init
fi

[[ ${BDOCK_SHORTCUTS-1} != 0 ]] || return 0

function bdock-git-branch {
    command git symbolic-ref --quiet --short HEAD || {
        printf '%s\n' 'bdock: not on a branch (detached HEAD)' >&2
        return 1
    }
}
function bdock-git-default-branch {
    local ref
    if ref=$(command git symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null); then
        printf '%s\n' "${ref#origin/}"
    elif command git show-ref --quiet --verify refs/heads/main; then
        printf '%s\n' main
    else
        printf '%s\n' master
    fi
}

typeset -gA __bdock_shortcuts
__bdock_shortcuts=(
    g 'git'
    ga 'git add'
    gaa 'git add --all'
    gapa 'git add --patch'
    gau 'git add --update'
    gb 'git branch -vv'
    gba 'git branch -a -v'
    gbd 'git branch -d'
    gbl 'git blame -w'
    gbs 'git bisect'
    gc 'git commit -v'
    gca 'git commit -v -a'
    gcam 'git commit -a -m'
    gcm 'git commit -m'
    gcn 'git commit --no-edit --amend'
    gcf 'git config --list'
    gcl 'git clone --recurse-submodules'
    gcp 'git cherry-pick'
    gcpa 'git cherry-pick --abort'
    gcpc 'git cherry-pick --continue'
    gd 'git diff'
    gdca 'git diff --cached'
    gds 'git diff --stat'
    gdw 'git diff --word-diff'
    gsh 'git show'
    gss 'git status -s'
    gsb 'git status -sb'
    gst 'git status'
    gf 'git fetch'
    gfa 'git fetch --all --prune'
    gfo 'git fetch origin'
    gl 'git pull'
    glr 'git pull --rebase'
    gp 'git push'
    gpf 'git push --force-with-lease'
    gr 'git remote -v'
    glg 'git log --stat'
    glo 'git log --oneline --decorate'
    glog 'git log --oneline --decorate --graph'
    gloga 'git log --oneline --decorate --graph --all'
    glol 'git log --graph --pretty='\''%Cred%h%Creset -%C(auto)%d%Creset %s %Cgreen(%ar) %C(bold blue)<%an>%Creset'\'''
    gloo 'git log --pretty=format:'\''%C(yellow)%h %Cred%ad %Cblue%an%Cgreen%d %Creset%s'\'' --date=short'
    gm 'git merge'
    gma 'git merge --abort'
    grb 'git rebase'
    grba 'git rebase --abort'
    grbc 'git rebase --continue'
    grbi 'git rebase --interactive'
    grs 'git restore'
    grst 'git restore --staged'
    gsta 'git stash push'
    gstl 'git stash list'
    gstp 'git stash pop'
    gsw 'git switch'
    gswc 'git switch --create'
    gwt 'git worktree'
    gwta 'git worktree add'
    gwtls 'git worktree list'
    gwtrm 'git worktree remove'
    j 'jj'
    jab 'jj abandon'
    jbl 'jj bookmark list'
    jbs 'jj bookmark set'
    jc 'jj commit'
    jd 'jj diff'
    jdesc 'jj describe'
    je 'jj edit'
    jgf 'jj git fetch'
    jgp 'jj git push'
    jl 'jj log'
    jn 'jj new'
    jop 'jj op log'
    jrb 'jj rebase'
    jsp 'jj split'
    jsq 'jj squash'
    jst 'jj status'
    jun 'jj undo'
    ll 'lsd -l --color auto'
    la 'lsd -A --color auto'
    lla 'lsd -lA --color auto'
    lt 'lsd -lt --color auto'
    ltr 'lsd -ltr --color auto'
    l1 'lsd -1 --color auto'
    ltree 'lsd --tree --color auto'
    llg 'lsd -l --group-dirs first --color auto'
    rgi 'rg -i'
    rgl 'rg -l'
    rgf 'rg --files'
    fdh 'fd --hidden'
    dfh 'df -h'
    duh 'du -h -d 1'
    fr 'free -h'
    md 'mkdir -p'
    rd 'rmdir'
)
alias g='git'
alias ga='git add'
alias gaa='git add --all'
alias gapa='git add --patch'
alias gau='git add --update'
alias gb='git branch -vv'
alias gba='git branch -a -v'
alias gbd='git branch -d'
alias gbl='git blame -w'
alias gbs='git bisect'
alias gc='git commit -v'
alias gca='git commit -v -a'
alias gcam='git commit -a -m'
alias gcm='git commit -m'
alias gcn='git commit --no-edit --amend'
alias gcf='git config --list'
alias gcl='git clone --recurse-submodules'
alias gcp='git cherry-pick'
alias gcpa='git cherry-pick --abort'
alias gcpc='git cherry-pick --continue'
alias gd='git diff'
alias gdca='git diff --cached'
alias gds='git diff --stat'
alias gdw='git diff --word-diff'
alias gsh='git show'
alias gss='git status -s'
alias gsb='git status -sb'
alias gst='git status'
alias gf='git fetch'
alias gfa='git fetch --all --prune'
alias gfo='git fetch origin'
alias gl='git pull'
alias glr='git pull --rebase'
alias gp='git push'
alias gpf='git push --force-with-lease'
function gpsup { local __bdock_b __bdock_d; __bdock_b=$(bdock-git-branch) || return; command git push --set-upstream origin "$__bdock_b" "$@"; }
alias gr='git remote -v'
alias glg='git log --stat'
alias glo='git log --oneline --decorate'
alias glog='git log --oneline --decorate --graph'
alias gloga='git log --oneline --decorate --graph --all'
alias glol='git log --graph --pretty='\''%Cred%h%Creset -%C(auto)%d%Creset %s %Cgreen(%ar) %C(bold blue)<%an>%Creset'\'''
alias gloo='git log --pretty=format:'\''%C(yellow)%h %Cred%ad %Cblue%an%Cgreen%d %Creset%s'\'' --date=short'
alias gm='git merge'
alias gma='git merge --abort'
alias grb='git rebase'
alias grba='git rebase --abort'
alias grbc='git rebase --continue'
alias grbi='git rebase --interactive'
function grbm { local __bdock_b __bdock_d; __bdock_d=$(bdock-git-default-branch) || return; command git rebase "$__bdock_d" "$@"; }
alias grs='git restore'
alias grst='git restore --staged'
alias gsta='git stash push'
alias gstl='git stash list'
alias gstp='git stash pop'
alias gsw='git switch'
alias gswc='git switch --create'
function gswm { local __bdock_b __bdock_d; __bdock_d=$(bdock-git-default-branch) || return; command git switch "$__bdock_d" "$@"; }
alias gwt='git worktree'
alias gwta='git worktree add'
alias gwtls='git worktree list'
alias gwtrm='git worktree remove'
alias j='jj'
alias jab='jj abandon'
alias jbl='jj bookmark list'
alias jbs='jj bookmark set'
alias jc='jj commit'
alias jd='jj diff'
alias jdesc='jj describe'
alias je='jj edit'
alias jgf='jj git fetch'
alias jgp='jj git push'
alias jl='jj log'
alias jn='jj new'
alias jop='jj op log'
alias jrb='jj rebase'
alias jsp='jj split'
alias jsq='jj squash'
alias jst='jj status'
alias jun='jj undo'
alias ll='lsd -l --color auto'
alias la='lsd -A --color auto'
alias lla='lsd -lA --color auto'
alias lt='lsd -lt --color auto'
alias ltr='lsd -ltr --color auto'
alias l1='lsd -1 --color auto'
alias ltree='lsd --tree --color auto'
alias llg='lsd -l --group-dirs first --color auto'
alias rgi='rg -i'
alias rgl='rg -l'
alias rgf='rg --files'
alias fdh='fd --hidden'
alias dfh='df -h'
alias duh='du -h -d 1'
alias fr='free -h'
alias md='mkdir -p'
alias rd='rmdir'

# gsync and gsync-all (gsync.sh): resync after GitHub rebase-merges rewrote SHAs.
function bdock-gsync-sh { command sh -c '# BundleDock gsync and gsync-all: one POSIX sh script, run by sh from a function
# in bash, zsh, fish and Nushell, so all four shells behave the same.
# Usage: sh -c <this script> bdock-gsync sync|all [pull arguments]
# The text is embedded into every generated shell file inside single quotes
# (and a Nushell raw string), so it must contain no single quote and no
# backslash; a test enforces that.

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

# gsync: fetch with pruning, then rebase the current branch onto its upstream.
sync_one() {
    in_work_tree || fail "not inside a Git work tree"
    check_idle
    if ! git symbolic-ref --quiet HEAD >/dev/null 2>&1; then
        git fetch --prune --all || fail "fetch failed"
        fail "detached HEAD: fetched and pruned, but not pulling; switch to a branch first"
    fi
    git fetch --prune --all || fail "fetch failed"
    git pull --rebase --autostash "$@"
}

# gsync-all: one status line per repository, fetch --prune only.
report_one() {
    dir=$1
    if [ "$dir" != . ] && ! [ -e "$dir/.git" ]; then
        say "$dir: not checked out (git submodule update --init $dir)"
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
        position="no upstream"
    fi
    if [ -n "$(git -C "$dir" status --porcelain 2>/dev/null)" ]; then
        state=dirty
    else
        state=clean
    fi
    say "$dir: $branch, $position, $state$note"
    if [ "$behind" -gt 0 ]; then
        say "  behind: run  git -C $dir pull --rebase --autostash"
    fi
}

sync_all() {
    in_work_tree || fail "not inside a Git work tree"
    status=0
    report_one .
    if [ -f .gitmodules ]; then
        paths=$(git config --file .gitmodules --get-regexp "[.]path$")
        while read -r line; do
            [ -n "$line" ] && report_one "${line#* }"
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
all) sync_all ;;
*) fail "unknown mode $mode" ;;
esac
' bdock-gsync "$@"; }
function gsync { bdock-gsync-sh sync "$@"; }
function gsync-all { bdock-gsync-sh all; }

# Navigation names this shell does not run natively.
alias -- ..='cd ..'
alias -- ...='cd ../..'
alias -- ....='cd ../../..'

# Like fish abbreviations: a static shortcut typed in command position expands
# in place when space is pressed, so the real command is visible and in
# history. Branch-dependent shortcuts stay functions. Ctrl+Space inserts a
# plain space.
if [[ ${BDOCK_ZSH_EXPAND-1} != 0 ]]; then
    __bdock-expand-space() {
        emulate -L zsh
        setopt extendedglob
        local word=${LBUFFER##*[[:space:];|&]}
        local before=${LBUFFER%$word}
        if [[ -n $word && -n ${__bdock_shortcuts[$word]-} ]] &&
            [[ $before == [[:space:]]# || $before == *[\;\|\&][[:space:]]# ]]; then
            LBUFFER=$before${__bdock_shortcuts[$word]}
        fi
        zle self-insert
    }
    zle -N __bdock-expand-space
    bindkey -M emacs ' ' __bdock-expand-space
    bindkey -M viins ' ' __bdock-expand-space
    bindkey -M emacs '^ ' magic-space
    bindkey -M viins '^ ' magic-space
    bindkey -M isearch ' ' self-insert
fi
