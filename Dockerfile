# syntax=docker/dockerfile:1

# --- Build stage ---
FROM golang:1.25.11-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT_HASH=unknown
ARG BUILD_TIME=unknown
ARG GO_BUILD_TAGS
RUN if [ -n "${GO_BUILD_TAGS}" ]; then \
			CGO_ENABLED=0 GOOS=linux go build -tags "${GO_BUILD_TAGS}" -ldflags="-s -w -X main.version=${VERSION} -X main.commitHash=${COMMIT_HASH} -X main.buildTime=${BUILD_TIME}" -o /bin/smackerel-core ./cmd/core; \
		else \
			CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.version=${VERSION} -X main.commitHash=${COMMIT_HASH} -X main.buildTime=${BUILD_TIME}" -o /bin/smackerel-core ./cmd/core; \
		fi

# --- Runtime stage ---
# Named "core" so build.yml can target it with `--target core`.
# This is the deployable image consumed by deploy/<target>/apply.sh per G074.
# alpine:3.22 = current LTS (alpine:3.20 reached EOL 2026-04-30 — trivy WARN'd
# the deprecated OS family which the trivy-action wrapper treats as a deploy
# blocker; spec 047 R5 bumps to a supported release).
FROM alpine:3.22 AS core

ARG VERSION=dev
ARG COMMIT_HASH=unknown
ARG BUILD_TIME=unknown
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.revision="${COMMIT_HASH}"
LABEL org.opencontainers.image.created="${BUILD_TIME}"
LABEL org.opencontainers.image.title="smackerel-core"
LABEL org.opencontainers.image.source="https://github.com/smackerel/smackerel"

# spec-047 R15 (BUG-047-003 close-out 2026-06-11): upgrade the OpenSSL
# packages shipped by the alpine:3.22 base image. alpine:3.22 is a fixed
# base tag whose package layer froze libssl3/libcrypto3 at 3.5.6-r0;
# Alpine Security has since published 3.5.7-r0. CVE addressed:
#   CVE-2026-45447  libssl3+libcrypto3  3.5.6-r0 -> 3.5.7-r0
#     (OpenSSL heap use-after-free in PKCS7_verify(); HIGH, CVSS 8.0)
# `apk upgrade` (not a literal pin) pulls the newest available within
# alpine:3.22, so the image self-heals across future OpenSSL backports;
# the spec 047 R13 Trivy gate catches anything left behind. smackerel-ml
# (Debian python:3.12-slim) is unaffected and carries its own R14 upgrade.
RUN apk add --no-cache ca-certificates tzdata \
    && apk upgrade --no-cache libssl3 libcrypto3

# SEC-002: Run as non-root user
RUN addgroup -S smackerel && adduser -S smackerel -G smackerel

COPY --from=builder /bin/smackerel-core /usr/local/bin/smackerel-core

USER smackerel

EXPOSE 8080

ENTRYPOINT ["smackerel-core"]
