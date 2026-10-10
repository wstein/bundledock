#!/bin/sh
set -eu
setup=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/setup-python.sh
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
mkdir -p "$fixture/bin" "$fixture/target with spaces" "$fixture/existing" "$fixture/preserved"
printf '#!/bin/sh\nprintf "Python 3 fixture\\n"\n' > "$fixture/bin/python3"
chmod +x "$fixture/bin/python3"
ln -s "$(command -v ln)" "$fixture/bin/ln"
# An absent command gains an alias to the discovered executable, including spaces.
PATH="$fixture/bin" /bin/sh "$setup" "$fixture/target with spaces"
test -L "$fixture/target with spaces/python" || { echo 'Missing python alias' >&2; exit 1; }
test "$(readlink "$fixture/target with spaces/python")" = "$fixture/bin/python3"
PATH="$fixture/target with spaces:$fixture/bin" /bin/sh "$setup" "$fixture/target with spaces"
test "$(readlink "$fixture/target with spaces/python")" = "$fixture/bin/python3"
# Preserve an existing python command elsewhere on PATH.
printf '#!/bin/sh\nprintf "existing python\\n"\n' > "$fixture/existing/python"
chmod +x "$fixture/existing/python"
PATH="$fixture/existing:$fixture/bin" /bin/sh "$setup" "$fixture/preserved"
test ! -e "$fixture/preserved/python"
test "$("$fixture/existing/python")" = 'existing python'
echo 'Python alias setup tests passed'
