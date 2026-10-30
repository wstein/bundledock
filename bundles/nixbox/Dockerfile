# syntax=docker/dockerfile:1
# nixbox: a Nix store volume. Store paths come only from cache.nixos.org and are
# pinned by bundles/nixbox/packages.lock.json. The eight glibc-dynamic official
# release binaries come from their pinned releases in bundles/nixbox/tools, with
# the toolbox builder (sha256 checked against that lock). Nothing is compiled
# except BundleDock's own nixbox and tools-bundle tools.
FROM --platform=$BUILDPLATFORM golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS compiler
ARG TARGETARCH
ARG BUILDARCH
WORKDIR /src
# The tools builder uses the checksum-pinned pure Go XZ decoder.
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/nixbox ./cmd/nixbox
COPY cmd/tools-bundle ./cmd/tools-bundle
COPY internal/nixbox ./internal/nixbox
COPY internal/toolconfig ./internal/toolconfig
COPY internal/toolbundle ./internal/toolbundle
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$BUILDARCH go build -trimpath -o /out/nixbox-build ./cmd/nixbox \
    && CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -o /out/nixbox ./cmd/nixbox \
    && CGO_ENABLED=0 GOOS=linux GOARCH=$BUILDARCH go build -trimpath -o /out/tools-bundle ./cmd/tools-bundle

# The official release binaries of the target architecture are downloaded and
# checked against bundles/nixbox/tools/packages.lock.json by the toolbox builder
# (a hash mismatch fails the build), then only unpacked, never executed.
FROM compiler AS official
ARG TARGETARCH
COPY bundles/nixbox/tools /build
RUN /out/tools-bundle -arch "${TARGETARCH}" -directory /build -destination /official

# The copy runs on the build platform; the store paths of the target architecture
# are fetched, never executed. Nix creates an empty .links directory (rmdir fails
# if it holds anything); the store must hold the locked paths and nothing else.
FROM --platform=$BUILDPLATFORM nixos/nix@sha256:f159a58f390677f41e8e900a74b6e2f9c7f27b364122ae6acf7a652ce6c551c0 AS fetch
ARG TARGETARCH
COPY --from=compiler /out/nixbox-build /build/nixbox
COPY bundles/nixbox/packages.catalog.json bundles/nixbox/packages.lock.json /build/
COPY bundles/nixbox/shims/nixbox-shim /build/nixbox-shim
COPY --from=official /official /official
RUN set -eu; \
    /build/nixbox check -catalog /build/packages.catalog.json -lock /build/packages.lock.json; \
    paths=$(/build/nixbox roots -arch "${TARGETARCH}" -catalog /build/packages.catalog.json -lock /build/packages.lock.json); \
    nix --extra-experimental-features nix-command copy \
        --from https://cache.nixos.org --to /image-root \
        --option require-sigs true \
        --option trusted-public-keys 'cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY=' \
        --option substituters '' \
        $paths; \
    rmdir /image-root/nix/store/.links; \
    /build/nixbox verify-store -arch "${TARGETARCH}" -catalog /build/packages.catalog.json -lock /build/packages.lock.json -store /image-root/nix/store; \
    mkdir -p /image-root/nix/profile/libexec; \
    cp -a /official /image-root/nix/profile/libexec/nixbox; \
    /build/nixbox profile -arch "${TARGETARCH}" -catalog /build/packages.catalog.json -lock /build/packages.lock.json \
        -store /image-root/nix/store -out /image-root/nix/profile -shim /build/nixbox-shim; \
    mkdir -p /out/opt/bdock; \
    mv /image-root/nix /out/opt/bdock/nixbox; \
    ln -s bdock /out/opt/bundledock

FROM scratch
COPY --from=fetch /out/opt/ /opt/
COPY --from=compiler /out/nixbox /usr/local/bin/nixbox
ENTRYPOINT ["/usr/local/bin/nixbox", "init", "--source", "/opt/bdock/nixbox"]
