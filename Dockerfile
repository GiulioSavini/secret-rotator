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
      -X github.com/giulio/secret-rotator/internal/interfaces/cli.version=${VERSION} \
      -X github.com/giulio/secret-rotator/internal/interfaces/cli.commit=${COMMIT} \
      -X github.com/giulio/secret-rotator/internal/interfaces/cli.date=${DATE}" \
    -o /rotator ./cmd/rotator

# Create the mount points with the right ownership. distroless has no shell,
# so the directories must exist in the image before the runtime stage. The
# .keep files make the COPY below deterministic: Docker copies a directory's
# contents, not the directory itself, so an empty one is not reliable.
# /data matters in particular: a fresh named volume inherits the ownership of
# the image directory it is mounted over.
RUN mkdir -p /out/config /out/data \
    && touch /out/config/.keep /out/data/.keep \
    && chown -R 65532:65532 /out

# Stage 2: Runtime
FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.source="https://github.com/GiulioSavini/secret-rotator"
LABEL org.opencontainers.image.title="secret-rotator"
LABEL org.opencontainers.image.description="Automatic secret rotation for self-hosted Docker environments"
LABEL io.secret-rotator.volumes.socket="/var/run/docker.sock - Docker socket for container management"
LABEL io.secret-rotator.volumes.config="/config - project directory holding rotator.yml and the .env files"
LABEL io.secret-rotator.volumes.data="/data - writable volume for the encrypted rotation history"

COPY --from=builder /rotator /rotator
COPY --from=builder --chown=65532:65532 /out/config /config
COPY --from=builder --chown=65532:65532 /out/data /data

# /config is both the working directory and the first place the config file is
# looked up, so mounting the project there is all the setup that is required.
WORKDIR /config

# Keep the rotation history on its own volume rather than inside the mounted
# project, so history survives a recreate of the project directory.
# Note: /config must stay writable -- rotation rewrites the .env files in place
# (atomically, via a temp file in the same directory).
ENV ROTATOR_DATA_DIR=/data

VOLUME ["/data"]

ENTRYPOINT ["/rotator"]
CMD ["daemon"]
