# Anchorage

Anchorage is an early-stage project for a lightweight, AI-native development
environment built on Docker, inspired by GitHub Codespaces.

The first proof of concept will provide Codex, Claude Code, and Google
Antigravity (`agy`) inside development containers through a shared tool volume.

## Planned proof of concept

- Package pinned CLI versions and their required runtimes as an OCI tool bundle.
- Initialize a versioned Docker volume from the bundle and mount it read-only
  into development containers.
- Keep credentials, configuration, caches, and session state separate from the
  shared executables.
- Support upgrades and rollback by selecting a different bundle version.

Start with a Debian/glibc container baseline and platform-specific Linux
artifacts. Validate actual CLI execution and authentication, concurrent use by
two containers, and upgrade and rollback behavior.

The tool bundle, registry integration, volume initialization, and CLI
installation are planned; they are not implemented yet.

## Development

The repository includes a Go devcontainer configuration. Open it with an editor
that supports development containers, or use a local Go 1.24 or newer toolchain.

```sh
go run ./cmd/anchorage
go test ./cmd/...
go vet ./cmd/...
go build -o /tmp/anchorage ./cmd/anchorage
```

The CLI currently prints a project identifier only.

Checks target `cmd` explicitly because the CrewBook submodule contains Go
evaluation fixtures that are not Anchorage packages.

## Repository layout

- `cmd/anchorage`: Go CLI entry point.
- `.devcontainer`: development container configuration.
- `crewbook`: pinned CrewBook workflow submodule.

To clone with the workflow resources:

```sh
git clone --recurse-submodules <repository-url>
```

For an existing clone, run `git submodule update --init --recursive`.
