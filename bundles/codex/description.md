# BundleDock codex

The official prebuilt Codex package, published as a multi-architecture OCI image
(linux/amd64 and linux/arm64). The bundle carries the vendor's package unmodified,
including its helper binaries. Mount it as a read-only tool volume at
`/opt/bdock/codex`. HOME, `CODEX_HOME`, caches and configuration stay with the
consumer; see the documentation.

- Documentation: https://wstein.github.io/bundledock/
- Source and issues: https://github.com/wstein/bundledock
