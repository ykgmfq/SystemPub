#!/bin/bash
set -e

# https://regex101.com/r/Gi9d3E/1
LATEST_TAG=$(curl --silent https://mcr.microsoft.com/v2/devcontainers/go/tags/list | jq --raw-output '.tags[]' | grep --perl-regexp '^1\.\d+$' | sort --version-sort | tail --lines 1)
CURRENT_TAG=$(jq --raw-output '.image' .devcontainer/devcontainer.json | cut --delimiter=":" --fields=2)

if [ "$CURRENT_TAG" = "$LATEST_TAG" ]; then
  exit 2
fi
jq --arg latest_image "mcr.microsoft.com/devcontainers/go:$LATEST_TAG" '.image = $latest_image' .devcontainer/devcontainer.json > /tmp/devcontainer.json
mv /tmp/devcontainer.json .devcontainer/
echo $LATEST_TAG
echo "tag=$LATEST_TAG" >> $GITHUB_OUTPUT
