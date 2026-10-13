# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS compiler
ARG TARGETARCH
WORKDIR /src
# The bundle builder uses the checksum-pinned pure Go XZ decoder.
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/tools-bundle ./cmd/tools-bundle
COPY cmd/tools-init ./cmd/tools-init
COPY internal/toolconfig ./internal/toolconfig
COPY internal/toolbundle ./internal/toolbundle
COPY internal/toolvolume ./internal/toolvolume
RUN CGO_ENABLED=0 go build -trimpath -o /out/tools-bundle ./cmd/tools-bundle \
	&& CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -o /out/tools-bundle-native ./cmd/tools-bundle \
    && CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -o /out/init-volume ./cmd/tools-init

# The single user-authorized source exception. Stock image includes all native
# bootstrap dependencies; no mutable apt/apk installs or Rust downloads occur.
FROM --platform=$TARGETPLATFORM rust:1.75.0-bookworm@sha256:87f3b2f93b82995443a1a558c234212dafe79cfdc3af956539610560369ddcd0 AS git-trim-source
ARG TARGETARCH
WORKDIR /build
COPY --from=compiler /out/tools-bundle-native /usr/local/bin/tools-bundle
COPY bundles/toolbox/packages.txt bundles/toolbox/packages.catalog.json bundles/toolbox/packages.lock.json /build/
COPY bundles/toolbox/scripts/build-git-trim.sh /build/build-git-trim.sh
COPY bundles/toolbox/licenses/git-trim /build/licenses/git-trim
ENV HOME=/build-home CARGO_HOME=/cargo-home CARGO_TARGET_DIR=/target \
    GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null \
    SOURCE_DATE_EPOCH=1670865239 CARGO_INCREMENTAL=0 \
    OPENSSL_STATIC=1 LIBZ_SYS_STATIC=1 LIBGIT2_NO_PKG_CONFIG=1 \
    RUSTFLAGS="--remap-path-prefix=/src=git-trim --remap-path-prefix=/target=target -C link-arg=-Wl,--build-id=none"
RUN sh /build/build-git-trim.sh prepare "$TARGETARCH"
RUN --network=none sh /build/build-git-trim.sh compile "$TARGETARCH"

FROM --platform=$BUILDPLATFORM alpine:3.23@sha256:85fe1e81d6758c208f3e1eed4338a1997e19d4be002d4dd32d3100c9a8c010a0 AS downloads
ARG TARGETARCH
RUN apk add --no-cache ca-certificates
COPY --from=compiler /out/tools-bundle /usr/local/bin/tools-bundle
COPY bundles/toolbox/packages.txt bundles/toolbox/packages.catalog.json bundles/toolbox/packages.lock.json /build/
COPY bundles/toolbox/licenses /build/licenses
FROM downloads AS builder
ARG TARGETARCH
COPY --from=git-trim-source /out /git-trim-stage
RUN tools-bundle -arch "${TARGETARCH}" -destination /image-root/opt/bdock/toolbox --git-trim-stage /git-trim-stage \
    && ln -s /opt/bdock /image-root/opt/bundledock

FROM scratch
COPY --from=builder /image-root/opt/ /opt/
COPY --from=compiler /out/init-volume /usr/local/bin/init-volume
ENV PATH="/opt/bdock/toolbox/bin:/usr/local/bin" DISABLE_UPDATES=1 DISABLE_AUTOUPDATER=1
ENTRYPOINT ["/usr/local/bin/init-volume"]
