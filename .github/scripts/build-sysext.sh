#!/usr/bin/env bash
set -eu

ver="${TAG#v}"
for arch in amd64 arm64; do
  just VERSION="$ver" sysext "$arch"
done
