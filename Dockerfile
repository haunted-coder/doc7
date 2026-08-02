# syntax=docker/dockerfile:1

FROM golang:1.26.5-bookworm AS build

WORKDIR /src

ARG HTTP_PROXY
ARG HTTPS_PROXY
ENV HTTP_PROXY=${HTTP_PROXY}
ENV HTTPS_PROXY=${HTTPS_PROXY}

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG DOC7_BUILD_VERSION=dev
ARG DOC7_BUILD_COMMIT=unknown
ARG DOC7_BUILD_DATE=unknown
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X github.com/magicrew/doc7/internal/cli.buildVersion=${DOC7_BUILD_VERSION} -X github.com/magicrew/doc7/internal/cli.buildCommit=${DOC7_BUILD_COMMIT} -X github.com/magicrew/doc7/internal/cli.buildDate=${DOC7_BUILD_DATE}" \
    -o /out/doc7 ./cmd/doc7

FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive

ARG HTTP_PROXY
ARG HTTPS_PROXY

RUN http_proxy="${HTTP_PROXY}" \
    https_proxy="${HTTPS_PROXY}" \
    HTTP_PROXY="${HTTP_PROXY}" \
    HTTPS_PROXY="${HTTPS_PROXY}" \
    apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        chromium \
        curl \
        fonts-noto-cjk \
        libreoffice \
        mupdf-tools \
    && rm -rf /var/lib/apt/lists/*

RUN groupadd --system doc7 \
    && useradd --system --gid doc7 --create-home --home-dir /home/doc7 doc7 \
    && mkdir -p /config /data \
    && chown -R doc7:doc7 /config /data /home/doc7

ARG DOC7_BUILD_VERSION=dev
ARG DOC7_BUILD_COMMIT=unknown
ARG DOC7_BUILD_DATE=unknown

LABEL org.opencontainers.image.title="doc7" \
      org.opencontainers.image.description="Visual document to AI-ready Markdown" \
      org.opencontainers.image.source="https://github.com/magicrew/doc7" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${DOC7_BUILD_VERSION}" \
      org.opencontainers.image.revision="${DOC7_BUILD_COMMIT}" \
      org.opencontainers.image.created="${DOC7_BUILD_DATE}"

COPY --from=build /out/doc7 /usr/local/bin/doc7
COPY LICENSE /usr/share/doc/doc7/LICENSE

ENV HOME=/home/doc7 \
    XDG_CONFIG_HOME=/config \
    XDG_CACHE_HOME=/data/cache

VOLUME ["/config", "/data"]
EXPOSE 8787

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD curl --fail --silent http://127.0.0.1:8787/healthz || exit 1

USER doc7
ENTRYPOINT ["doc7"]
CMD ["serve", "--addr", "0.0.0.0:8787", "--data-dir", "/data/server"]
