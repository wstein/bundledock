# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.24-trixie AS compiler
ARG TARGETARCH
WORKDIR /src
# These commands use only the standard library; no module downloads are needed.
COPY go.mod ./
COPY cmd/tools-bundle ./cmd/tools-bundle
COPY cmd/tools-init ./cmd/tools-init
COPY internal/toolbundle ./internal/toolbundle
COPY internal/toolvolume ./internal/toolvolume
RUN CGO_ENABLED=0 go build -trimpath -o /out/tools-bundle ./cmd/tools-bundle \
    && CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -o /out/init-volume ./cmd/tools-init

FROM --platform=$BUILDPLATFORM debian:trixie-slim@sha256:a29215f6a35e51e22adffa17f89e9d2ef06214e64a2bad10d765c46aea49f11f AS builder
ARG TARGETARCH
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=compiler /out/tools-bundle /usr/local/bin/tools-bundle
COPY oci/tools.lock.json /build/tools.lock.json
RUN tools-bundle -arch "${TARGETARCH}"

FROM debian:trixie-slim@sha256:a29215f6a35e51e22adffa17f89e9d2ef06214e64a2bad10d765c46aea49f11f
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates git libstdc++6 libgcc-s1 libatomic1 libsecret-1-0 \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /opt/anchorage /opt/anchorage
COPY --from=compiler /out/init-volume /usr/local/bin/init-volume
ENV PATH="/opt/anchorage/bin:${PATH}" DISABLE_UPDATES=1 DISABLE_AUTOUPDATER=1
ENTRYPOINT ["/usr/local/bin/init-volume"]
