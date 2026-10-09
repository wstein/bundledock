"""Download checksum-pinned native tools; run only inside the bundle builder."""

import hashlib
import json
from pathlib import Path
import shutil
import sys
import tarfile
import tempfile
import urllib.request

root = Path('/opt/anchorage')
root.joinpath('bin').mkdir(parents=True)
lock_path = Path('/build/tools.lock.json')
lock = json.loads(lock_path.read_text())
arch = sys.argv[1]
tools = lock['platforms'][arch]

for tool, spec in tools.items():
    print('Installing', tool, spec['version'], flush=True)
    with tempfile.TemporaryDirectory() as tmp:
        payload = Path(tmp) / 'payload'
        with urllib.request.urlopen(spec['url'], timeout=120) as response:
            with payload.open('wb') as output:
                shutil.copyfileobj(response, output)
        algorithm = 'sha512' if 'sha512' in spec else 'sha256'
        digest = hashlib.new(algorithm)
        with payload.open('rb') as source:
            for block in iter(lambda: source.read(1024 * 1024), b''):
                digest.update(block)
        if digest.hexdigest() != spec[algorithm]:
            raise SystemExit('Checksum mismatch: ' + tool)
        if tool == 'claude':
            shutil.copyfile(payload, root / 'bin' / tool)
        else:
            with tarfile.open(payload, 'r:gz') as archive:
                for member in archive.getmembers():
                    if not member.isfile():
                        continue
                    name = Path(member.name).name
                    # Codex's package includes its native helper executables.
                    if tool == 'codex' and (name == 'codex' or name == 'bwrap' or name.startswith('codex-')):
                        target = root / 'bin' / name
                    elif tool == 'gh' and name == 'gh':
                        target = root / 'bin' / 'gh'
                    elif tool == 'agy' and name == 'antigravity':
                        target = root / 'bin' / 'agy'
                    else:
                        continue
                    with archive.extractfile(member) as source, target.open('wb') as output:
                        shutil.copyfileobj(source, output)
                    target.chmod(0o755)
        root.joinpath('bin', tool).chmod(0o755)

manifest = {'schema': 1, 'platform': 'linux/' + arch, 'tools': tools}
root.joinpath('manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
with root.joinpath('SHA256SUMS').open('w') as output:
    for path in sorted(root.rglob('*')):
        if path.is_file() and path.name != 'SHA256SUMS':
            digest = hashlib.sha256(path.read_bytes()).hexdigest()
            output.write(digest + '  ' + str(path.relative_to(root)) + '\n')
