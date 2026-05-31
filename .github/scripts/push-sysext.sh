#!/usr/bin/env bash
set -eu

for arch in amd64 arm64; do
  config=/tmp/config-${arch}.json
  echo "{\"architecture\":\"${arch}\",\"os\":\"linux\"}" > "$config"

  config_arg="${config}:application/vnd.oci.image.config.v1+json"
  raw=/tmp/systempub-${arch}-${TAG}.raw
  ref="${IMAGE_BASE}:${TAG}-${arch}"
  oras push --config "$config_arg" "$ref" "$raw"
  echo "${arch}_digest=$(oras resolve "$ref")" >> "$GITHUB_OUTPUT"
done

index_ref="${IMAGE_BASE}:${TAG}"
oras manifest index create "$index_ref" "${IMAGE_BASE}:${TAG}-amd64" "${IMAGE_BASE}:${TAG}-arm64"
echo "image=${IMAGE_BASE}" >> "$GITHUB_OUTPUT"
