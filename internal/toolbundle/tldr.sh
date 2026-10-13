#!/bin/sh
# BundleDock-owned launcher; installs only after explicit invocation.
set -eu
bundle=$(CDPATH= cd -- "${0%/*}/.." && pwd -P)
python=$(command -v python3) || { printf '%s\n' 'tldr: Python 3.10+ is required in the language image (python3 on PATH)' >&2; exit 1; }
exec "$python" -I -B "$bundle/libexec/python-tool.py" tldr "$@"
