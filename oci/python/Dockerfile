# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.24-trixie AS compiler
ARG TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY cmd/python-bundle ./cmd/python-bundle
COPY cmd/tools-init ./cmd/tools-init
COPY internal/toolbundle ./internal/toolbundle
COPY internal/toolvolume ./internal/toolvolume
RUN CGO_ENABLED=0 go build -trimpath -o /out/python-bundle ./cmd/python-bundle \
    && CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -o /out/init-volume ./cmd/tools-init

FROM --platform=$BUILDPLATFORM debian:trixie-slim@sha256:a29215f6a35e51e22adffa17f89e9d2ef06214e64a2bad10d765c46aea49f11f AS builder
ARG TARGETARCH
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=compiler /out/python-bundle /usr/local/bin/python-bundle
COPY oci/python/python.lock.json /build/python.lock.json
RUN python-bundle -arch "${TARGETARCH}"

FROM debian:trixie-slim@sha256:a29215f6a35e51e22adffa17f89e9d2ef06214e64a2bad10d765c46aea49f11f
COPY --from=builder /opt/anchorage-python /opt/anchorage-python
COPY --from=compiler /out/init-volume /usr/local/bin/init-volume
ENV PATH="/opt/anchorage-python/bin:${PATH}" PYTHONDONTWRITEBYTECODE=1
ENTRYPOINT ["/usr/local/bin/init-volume", "--source", "/opt/anchorage-python"]
