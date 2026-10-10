# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.24-alpine@sha256:8bee1901f1e530bfb4a7850aa7a479d17ae3a18beb6e09064ed54cfd245b7191 AS compiler
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

FROM --platform=$BUILDPLATFORM alpine:3.23@sha256:85fe1e81d6758c208f3e1eed4338a1997e19d4be002d4dd32d3100c9a8c010a0 AS downloads
ARG TARGETARCH
RUN apk add --no-cache ca-certificates
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
