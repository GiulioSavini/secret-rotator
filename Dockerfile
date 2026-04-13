# Stage 1: Build
FROM golang:1.26-alpine AS builder

WORKDIR /build

# Cache dependency downloads
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w \
      -X github.com/giulio/secret-rotator/internal/cli.version=${VERSION} \
      -X github.com/giulio/secret-rotator/internal/cli.commit=${COMMIT} \
      -X github.com/giulio/secret-rotator/internal/cli.date=${DATE}" \
    -o /rotator ./cmd/rotator

# Stage 2: Runtime
FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.source="https://github.com/giulio/secret-rotator"
LABEL org.opencontainers.image.title="secret-rotator"
LABEL org.opencontainers.image.description="Automatic secret rotation for self-hosted Docker environments"
LABEL io.secret-rotator.volumes.socket="/var/run/docker.sock - Docker socket for container management"
LABEL io.secret-rotator.volumes.config="/config - Mount rotator.yml and .env files"

COPY --from=builder /rotator /rotator

ENTRYPOINT ["/rotator"]
