---
title: Package-installed tools
description: Keep package-managed command-line tools isolated from projects and the immutable toolbox.
weight: 25
---

# Package-installed tools

[Toolbox inventory](toolbox.md#source-inventory) distinguishes downloaded tools
from first-use package launchers. These are source defaults for additions;
the published image changes only after an approved build and publication.

## Python default

Use plain pip in one virtual environment per command-line tool, outside the
read-only toolbox. Ship reviewed requirements and an owned launcher with the
bundle. The consumer's stock language image must supply a supported Python,
`venv` and pip. Installation happens only when the user explicitly invokes the
tool; it never creates a project environment or changes the global interpreter.

The shared launcher currently supports Spec Kit and tldr. It selects official
PyPI, ignores pip configuration and installer/Python overrides, and requires
prebuilt wheels. Missing compatible wheels fail instead of building from source.
A complete environment is keyed by requirements, helper source, interpreter,
ABI and machine. Concurrent first uses serialize, and only a completed
installation is launched. Old environments remain available after input changes. Keep any chosen XDG
ancestor directories under your control; the launcher checks its per-tool state
and environment, rather than auditing every ancestor or filesystem ACL.

Spec Kit pins its top-level wheel URL and SHA256 while resolving transitive
versions on first use; those versions remain unfrozen. tldr pins its complete
three-wheel Linux closure and disables dependency resolution. Keep each policy
explicit when updating a tool. Both require network access for an uncached first
installation. Package caches and completed environments are different states.
See [Spec Kit](toolbox.md#spec-kit-launcher-source) and
[tldr](toolbox.md#tldr-launcher-source) for paths and runtime limitations.

## npm default for future additions

Each future npm-installed CLI gets its own reviewed `package.json` and
`package-lock.json`, plus a dedicated writable installation directory. Pin the
entire production dependency graph, official registry tarball URLs and integrity
hashes. Update and review those files together. Use `npm ci --ignore-scripts`
against those exact inputs; source compilation and dependency lifecycle scripts
are forbidden. Only published JavaScript or independently verified official
prebuilt payloads that work with scripts disabled qualify.

A launcher must isolate npm project, user, global and environment configuration,
select the official registry, and keep HOME, cache and package directories
outside the immutable bundle. It must preserve arguments and exit status,
verify Node/npm compatibility, and bind completed state to the reviewed lock and
runtime/platform identity. First-use installation must serialize and publish
completion safely; it must preserve unknown state and fail clearly when a
network-free install cannot complete. Use Node/npm from the stock language image
or the selected official BundleDock Node carrier; do not bootstrap a runtime.

These are requirements for a future concrete npm launcher. No npm installer
helper is shipped while there is no npm-installed CLI. npm's documented
[`ci` contract](https://docs.npmjs.com/cli/v11/commands/npm-ci/) requires an
existing lock and rejects disagreement with `package.json` without updating it.

## Existing packages

The current catalog contains **no npm-installed command-line tool**. Codex,
Antigravity and Claude use official native prebuilts; code-server uses its
official complete payload. The Node LTS and Current carriers preserve their
official runtime and bundled npm/npx. These remain on their existing installation
paths and do not acquire a separate npm installation. The package launchers are
Spec Kit and the Python tldr client; Spec Kit moves from uvx to its own plain-pip
virtual environment, and tldr is added using that same per-tool policy.

Refreshing a package pin is source work. Tests may use synthetic installers;
real first-use installation, image builds, publication and consumer activation
retain their own explicit authorization and verification requirements.
