#!/usr/bin/env bash
set -eu

cd /tmp
sha256sum systempub-*.tar.xz > SHA256SUMS
assets=(systempub-*.tar.xz SHA256SUMS)
gh release create "$TAG" --title "$TAG" --generate-notes "${assets[@]}"
