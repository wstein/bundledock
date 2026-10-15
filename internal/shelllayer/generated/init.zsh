# BundleDock shared shell layer. Generated from internal/shelllayer/shortcuts.tsv; do not edit.
# Source from an interactive zsh (for example /etc/zshrc or /etc/zsh/zshrc).
# Opt out: BDOCK_SHORTCUTS=0 (aliases), BDOCK_STARSHIP=0 (prompt),
# BDOCK_ZSH_EXPAND=0 (keep aliases, but do not expand them on space).
[[ -n ${ZSH_VERSION-} ]] || return 0
[[ -o interactive ]] || return 0

if [[ ${BDOCK_STARSHIP-1} != 0 && -z ${STARSHIP_SHELL-} ]] && (( $+commands[starship] )); then
    eval "$(starship init zsh)"
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
