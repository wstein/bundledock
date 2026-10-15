# BundleDock shared shell layer. Generated from internal/shelllayer/shortcuts.tsv; do not edit.
# Loaded automatically by interactive Nushell when <toolbox>/share is in
# XDG_DATA_DIRS. Opt out of the prompt with BDOCK_STARSHIP=0. Nushell
# definitions cannot be conditional, so the shortcuts are always defined;
# leave <toolbox>/share out of XDG_DATA_DIRS to skip this file entirely.

if ($env.BDOCK_STARSHIP? | default "1") != "0" and ($env.STARSHIP_SHELL? | is-empty) and (which starship | is-not-empty) {
    # Equivalent to `starship init nu`, kept static because autoload files
    # cannot evaluate generated code.
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
