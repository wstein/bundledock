#!/usr/bin/env python3
"""Generate/check the source Markdown inventory; never render a documentation site."""
import argparse
import json
from pathlib import Path
import re
import sys
from urllib.parse import urlparse

BEGIN = "<!-- BEGIN GENERATED TOOL INVENTORY -->"
END = "<!-- END GENERATED TOOL INVENTORY -->"
# Existing display labels only. Versions come exclusively from the
# lock; exact catalog coverage prevents a new tool from being silently omitted.
LABELS = (
    ("xh", "xh (`xh`)"),
    ("dust", "dust (`dust`)"),
    ("direnv", "direnv (`direnv`)"),
    ("sops", "SOPS (`sops`)"),
    ("git-lfs", "Git LFS (`git-lfs`)"),
    ("lazygit", "lazygit (`lazygit`)"),
    ("leaf", "Leaf (`leaf`)"),
    ("jj", "Jujutsu (`jj`)"),
    ("shellcheck", "ShellCheck (`shellcheck`)"),
    ("shfmt", "shfmt (`shfmt`)"),
    ("just", "just (`just`)"),
    ("ruff", "Ruff (`ruff`)"),
    ("delta", "delta (`delta`)"),
    ("watchexec", "watchexec (`watchexec`)"),
    ("hyperfine", "hyperfine (`hyperfine`)"),
    ("age", "age (`age`, `age-keygen`, `age-inspect`, plugins)"),
    ("gh", "GitHub CLI"),
    ("gh-stack", "GitHub CLI Stacked PRs (`gh stack`)"),
    ("jq", "jq"),
    ("rg", "ripgrep (`rg`)"),
    ("fzf", "fzf (`fzf`)"),
    ("bat", "bat (`bat`)"),
    ("lsd", "lsd (`lsd`)"),
    ("regctl", "regctl (`regctl`)"),
    ("fd", "fd (`fd`, `fdfind`)"),
    ("ast-grep", "ast-grep (`ast-grep`)"),
    ("yq", "Mike Farah yq (`yq`)"),
    ("uv", "uv (`uv`, `uvx`)"),
    ("mise", "mise (`mise`)"),
    ("herdr", "Herdr (`herdr`)"),
    ("fish", "Fish (`fish`)"),
    ("starship", "Starship (`starship`)"),
    ("navi", "navi (`navi`)"),
    ("nu", "Nushell (`nu`)"),
    ("tmux", "tmux (`tmux`)"),
    ("typst", "Typst (`typst`)"),
    ("typos", "typos (`typos`)"),
    ("lychee", "lychee (`lychee`)"),
    ("gitleaks", "Gitleaks (`gitleaks`)"),
    ("trivy", "Trivy (`trivy`)"),
    ("trufflehog", "TruffleHog (`trufflehog`)"),
    ("syft", "Syft (`syft`)"),
    ("scc", "SCC (`scc`, wstein fork)"),
    ("smill", "SheetMill (`smill`)"),
    ("cscribe", "ChartScribe (`cscribe`)"),
    ("opentofu", "OpenTofu (`tofu`)"),
    ("whr", "WorkHarbor (`whr`)"),
    ("stw", "SourceSteward (`stw`)"),
    ("zk", "zk (`zk`)"),
    ("fastfetch", "Fastfetch (`fastfetch`)"),
    ("bottom", "bottom (`btm`)"),
    ("btop", "btop (`btop`)"),
    ("helix", "Helix (`hx`)"),
    ("hugo", "Hugo (`hugo`, standard)"),
    ("jsonschema-cli", "jsonschema-cli (`jsonschema-cli`)"),
    ("difftastic", "Difftastic (`difft`)"),
    ("zoxide", "zoxide (`zoxide`)"),
    ("git-cliff", "git-cliff (`git-cliff`)"),
    ("cosign", "Cosign (`cosign`)"),
    ("zizmor", "zizmor (`zizmor`)"),
    ("pinact", "pinact (`pinact`)"),
    ("hadolint", "Hadolint (`hadolint`)"),
    ("actionlint", "actionlint (`actionlint`)"),
)
DISPLAY_SUFFIX = {"tmux": " (official static builds)"}


# Official homepage fields read from each catalog repository on 2026-10-13.
# A repository without a separate homepage uses its authoritative Source URL.
HOMEPAGES = {'age': 'https://age-encryption.org',
 'ast-grep': 'https://ast-grep.github.io/',
 'bottom': 'https://bottom.pages.dev',
 'delta': 'https://dandavison.github.io/delta/',
 'fish': 'https://fishshell.com',
 'fzf': 'https://junegunn.github.io/fzf/',
 'gh': 'https://cli.github.com',
 'gh-stack': 'https://gh.io/stacks',
 'git-lfs': 'https://git-lfs.com',
 'gitleaks': 'https://gitleaks.io',
 'helix': 'https://helix-editor.com',
 'herdr': 'https://herdr.dev',
 'hugo': 'https://gohugo.io',
 'jj': 'https://jj-vcs.dev',
 'jq': 'https://jqlang.org',
 'just': 'https://just.systems',
 'leaf': 'https://leaf.rivolink.mg',
 'lychee': 'https://lychee.cli.rs',
 'mise': 'https://mise.jdx.dev',
 'nu': 'https://www.nushell.sh/',
 'opentofu': 'https://opentofu.org',
 'regctl': 'https://regclient.org',
 'ruff': 'https://docs.astral.sh/ruff',
 'shellcheck': 'https://www.shellcheck.net',
 'shfmt': 'https://pkg.go.dev/mvdan.cc/sh/v3',
 'starship': 'https://starship.rs',
 'trivy': 'https://trivy.dev',
 'typst': 'https://typst.app',
 'uv': 'https://docs.astral.sh/uv',
 'watchexec': 'https://watchexec.github.io',
 'yq': 'https://mikefarah.gitbook.io/yq/'}
RUNTIME_TOOLS = (
    ("specify", "Spec Kit (`specify`, first-use pip launcher)", "specify_cli", "https://github.github.io/spec-kit/", "https://github.com/github/spec-kit"),
    ("tldr", "tldr (`tldr`, first-use pip launcher)", "tldr", "https://pypi.org/project/tldr/", "https://github.com/tldr-pages/tldr-python-client"),
)


def source_url(package):
    if package.get("provider") == "source-build":
        # The publisher's download page is the authoritative source of a source build.
        url = urlparse(package.get("version_source", {}).get("url", ""))
        if url.scheme != "https" or url.netloc not in package.get("allowed_hosts", []) or not re.fullmatch(r"/[A-Za-z0-9_./-]*", url.path) or url.query or url.fragment:
            raise ValueError("inventory requires a safe authoritative version source URL")
        return url.geturl()
    repository = package.get("repository", "")
    if package.get("provider") != "github" or not isinstance(repository, str) or not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repository):
        raise ValueError("inventory requires a safe authoritative GitHub repository")
    return "https://github.com/" + repository


def runtime_rows(root):
    rows = []
    for tool, label, wheel_name, homepage, source in RUNTIME_TOOLS:
        requirements = root / "internal/toolbundle/python" / tool / (tool + ".requirements.txt")
        lines = requirements.read_text().splitlines()
        versions = []
        for line in lines:
            if line.startswith("#"):
                continue
            url = urlparse(line.split(" @ ", 1)[-1].strip())
            match = re.fullmatch(re.escape(wheel_name) + r"-([0-9]+\.[0-9]+\.[0-9]+)-py3-none-any\.whl", Path(url.path).name)
            if match and url.scheme == "https" and url.netloc == "files.pythonhosted.org" and re.fullmatch(r"sha256=[0-9a-f]{64}", url.fragment):
                versions.append(match[1])
        if len(versions) != 1:
            raise ValueError(f"{tool}: expected one pinned official top-level wheel")
        rows.append((label, versions[0], homepage, source))
    return rows


def generated(root, bundle_name="toolbox"):
    bundle = root / "bundles" / bundle_name
    labels = LABELS if bundle_name == "toolbox" else (("node", "node"),) if bundle_name == "node-current" else ((bundle_name, bundle_name),)
    lock = json.loads((bundle / "packages.lock.json").read_text())
    catalog = json.loads((bundle / "packages.catalog.json").read_text())
    ids = {tool for tool, _ in labels}
    if set(catalog["packages"]) != ids:
        raise ValueError("inventory labels must cover the exact catalog IDs")
    platforms = lock["platforms"]
    if set(platforms) != {"amd64", "arm64"}:
        raise ValueError("inventory requires both supported architectures")
    for architecture in ("amd64", "arm64"):
        if set(platforms[architecture]) != ids:
            raise ValueError("inventory lock must cover the exact catalog IDs")
    entries = []
    for tool, label in sorted(labels, key=lambda item: item[1].casefold()):
        version = platforms["amd64"][tool]["version"]
        if version != platforms["arm64"][tool]["version"]:
            raise ValueError(f"{tool}: architecture versions differ")
        if not isinstance(version, str):
            raise ValueError(f"{tool}: unsafe inventory version")
        version = version.removeprefix(catalog["packages"][tool].get("tag_prefix", ""))
        if not re.fullmatch(r"[0-9]+\.[0-9]+(?:\.[0-9]+|[a-z])?", version):
            raise ValueError(f"{tool}: unsafe inventory version")
        source = source_url(catalog["packages"][tool]) if bundle_name == "toolbox" else ""
        entries.append((label, version + DISPLAY_SUFFIX.get(tool, ""), HOMEPAGES.get(tool, source), source))
    if bundle_name == "toolbox":
        entries.extend(runtime_rows(root))
        rows = ["| Tool | Pinned version | Homepage | Source |", "| --- | --- | --- | --- |"]
        rows.extend(f"| {label} | {version} | [Homepage]({homepage}) | [Source]({source}) |"
                    for label, version, homepage, source in sorted(entries, key=lambda item: item[0].casefold()))
    else:
        rows = ["| Tool | Pinned version |", "| --- | --- |"]
        rows.extend(f"| {label} | {version} |" for label, version, _, _ in entries)
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
    parser.add_argument("--bundle", choices=("toolbox", "code-server", "node", "node-current", "bun", "codex", "agy", "claude", "opentofu"), default="toolbox")
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
