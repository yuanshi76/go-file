# syntax=docker/dockerfile:1

# ---- Build stage ----
# Alpine + musl so we can produce a fully static CGO binary (mattn/go-sqlite3
# requires CGO) that runs on a bare alpine runtime.
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache gcc musl-dev

# _LARGEFILE64_SOURCE re-exposes pread64/pwrite64/off64_t, which musl 1.2.4+
# (Alpine 3.18+) hides by default and which mattn/go-sqlite3's bundled SQLite
# still references. Without it the CGO build fails with "off64_t undeclared".
ENV CGO_ENABLED=1 \
    GOOS=linux \
    CGO_CFLAGS=-D_LARGEFILE64_SOURCE \
    GOPROXY=https://goproxy.cn,direct

WORKDIR /build

# Download dependencies first so this layer is cached across source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# VERSION is injected by CI (defaults to "dev" for local builds) and embedded
# into the binary via -ldflags, so no VERSION file is required.
ARG VERSION=dev
RUN go build \
    -ldflags "-s -w -X 'go-file/common.Version=${VERSION}' -linkmode external -extldflags '-static'" \
    -o go-file .

# ---- Runtime stage ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
    && update-ca-certificates 2>/dev/null || true

ENV PORT=3000
COPY --from=builder /build/go-file /go-file

# SQLite DB (go-file.db) and uploads live under the working directory, so mount
# a volume here to persist data across container restarts.
WORKDIR /data
VOLUME ["/data"]
EXPOSE 3000

# -no-browser: never try to launch a browser from inside the container.
# Override CMD to pass other flags, e.g. docker run ... -port 8080 -enable-p2p.
ENTRYPOINT ["/go-file"]
CMD ["-no-browser"]
