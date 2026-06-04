#!/bin/bash
set -e

# https://regex101.com/r/Gi9d3E/1
LATEST_TAG=$(curl --silent https://mcr.microsoft.com/v2/devcontainers/go/tags/list | jq --raw-output '.tags[] | select(test("^1\\.\\d+$"))' | sort --version-sort | tail --lines 1)
CURRENT_TAG=$(jq --raw-output '.build.args.GO_TAG' .devcontainer/devcontainer.json)

if [ "$CURRENT_TAG" = "$LATEST_TAG" ]; then
  if [ -n "$GITHUB_OUTPUT" ]; then
    echo "result=up-to-date" >> "$GITHUB_OUTPUT"
  fi
  exit 0
fi
jq --arg tag "$LATEST_TAG" '.build.args.GO_TAG = $tag' .devcontainer/devcontainer.json > /tmp/devcontainer.json
mv /tmp/devcontainer.json .devcontainer/
if [ -n "$GITHUB_OUTPUT" ]; then
  echo "tag=$LATEST_TAG" >> "$GITHUB_OUTPUT"
  echo "result=updated" >> "$GITHUB_OUTPUT"
fi
echo "$LATEST_TAG"
