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

# Navigation names this shell does not run natively.
abbr --add --global -- ... 'cd ../..'
abbr --add --global -- .... 'cd ../../..'
abbr --add --global -- - 'cd -'
