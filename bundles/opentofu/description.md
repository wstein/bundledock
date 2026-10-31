# BundleDock opentofu

The official prebuilt OpenTofu CLI (`tofu`), published as a multi-architecture
OCI image (linux/amd64 and linux/arm64). Mount it as a read-only tool volume at
`/opt/bdock/opentofu`. Providers are not bundled; projects manage their own
provider versions, lock files and credentials. The toolbox also ships
`tofu`; this separate bundle remains supported.

- Documentation: https://wstein.github.io/bundledock/
- Source and issues: https://github.com/wstein/bundledock
