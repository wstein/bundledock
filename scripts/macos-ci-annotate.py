#!/usr/bin/env python3
"""Turn a unittest log into one GitHub error annotation per failing test (macOS evidence job)."""
import re
import sys

text = open(sys.argv[1], errors='replace').read()
blocks = re.split(r'\n={70}\n', text)[1:]
print('summary:', ' | '.join(l for l in text.splitlines() if l.startswith(('Ran ', 'FAILED', 'OK'))))
for block in blocks[:40]:
    head, _, body = block.partition('\n' + '-' * 70 + '\n')
    tail = body.strip()[-500:]
    msg = (head.strip() + '\n' + tail).replace('%', '%25').replace('\r', '').replace('\n', '%0A')
    print('::error title=macOS test failure::' + msg)
