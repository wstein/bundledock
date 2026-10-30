# BundleDock toolbox

Common development command-line tools for devcontainers and CI, published as a
multi-architecture OCI image (linux/amd64 and linux/arm64) with pinned,
verified versions. Mount it as a read-only tool volume instead of installing
the tools in every image.

The toolbox holds the statically linked and portable tools. Tools that need
glibc, or that have no static prebuilt, ship in the companion
[nixbox](https://quay.io/repository/bundledock/nixbox) bundle, which is used
beside the toolbox.

- Documentation: https://wstein.github.io/bundledock/
- Source and issues: https://github.com/wstein/bundledock
