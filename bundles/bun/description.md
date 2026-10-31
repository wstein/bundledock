# BundleDock bun

The official prebuilt Bun runtime with `bunx`, published as a multi-architecture
OCI image (linux/amd64 and linux/arm64). Mount it as a read-only tool volume at
`/opt/bdock/bun` in a glibc language image. The upstream binaries are unchanged;
HOME, caches and global installations stay in writable consumer storage.

- Documentation: https://wstein.github.io/bundledock/
- Source and issues: https://github.com/wstein/bundledock
