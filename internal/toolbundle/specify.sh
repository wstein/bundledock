#!/bin/sh
# BundleDock-owned launcher; the Python package is resolved only on explicit use.
set -eu
fail() { printf '%s\n' "specify: $*" >&2; exit 1; }
bundle=$(CDPATH= cd -- "${0%/*}/.." && pwd -P)
[ -x "$bundle/bin/uvx" ] || fail 'the toolbox uvx executable is missing'
python=$(command -v python3) || fail 'Python 3.11+ is required in the language image (python3 on PATH)'
case "$python" in
    /*) ;;
    */*) python="$(CDPATH= cd -- "${python%/*}" && pwd -P)/${python##*/}" ;;
    *) python="$PWD/$python" ;;
esac
"$python" -I -B -c 'import os, sys; executable = os.path.realpath(sys.executable); managed = os.environ.get("UV_PYTHON_INSTALL_DIR"); sys.exit(not (sys.version_info >= (3, 11) and sys.prefix == sys.base_prefix and "/uv/python/" not in executable and not (managed and os.path.commonpath([executable, os.path.realpath(managed)]) == os.path.realpath(managed))))' || fail 'Python 3.11+ from the language image is required; virtual or uv-managed Python is unsupported'
[ -n "${HOME:-}" ] || fail 'HOME must be a writable directory outside the toolbox'
[ -d "$HOME" ] && [ -w "$HOME" ] || fail 'HOME must be a writable directory outside the toolbox'
home=$(CDPATH= cd -- "$HOME" && pwd -P)
case "$home" in "$bundle"|"$bundle"/*|/opt/bundledock/toolbox|/opt/bundledock/toolbox/*) fail 'HOME must be outside the toolbox' ;; esac
UV_CACHE_DIR=${UV_CACHE_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/uv}
mkdir -p -- "$UV_CACHE_DIR" || fail 'UV_CACHE_DIR must be writable outside the toolbox'
cache=$(CDPATH= cd -- "$UV_CACHE_DIR" && pwd -P)
case "$cache" in "$bundle"|"$bundle"/*|/opt/bundledock/toolbox|/opt/bundledock/toolbox/*) fail 'UV_CACHE_DIR must be outside the toolbox' ;; esac
[ -w "$cache" ] || fail 'UV_CACHE_DIR must be writable outside the toolbox'
# A caller may choose offline mode, but cannot redirect resolution to other
# indexes or local source trees. --no-config alone does not ignore UV_* env.
unset UV_INDEX UV_DEFAULT_INDEX UV_INDEX_URL UV_EXTRA_INDEX_URL UV_FIND_LINKS PIP_INDEX_URL PIP_EXTRA_INDEX_URL PIP_FIND_LINKS UV_OVERRIDE UV_CONSTRAINT UV_WORKING_DIR
UV_CACHE_DIR=$cache
export UV_CACHE_DIR UV_PYTHON_DOWNLOADS=never UV_PYTHON_PREFERENCE=only-system
exec "$bundle/bin/uvx" --no-config --no-managed-python --no-python-downloads --no-build --default-index https://pypi.org/simple --python "$python" --from specify-cli==1.1.3 specify "$@"
