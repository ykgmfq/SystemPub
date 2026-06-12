#!/usr/bin/env bash
set -eu

# Builds a self-contained source tarball with vendored Go dependencies.
# COPR and Launchpad build without network access, so the tarball must
# carry everything. Prints the path of the tarball on the last line.

ver=${TAG#v}
workdir=$(mktemp -d)
srcdir="$workdir/systempub-$ver"

mkdir -p "$srcdir"
git archive HEAD | tar -x -C "$srcdir"
pushd "$srcdir" > /dev/null
go mod vendor
popd > /dev/null
tar -cJf "$workdir/systempub-$ver.tar.xz" -C "$workdir" "systempub-$ver"
echo "$workdir/systempub-$ver.tar.xz"
