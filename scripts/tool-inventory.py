#!/usr/bin/env python3
"""Generate/check the source Markdown inventory; never render a documentation site."""
import argparse
import json
from pathlib import Path
import re
import sys

BEGIN = "<!-- BEGIN GENERATED TOOL INVENTORY -->"
END = "<!-- END GENERATED TOOL INVENTORY -->"
# Existing display labels only. Versions come exclusively from the
# lock; exact catalog coverage prevents a new tool from being silently omitted.
LABELS = (
    ("codex", "Codex"),
    ("claude", "Claude Code"),
    ("agy", "Antigravity"),
    ("gh", "GitHub CLI"),
    ("jq", "jq"),
    ("rg", "ripgrep (`rg`)"),
    ("fzf", "fzf (`fzf`)"),
    ("bat", "bat (`bat`)"),
    ("regctl", "regctl (`regctl`)"),
    ("fd", "fd (`fd`, `fdfind`)"),
    ("ast-grep", "ast-grep (`ast-grep`)"),
    ("yq", "Mike Farah yq (`yq`)"),
    ("uv", "uv (`uv`, `uvx`)"),
    ("herdr", "Herdr (`herdr`)"),
    ("fish", "Fish (`fish`)"),
    ("starship", "Starship (`starship`)"),
    ("nu", "Nushell (`nu`)"),
    ("tmux", "tmux (`tmux`)"),
    ("typos", "typos (`typos`)"),
    ("lychee", "lychee (`lychee`)"),
    ("gitleaks", "Gitleaks (`gitleaks`)"),
    ("trivy", "Trivy (`trivy`)"),
    ("syft", "Syft (`syft`)"),
    ("bottom", "bottom (`btm`)"),
    ("btop", "btop (`btop`)"),
    ("helix", "Helix (`hx`)"),
    ("hugo", "Hugo (`hugo`, standard)"),
)
DISPLAY_SUFFIX = {"tmux": " (official static builds)"}


def generated(root, bundle_name="toolbox"):
    bundle = root / "bundles" / bundle_name
    labels = LABELS if bundle_name == "toolbox" else (("node", "node"),) if bundle_name == "node-current" else ((bundle_name, bundle_name),)
    lock = json.loads((bundle / "tools.lock.json").read_text())
    catalog = json.loads((bundle / "tools.packages.json").read_text())
    ids = {tool for tool, _ in labels}
    if set(catalog["packages"]) != ids:
        raise ValueError("inventory labels must cover the exact catalog IDs")
    platforms = lock["platforms"]
    if set(platforms) != {"amd64", "arm64"}:
        raise ValueError("inventory requires both supported architectures")
    for architecture in ("amd64", "arm64"):
        if set(platforms[architecture]) != ids:
            raise ValueError("inventory lock must cover the exact catalog IDs")
    rows = ["| Tool | Pinned version |", "| --- | --- |"]
    for tool, label in sorted(labels, key=lambda item: item[1].casefold()):
        version = platforms["amd64"][tool]["version"]
        if version != platforms["arm64"][tool]["version"]:
            raise ValueError(f"{tool}: architecture versions differ")
        if not isinstance(version, str):
            raise ValueError(f"{tool}: unsafe inventory version")
        version = version.removeprefix(catalog["packages"][tool].get("tag_prefix", ""))
        if not re.fullmatch(r"[0-9]+\.[0-9]+(?:\.[0-9]+|[a-z])?", version):
            raise ValueError(f"{tool}: unsafe inventory version")
        rows.append(f"| {label} | {version}{DISPLAY_SUFFIX.get(tool, '')} |")
    document = root / "docs" / ("toolbox.md" if bundle_name == "toolbox" else bundle_name+".md")
    # read_bytes/decode preserves all prose and line endings outside the region.
    source = document.read_bytes().decode("utf-8")
    if source.count(BEGIN) != 1 or source.count(END) != 1:
        raise ValueError("expected exactly one pair of inventory markers")
    prefix, rest = source.split(BEGIN)
    _, suffix = rest.split(END)
    replacement = prefix + BEGIN + "\n" + "\n".join(rows) + "\n" + END + suffix
    return document, source, replacement


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path("."))
    parser.add_argument("--bundle", choices=("toolbox", "code-server", "node", "node-current", "bun", "codex", "agy", "claude"), default="toolbox")
    parser.add_argument("--check", action="store_true", help="fail on drift without writing")
    args = parser.parse_args()
    try:
        document, source, replacement = generated(args.root, args.bundle)
        if source != replacement:
            if args.check:
                raise ValueError("tool inventory is stale; run python3 scripts/tool-inventory.py")
            document.write_bytes(replacement.encode("utf-8"))
    except (OSError, ValueError, KeyError, TypeError) as error:
        print(f"tool inventory: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
