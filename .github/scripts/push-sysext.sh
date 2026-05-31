#!/usr/bin/env bash
set -eu

ver=${TAG#v}

for arch in amd64 arm64; do
  config=/tmp/config-${arch}.json
  echo "{\"architecture\":\"${arch}\",\"os\":\"linux\"}" > "$config"

  config_arg="${config}:application/vnd.oci.image.config.v1+json"
  raw=/tmp/systempub-${arch}-${ver}.tar.xz
  ref="${IMAGE_BASE}:${TAG}-${arch}"
  oras push --disable-path-validation --config "$config_arg" "$ref" "$raw"
  echo "${arch}_digest=$(oras resolve "$ref")" >> "$GITHUB_OUTPUT"
done

index_ref="${IMAGE_BASE}:${TAG}"
annotations=(
  --annotation "org.opencontainers.image.title=SystemPub"
  --annotation "org.opencontainers.image.description=Publishes ZFS pool status and snapshots to MQTT for Home Assistant autodiscovery"
  --annotation "org.opencontainers.image.url=https://github.com/ykgmfq/SystemPub"
  --annotation "org.opencontainers.image.source=https://github.com/ykgmfq/SystemPub"
  --annotation "org.opencontainers.image.version=${TAG}"
  --annotation "org.opencontainers.image.revision=${GITHUB_SHA}"
  --annotation "org.opencontainers.image.licenses=GPL-3.0-only"
)
oras manifest index create "$index_ref" "${IMAGE_BASE}:${TAG}-amd64" "${IMAGE_BASE}:${TAG}-arm64" "${annotations[@]}"
echo "image=${IMAGE_BASE}" >> "$GITHUB_OUTPUT"
