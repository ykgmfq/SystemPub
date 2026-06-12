ARG GO_TAG
FROM mcr.microsoft.com/devcontainers/go:${GO_TAG}
RUN apt-get update && apt-get install -y just gh python3-venv && rm -rf /var/lib/apt/lists/*
# copr-cli is not packaged for Debian, so install it into its own virtualenv.
# rich is an undeclared runtime dependency of copr-cli 2.5.
RUN python3 -m venv /opt/copr-cli && /opt/copr-cli/bin/pip install --no-cache-dir copr-cli rich && ln -s /opt/copr-cli/bin/copr-cli /usr/local/bin/copr-cli
