# syntax=docker/dockerfile:1

# --- Build stage ---
FROM golang:1.24.3-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT_HASH=unknown
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.version=${VERSION} -X main.commitHash=${COMMIT_HASH}" -o /bin/smackerel-core ./cmd/core

# --- Runtime stage ---
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

# SEC-002: Run as non-root user
RUN addgroup -S smackerel && adduser -S smackerel -G smackerel

COPY --from=builder /bin/smackerel-core /usr/local/bin/smackerel-core

USER smackerel

EXPOSE 8080

ENTRYPOINT ["smackerel-core"]
