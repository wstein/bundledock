#!/bin/sh
# Runs inside a pinned consumer image (see smoke.sh) with the nixbox volume
# read-only at /nix and, when present, the toolbox volume at /opt/bdock/toolbox.
# Network is used only by the https check; NIXBOX_OFFLINE=1 skips it.
set -u
pass=0
total=0
skipped=0
failures=''
root=$(id -u)
# Consumers are minimal: only POSIX tools and the nixbox profile are assumed.
export PATH=/nix/profile/bin:$PATH
export SSL_CERT_FILE=/nix/profile/etc/ssl/certs/ca-bundle.crt GIT_SSL_CAINFO=/nix/profile/etc/ssl/certs/ca-bundle.crt
export GIT_CONFIG_GLOBAL="$HOME/gitconfig" GIT_CONFIG_NOSYSTEM=1
work=$HOME/work
mkdir -p "$work"
# check NAME SCRIPT: SCRIPT runs in its own "sh -e", in a fresh directory.
check() {
    name=$1
    total=$((total + 1))
    dir=$work/$total
    mkdir -p "$dir"
    if out=$(cd "$dir" && sh -ec "$2" 2>&1); then
        pass=$((pass + 1))
        printf 'ok   %s\n' "$name"
    else
        failures="$failures $name"
        printf 'FAIL %s\n%s\n' "$name" "$out" >&2
    fi
}
skip() {
    skipped=$((skipped + 1))
    printf 'skip %s (%s)\n' "$1" "$2"
}

git config --global user.name 'Smoke Test'
git config --global user.email smoke@example.invalid
git config --global init.defaultBranch main

check 'volume is ready and read-only' 'test -f /nix/.ready && test -d /nix/store && test -f /nix/var/nix/db/db.sqlite && test -L /nix/profile/lib/ld.so && ! touch /nix/probe 2>/dev/null && ! touch /nix/store/probe 2>/dev/null'
check 'commands resolve into the profile' 'for c in git rsync zstd less ssh ssh-keygen; do test "$(command -v $c)" = "/nix/profile/bin/$c"; done'
check 'CA bundle present' 'test -s /nix/profile/etc/ssl/certs/ca-bundle.crt'

check 'git commit, log, fsck' 'git init -q r && cd r && echo a > f && git add f && git commit -q -m one && git log --oneline | grep -q one && git fsck --strict'
check 'git bundle create, verify, clone' 'git init -q r && cd r && echo a > f && git add f && git commit -q -m one && git bundle create ../b.bundle --all && git bundle verify ../b.bundle && git clone -q ../b.bundle ../c && test -f ../c/f'
check 'git stash' 'git init -q r && cd r && echo a > f && git add f && git commit -q -m one && echo b > f && git stash -q && test "$(cat f)" = a && git stash pop -q && test "$(cat f)" = b'
check 'git submodule' 'git init -q sub && (cd sub && echo s > s && git add s && git commit -q -m s) && git init -q top && cd top && git -c protocol.file.allow=always submodule add -q ../sub sub && git commit -q -m sm && git submodule status | grep -q sub'
check 'git branch, merge, diff' 'git init -q r && cd r && echo a > f && git add f && git commit -q -m one && git checkout -q -b topic && echo b >> f && git commit -q -am two && git checkout -q main && git merge -q topic && git diff --stat HEAD~1 | grep -q f'
if [ "${NIXBOX_OFFLINE:-0}" = 1 ]; then
    skip 'git ls-remote over https' 'NIXBOX_OFFLINE=1'
else
    check 'git ls-remote over https' 'git ls-remote https://github.com/octocat/Hello-World.git HEAD | grep -q HEAD'
fi

check 'rsync copy with progress' 'mkdir -p s/d && i=0 && while [ $i -lt 200 ]; do echo "file $i" > s/d/f$i; i=$((i + 1)); done && rsync -a --info=progress2 s/ d/ >/dev/null && test -z "$(rsync -a -i --dry-run s/ d/)" && rsync -a --delete --stats s/ d/ | grep -q "Number of files"'
check 'zstd round trip' 'i=0; while [ $i -lt 20000 ]; do echo "line $i"; i=$((i + 1)); done > f && zstd -q -3 f -o f.zst && zstd -t -q f.zst && zstd -q -d f.zst -o f.out && test "$(cksum < f)" = "$(cksum < f.out)" && test "$(zstdcat f.zst | cksum)" = "$(cksum < f)"'
check 'less pages a pipe' 'less --version | head -n 1 | grep -q less && printf "a\nb\n" | less -FX | grep -q b'
check 'ssh version' 'ssh -V 2>&1 | grep -q OpenSSH'
check 'ssh-keygen creates and fingerprints a key' 'ssh-keygen -q -t ed25519 -N "" -f k && ssh-keygen -l -f k.pub | grep -q ED25519'
check 'ssh parses a config' 'printf "Host demo\n  HostName 192.0.2.1\n  Port 2222\n  User tester\n" > cfg && ssh -G -F cfg demo | grep -q "^port 2222"'

toolbox=${BDOCK_TOOLBOX:-/opt/bdock/toolbox}
if [ -f "$toolbox/.ready" ]; then
    check 'shim without the toolbox fails clearly' 'if BDOCK_TOOLBOX=/nonexistent zk --version >out 2>&1; then exit 1; fi; grep -q "needs the toolbox bundle" out'
    if [ -f /etc/alpine-release ]; then
        check 'official binary does not start on musl without the loader' '! NIXBOX_LOADER=never zk --version >/dev/null 2>&1'
        check 'auto mode uses the Nix loader on musl' 'sh -x /nix/profile/bin/zk --version 2>&1 | grep -q "exec .*lib/ld.so --library-path"'
        if [ -n "${NIXBOX_GCOMPAT_SIM:-}" ]; then
            check 'gcompat-like glibc loader stub is present' 'test -e /lib/ld-linux-aarch64.so.1 && test -e /lib/ld-linux-x86-64.so.1'
        fi
    else
        check 'auto mode runs the toolbox binary directly on a glibc host' "sh -x /nix/profile/bin/zk --version 2>&1 | grep -q 'exec $toolbox/bin/zk --version'"
    fi
    check 'a user symlink to a shim finds the profile' 'mkdir ub && ln -s /nix/profile/bin/zk ub/zk && ub/zk --version'
    for mode in auto always; do
        export NIXBOX_LOADER=$mode
        check "ast-grep [$mode]" 'ast-grep --version && printf "foo(1)\n" > a.js && ast-grep run -p "foo(\$A)" -l js a.js | grep -q foo'
        check "delta [$mode]" 'delta --version && printf "a\n" > a && printf "b\n" > b && (git diff --no-index a b || true) | delta --paging=never | grep -q b'
        check "difft [$mode]" 'difft --version && printf "a\n" > a && printf "b\n" > b && difft --color=never a b > out; grep -q b out'
        check "fastfetch [$mode]" 'fastfetch --version && fastfetch --pipe --structure Kernel | grep -qi kernel'
        check "leaf [$mode]" 'leaf --version'
        check "zizmor [$mode]" 'zizmor --version && mkdir -p w/.github/workflows && printf "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n" > w/.github/workflows/a.yml && rc=0; zizmor --offline w >/dev/null 2>&1 || rc=$?; [ $rc -eq 0 ] || [ $rc -ge 10 ]'
        check "zk [$mode]" 'zk --version && zk init --no-input nb >/dev/null && test -d nb/.zk'
        check "hx [$mode]" 'hx --version && hx --health >/dev/null'
    done
    unset NIXBOX_LOADER
else
    skip 'the eight toolbox binaries' 'no toolbox volume mounted'
fi

printf 'RESULT %s %s skipped=%s uid=%s arch=%s\n' "$pass" "$total" "$skipped" "$root" "$(uname -m)"
if [ "$pass" -ne "$total" ]; then
    printf 'FAILED:%s\n' "$failures" >&2
    exit 1
fi
