# ---- Build Stage ----
FROM golang:1.26-alpine AS builder

# Every binary goes in one image; entrypoint.sh picks one at runtime.
ARG SERVICES="blog_platform loadgen"

ENV CGO_ENABLED=0 \
    PATH="$PATH:/go/bin"

# No protoc here: codegen goes through buf, which has its own compiler.
RUN apk add --no-cache make

WORKDIR /app

# 1. Layer caching: dependency files first
COPY go.mod go.sum Makefile tools.txt ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    make install-deps

# 2. Source, codegen and build
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    make build-services SERVICES="${SERVICES}"

# ---- Final Stage ----
FROM alpine:3.23

ENV BINARY=blog_platform
RUN adduser -D backend
WORKDIR /home/backend
USER backend

COPY --from=builder --chown=backend:backend /app/bin/ ./
COPY --from=builder --chown=backend:backend /app/entrypoint.sh ./
# kept so utils.GetProjectRoot finds the project folder (and configs/ next to it)
COPY --from=builder --chown=backend:backend /app/go.mod ./
COPY --from=builder --chown=backend:backend /app/configs/ ./configs/

ENTRYPOINT ["./entrypoint.sh"]
