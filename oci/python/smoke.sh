#!/bin/sh
set -eu
image=${1:-anchorage/python:3.15.0}
consumer=${2:-debian:trixie-slim@sha256:a29215f6a35e51e22adffa17f89e9d2ef06214e64a2bad10d765c46aea49f11f}
image=$(docker image inspect --format '{{.Id}}' "$image")
volume=$(docker volume create)
cleanup() { docker volume rm "$volume" >/dev/null; }
trap cleanup EXIT
initialize() {
    docker run --rm --network none --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$image"
}
initialize
initialize
check() {
    docker run --rm --network none --user 1000:1000 \
        --tmpfs /home/anchorage:exec,uid=1000,gid=1000,mode=0700 \
        --mount "type=volume,src=$volume,dst=/opt/anchorage-python,readonly,volume-nocopy" \
        --env HOME=/home/anchorage --env PYTHONDONTWRITEBYTECODE=1 \
        --env PIP_NO_INDEX=1 --env PIP_DISABLE_PIP_VERSION_CHECK=1 \
        --entrypoint /bin/sh "$consumer" -ec '
            test -f /opt/anchorage-python/.ready
            export PATH="/opt/anchorage-python/bin:$PATH"
            python3 -c "import ssl, sqlite3, ctypes, venv, zlib, bz2, lzma, sys; print(sys.version); print(sys.prefix, sys.executable); assert sys.prefix == \"/opt/anchorage-python\""
            python3 -m pip --version
            python3 -m venv "$HOME/venv"
            "$HOME/venv/bin/python" -c "import ssl, sqlite3; print(ssl.OPENSSL_VERSION, sqlite3.sqlite_version)"
            "$HOME/venv/bin/python" -m pip --version
            if touch /opt/anchorage-python/write-test 2>/dev/null; then
                echo "Unexpected write to Python volume" >&2
                exit 1
            fi
        '
}
check &
first=$!
check &
second=$!
result=0
wait "$first" || result=1
wait "$second" || result=1
test "$result" = 0
mutate() {
    docker run --rm --network none --entrypoint /bin/sh \
        --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$image" -ec "$1"
}
# Changed, partial, and locked volumes must be rejected rather than repaired.
mutate 'printf "invalid\n" > /bundle/manifest.json'
if initialize; then echo 'Accepted mismatched Python volume' >&2; exit 1; fi
mutate 'rm /bundle/.ready'
if initialize; then echo 'Accepted incomplete Python volume' >&2; exit 1; fi
mutate 'mkdir /bundle/.initializing'
if initialize; then echo 'Accepted locked Python volume' >&2; exit 1; fi
echo 'Shared Python volume smoke test passed'
