#!/usr/bin/env bash
set -eu

srpm_dir=$(dirname "$SRPM")
pushd "$srpm_dir"
sha256sum ./*.src.rpm > SHA256SUMS
popd
gh release create "$TAG" --title "$TAG" --generate-notes "$SRPM" "$srpm_dir/SHA256SUMS"
