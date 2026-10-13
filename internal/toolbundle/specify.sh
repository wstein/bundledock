#!/bin/sh
# BundleDock-owned launcher; the package is installed only on explicit use.
set -eu
bundle=$(CDPATH= cd -- "${0%/*}/.." && pwd -P)
python=$(command -v python3) || { printf '%s\n' 'specify: Python 3.11+ is required in the language image (python3 on PATH)' >&2; exit 1; }
exec "$python" -I -B "$bundle/libexec/python-tool.py" specify "$@"
