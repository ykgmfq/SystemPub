ARG GO_TAG
ARG ORAS_TAG
FROM ghcr.io/oras-project/oras:${ORAS_TAG} AS oras
FROM mcr.microsoft.com/devcontainers/go:${GO_TAG}
RUN apt-get update && apt-get install -y just gh && rm -rf /var/lib/apt/lists/*
COPY --from=oras /bin/oras /usr/local/bin/oras
