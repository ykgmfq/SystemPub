#!/usr/bin/env bash
set -eu

gh release create "$TAG" --title "$TAG" --generate-notes /tmp/systempub-*.tar.xz
