# syntax=docker/dockerfile:1
FROM debian:trixie-slim@sha256:a29215f6a35e51e22adffa17f89e9d2ef06214e64a2bad10d765c46aea49f11f AS builder
ARG TARGETARCH
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates python3 \
    && rm -rf /var/lib/apt/lists/*
COPY tools.lock.json install.py /build/
RUN python3 /build/install.py "${TARGETARCH}"

FROM debian:trixie-slim@sha256:a29215f6a35e51e22adffa17f89e9d2ef06214e64a2bad10d765c46aea49f11f
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates git libstdc++6 libgcc-s1 libatomic1 libsecret-1-0 \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /opt/anchorage /opt/anchorage
COPY --chmod=755 init-volume.sh /usr/local/bin/init-volume
ENV PATH="/opt/anchorage/bin:${PATH}" DISABLE_UPDATES=1 DISABLE_AUTOUPDATER=1
ENTRYPOINT ["/usr/local/bin/init-volume"]
