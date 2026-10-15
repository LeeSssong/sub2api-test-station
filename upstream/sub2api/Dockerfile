# Bounded application-only release: reuse the verified production runtime.
# Before using, verify token-guard requirements, vendor inputs, entrypoint, and
# all other runtime dependencies match the RUNTIME_IMAGE source commit.
# Compile application code and embed frontend from this checkout.
# =============================================================================
# Sub2API Multi-Stage Dockerfile
# =============================================================================
# Stage 1: Build frontend
# Stage 2: Build Go backend with embedded frontend
# Stage 3: Final minimal image
# =============================================================================

# Official registry digests avoid the mirror authentication failure.
ARG NODE_IMAGE=docker.io/library/node:24-alpine@sha256:ebfe2f90462722a7a4de65e91990e97fe0d401c70e0e762c5b53302f905ec1c1
ARG GOLANG_IMAGE=docker.io/library/golang:1.27.2-alpine
ARG ALPINE_IMAGE=docker.io/library/alpine:3.21@sha256:ce64758a109eb420d874a118f87920e625e12d3634e03b4a5573fd9f6e5d3507
ARG POSTGRES_IMAGE=docker.io/library/postgres:18-alpine@sha256:77f585114c32fbca283dc835b0596f4e52b51b4c6662d7810b2f4084f60a1873
ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn
ARG NPM_CONFIG_REGISTRY=
ARG RUNTIME_IMAGE=ghcr.io/leesssong/xingqiao-sub2api:release-15679c9de17731413dc02c8b4594442a8b540883-514c0def75a304fcbbd36b3f77d363bf97b963d2b46dbd9dd0248d101d0b5889
ARG GPT56_VENDOR_IMAGE=ghcr.io/leesssong/xingqiao-sub2api:release-5e098c4a91865853312fe5bdc869d9d15c00eb3f-e3e958d95cf12c08acbb74f6449114ef7069a3ad92527d80da1bab9eb9503c16

# -----------------------------------------------------------------------------
# Stage 1: Frontend Builder
# -----------------------------------------------------------------------------
# --platform=$BUILDPLATFORM: the frontend output is JS (arch-neutral), so build
# it on the native host arch instead of under QEMU emulation for the target.
FROM --platform=${BUILDPLATFORM} ${NODE_IMAGE} AS frontend-builder
ARG NPM_CONFIG_REGISTRY
ARG FRONTEND_NODE_MAX_OLD_SPACE_SIZE=3072
ARG VITE_APP_BASE_PATH=/
ARG VITE_API_BASE_URL=/api/v1
ARG VITE_AUTH_STORAGE_PREFIX=

WORKDIR /app/frontend
ENV NODE_OPTIONS=--max-old-space-size=${FRONTEND_NODE_MAX_OLD_SPACE_SIZE}

# Install pnpm (pinned to v9 to match CI and keep builds reproducible)
RUN corepack enable && corepack prepare pnpm@9 --activate

# Install dependencies first (better caching)
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN --mount=type=cache,id=sub2api-pnpm-store,target=/root/.local/share/pnpm/store \
    if [ -n "${NPM_CONFIG_REGISTRY}" ]; then pnpm config set registry "${NPM_CONFIG_REGISTRY}"; fi && \
    pnpm install --frozen-lockfile --prefer-offline

# Copy frontend source and build.
# LegalDocumentView.vue (admin-compliance gate) build-time imports
# ../../../../docs/legal/*.md?raw, so docs/legal/ must sit beside frontend/
# in the image (WORKDIR /app/frontend -> resolves to /app/docs/legal/*.md).
# Copy only that subtree to keep the build dependency minimal.
COPY frontend/ ./
COPY docs/legal/ /app/docs/legal/
RUN VITE_APP_BASE_PATH="${VITE_APP_BASE_PATH}" \
    VITE_API_BASE_URL="${VITE_API_BASE_URL}" \
    VITE_AUTH_STORAGE_PREFIX="${VITE_AUTH_STORAGE_PREFIX}" \
    pnpm run build

# -----------------------------------------------------------------------------
# Stage 2: Backend Builder
# -----------------------------------------------------------------------------
# --platform=$BUILDPLATFORM: run the Go toolchain on the native host arch and
# cross-compile to the target arch below. The binary is CGO_ENABLED=0, so this
# is a clean pure-Go cross-compile — no QEMU emulation of go mod download / go
# build (emulated networking here was dropping module fetches with EOF).
FROM --platform=${BUILDPLATFORM} ${GOLANG_IMAGE} AS backend-builder

# Build arguments for version info (set by CI)
ARG VERSION=
ARG COMMIT=docker
ARG DATE
ARG GOPROXY
ARG GOSUMDB
# Populated by buildx from the --platform target (e.g. linux/amd64).
ARG TARGETOS
ARG TARGETARCH

ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}

# Install build dependencies

WORKDIR /app/backend

# Copy go mod files first (better caching)
COPY backend/go.mod backend/go.sum ./
# Cache mount keeps the module cache across builds so a transient CDN blip on
# retry resumes instead of re-fetching every zip from scratch.
RUN --mount=type=cache,id=sub2api-gomod,target=/go/pkg/mod \
    go mod download

# Copy backend source first
COPY backend/ ./

# Copy frontend dist from previous stage (must be after backend copy to avoid being overwritten)
COPY --from=frontend-builder /app/backend/internal/web/dist ./internal/web/dist

# Build the binary (BuildType=release for CI builds, embed frontend)
# Version precedence: build arg VERSION > exact git tag > cmd/server/VERSION
RUN --mount=type=cache,id=sub2api-gomod,target=/go/pkg/mod \
    --mount=type=cache,id=sub2api-gobuild,target=/root/.cache/go-build \
    VERSION_VALUE="${VERSION}" && \
    if [ -z "${VERSION_VALUE}" ]; then VERSION_VALUE="$(./scripts/resolve-version.sh)"; fi && \
    DATE_VALUE="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}" && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build \
    -tags embed \
    -ldflags="-s -w -X main.Version=${VERSION_VALUE} -X main.Commit=${COMMIT} -X main.Date=${DATE_VALUE} -X main.BuildType=release" \
    -trimpath \
    -o /app/sub2api \
    ./cmd/server

RUN --mount=type=cache,id=sub2api-gomod,target=/go/pkg/mod \
    --mount=type=cache,id=sub2api-gobuild,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w" -trimpath \
    -o /app/model-detector \
    ./cmd/model-detector

# -----------------------------------------------------------------------------
# Stage 3: PostgreSQL Client (version-matched with docker-compose)
# -----------------------------------------------------------------------------
FROM ${POSTGRES_IMAGE} AS pg-client

# Reuse the checksum-verified vendor files from the current production image.
FROM ${GPT56_VENDOR_IMAGE} AS gpt56-v411-vendor

# -----------------------------------------------------------------------------
# Stage 4: Final Runtime Image
# -----------------------------------------------------------------------------
FROM ${RUNTIME_IMAGE}

# One isolated offline screenshot per intelligence drawing; app remains unprivileged.
RUN apk add --no-cache chromium font-noto-cjk
ENV INTELLIGENCE_CHROMIUM_PATH=/usr/bin/chromium-browser

# Labels
LABEL maintainer="Wei-Shaw <github.com/Wei-Shaw>"
LABEL description="Sub2API - AI API Gateway Platform"
LABEL org.opencontainers.image.source="https://github.com/Wei-Shaw/sub2api"

# Native credential guard executor, pinned dependencies and upstream sources.
RUN --mount=type=bind,source=token-guard/requirements.txt,target=/tmp/token-guard-requirements.txt,readonly \
    cmp /tmp/token-guard-requirements.txt /app/token-guard/requirements.txt
COPY token-guard /app/token-guard
COPY Dockerfile deploy/docker-entrypoint.sh /app/token-guard/build/
# The verified runtime retains its PostgreSQL clients and dependency environment.

# Set working directory
WORKDIR /app

# Copy binary/resources with ownership to avoid extra full-layer chown copy
COPY --from=backend-builder --chown=sub2api:sub2api /app/sub2api /app/sub2api
COPY --from=backend-builder --chown=sub2api:sub2api /app/model-detector /app/model-detector
COPY --from=backend-builder --chown=sub2api:sub2api /app/backend/resources /app/resources

# This image was built from the pinned v4.1.1 archive and verified SHA256.
# Its full image ID is embedded in the source image tag.
COPY --from=gpt56-v411-vendor --chown=sub2api:sub2api /app/vendor/gpt56-v411 /app/vendor/gpt56-v411

COPY --chown=sub2api:sub2api deploy/model-detector-v411-adapter.py /app/model-detector
RUN chmod 0555 /app/model-detector && chown -R sub2api:sub2api /app/vendor/gpt56-v411

# Create data directory
RUN mkdir -p /app/data && chown sub2api:sub2api /app/data

# Copy entrypoint script (fixes volume permissions then drops to sub2api)
COPY deploy/docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh

# Expose port (can be overridden by SERVER_PORT env var)
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget -q -T 5 -O /dev/null http://localhost:${SERVER_PORT:-8080}/health || exit 1

# Run the application (entrypoint fixes /app/data ownership then execs as sub2api)
ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["/app/sub2api"]
