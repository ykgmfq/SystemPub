ARG GO_TAG
ARG ORAS_TAG
FROM mcr.microsoft.com/devcontainers/go:${GO_TAG}
RUN apt-get update && apt-get install -y just && rm -rf /var/lib/apt/lists/*
COPY --from=ghcr.io/oras-project/oras:${ORAS_TAG} /bin/oras /usr/local/bin/oras
