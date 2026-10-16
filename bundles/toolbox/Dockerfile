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

FROM --platform=$BUILDPLATFORM alpine:3.23@sha256:85fe1e81d6758c208f3e1eed4338a1997e19d4be002d4dd32d3100c9a8c010a0 AS downloads
ARG TARGETARCH
RUN apk add --no-cache ca-certificates
COPY --from=compiler /out/tools-bundle /usr/local/bin/tools-bundle
COPY bundles/toolbox/packages.txt bundles/toolbox/packages.catalog.json bundles/toolbox/packages.lock.json /build/
COPY bundles/toolbox/licenses /build/licenses
# BundleDock's own shared shell layer (committed, generated files and notices);
# only the toolbox takes it, and the builder copies these files without running them.
COPY internal/shelllayer/generated /build/shell-layer/generated
COPY internal/shelllayer/notices /build/shell-layer/notices
COPY internal/shelllayer/shortcuts.tsv /build/shell-layer/shortcuts.tsv
FROM downloads AS builder
ARG TARGETARCH
RUN tools-bundle -arch "${TARGETARCH}" -destination /image-root/opt/bdock/toolbox --shell-layer /build/shell-layer \
    && ln -s /opt/bdock /image-root/opt/bundledock

FROM scratch
COPY --from=builder /image-root/opt/ /opt/
COPY --from=compiler /out/init-volume /usr/local/bin/init-volume
ENV PATH="/opt/bdock/toolbox/bin:/usr/local/bin" DISABLE_UPDATES=1 DISABLE_AUTOUPDATER=1
ENTRYPOINT ["/usr/local/bin/init-volume"]
