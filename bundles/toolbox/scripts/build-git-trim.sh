#!/bin/sh
set -eu
# Fixed reviewed recipe: no compiler command or flag is accepted from catalog data.
mode=${1:?prepare or compile required}
arch=${2:?architecture required}
case "$arch" in amd64|arm64) ;; *) echo 'unsupported git-trim architecture' >&2; exit 1 ;; esac
case "$mode" in
prepare)
    mkdir -p "$HOME" "$CARGO_HOME"
    tools-bundle --arch "$arch" --git-trim-mode prepare --git-trim-source /src
    cd /src
    git -c init.templateDir= -c core.hooksPath=/dev/null init
    git -c core.hooksPath=/dev/null -c fetch.fsckObjects=true fetch --depth=1 --no-tags \
        https://github.com/foriequal0/git-trim.git 65288617a637029573a5c3517abcb95b050ea02b
    test "$(git rev-parse FETCH_HEAD)" = 65288617a637029573a5c3517abcb95b050ea02b
    git -c core.hooksPath=/dev/null update-ref --no-deref HEAD 65288617a637029573a5c3517abcb95b050ea02b
    git config core.fileMode false
    git read-tree HEAD
    git diff --exit-code HEAD -- .
    # vergen 7.4.3 requires Git metadata. Its deterministic depth-one repo has
    # exactly one fixed tag; upstream build.rs and dependencies remain unchanged.
    GIT_COMMITTER_NAME=BundleDock GIT_COMMITTER_EMAIL=build@bundledock.invalid \
        GIT_COMMITTER_DATE=2022-12-12T17:13:59Z \
        git -c core.hooksPath=/dev/null tag -a -m v0.4.3 v0.4.3
    mkdir .cargo
    cargo vendor --locked --versioned-dirs /src/vendor > .cargo/config.toml
    ;;
compile)
    cd /src
    test "$(rustc --version)" = 'rustc 1.75.0 (82e1608df 2023-12-21)'
    cargo build --frozen --offline --release --bin git-trim
    git diff --exit-code HEAD -- .
    mkdir -p /out/bin
    cp /target/release/git-trim /out/bin/git-trim
    tools-bundle --arch "$arch" --git-trim-mode seal --git-trim-source /src --git-trim-stage /out
    ;;
*) echo 'unsupported git-trim recipe operation' >&2; exit 1 ;;
esac
