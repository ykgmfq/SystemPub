#!/usr/bin/env bash
set -eu

pushd /tmp
sha256sum systempub-*.tar.xz > SHA256SUMS
popd
gh release create "$TAG" --title "$TAG" --generate-notes /tmp/systempub-*.tar.xz /tmp/SHA256SUMS
