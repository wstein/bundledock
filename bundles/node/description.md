# BundleDock node

The official Node.js runtime with its bundled npm and npx, published as a
multi-architecture OCI image (linux/amd64 and linux/arm64). The `lts` tag, which
`latest` and `stable` also follow, carries the LTS line and the `current` tag
carries the Current line. An unqualified pull follows LTS. Mount the LTS
carrier as a read-only tool volume at `/opt/bdock/node`, or the Current carrier at
`/opt/bdock/node-current`, in a glibc language image and keep HOME, the npm cache
and projects in writable consumer storage.

- Documentation: https://wstein.github.io/bundledock/
- Source and issues: https://github.com/wstein/bundledock
