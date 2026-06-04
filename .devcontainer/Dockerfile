ARG GO_TAG
FROM mcr.microsoft.com/devcontainers/go:${GO_TAG}

RUN apt-get update && apt-get install -y just && rm -rf /var/lib/apt/lists/*

COPY --from=ghcr.io/oras-project/oras:v1.3.2 /bin/oras /usr/local/bin/oras
