# BundleDock opentofu

The official prebuilt OpenTofu CLI (`tofu`), published as a multi-architecture
OCI image (linux/amd64 and linux/arm64). Mount it as a read-only tool volume at
`/opt/bdock/opentofu`. Providers are not bundled; projects manage their own
provider versions, lock files and credentials. This bundle is retired
(2026-10-31): use `tofu` from the toolbox. Existing images stay available.

- Documentation: https://wstein.github.io/bundledock/
- Source and issues: https://github.com/wstein/bundledock
