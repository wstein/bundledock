# BundleDock code-server

The official standalone code-server distribution (browser-based VS Code),
published as a multi-architecture OCI image (linux/amd64 and linux/arm64).
Mount it as a read-only tool volume at `/opt/bdock/code-server` in a glibc
consumer image. No service starts automatically; configuration, data and
extensions stay in writable consumer storage.

- Documentation: https://wstein.github.io/bundledock/
- Source and issues: https://github.com/wstein/bundledock
