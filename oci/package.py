"""Extract the native Codex package."""

from pathlib import PurePosixPath
import shutil


def extract_codex(archive, root):
    for member in archive.getmembers():
        path = PurePosixPath(member.name)
        if path.is_absolute() or '..' in path.parts:
            raise ValueError('Unsafe Codex package path: ' + member.name)
        if member.isdir():
            continue
        if not member.isfile():
            raise ValueError('Unsupported Codex package member: ' + member.name)
        target = root.joinpath(*path.parts)
        target.parent.mkdir(parents=True, exist_ok=True)
        with archive.extractfile(member) as source, target.open('wb') as output:
            shutil.copyfileobj(source, output)
        target.chmod(0o755 if member.mode & 0o111 else 0o644)
