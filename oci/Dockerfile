# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.24-trixie AS compiler
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
    && CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -o /out/init-volume ./cmd/tools-init

FROM --platform=$BUILDPLATFORM debian:trixie-slim@sha256:a29215f6a35e51e22adffa17f89e9d2ef06214e64a2bad10d765c46aea49f11f AS downloads
ARG TARGETARCH
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=compiler /out/tools-bundle /usr/local/bin/tools-bundle
COPY tools.txt tools.packages.json tools.lock.json /build/
COPY oci/licenses /build/oci/licenses
FROM downloads AS builder
ARG TARGETARCH
RUN tools-bundle -arch "${TARGETARCH}"

FROM scratch
COPY --from=builder /opt/workharbor /opt/workharbor
COPY --from=compiler /out/init-volume /usr/local/bin/init-volume
ENV PATH="/opt/workharbor/bin:/usr/local/bin" DISABLE_UPDATES=1 DISABLE_AUTOUPDATER=1
ENTRYPOINT ["/usr/local/bin/init-volume"]
