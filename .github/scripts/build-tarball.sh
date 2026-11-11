#!/usr/bin/env bash
set -eu

# Builds a self-contained source tarball with vendored Go dependencies.
# COPR and Launchpad build without network access, so the tarball must
# carry everything. The file follows the Debian naming scheme, which
# dpkg-source requires and rpmbuild does not care about.
# Prints the path of the tarball on the last line.

ver=${TAG#v}
workdir=$(mktemp -d)
srcdir="$workdir/systempub-$ver"

mkdir -p "$srcdir"
git archive HEAD | tar -x -C "$srcdir"
pushd "$srcdir" > /dev/null
go mod vendor
popd > /dev/null
tarball="$workdir/systempub_$ver.orig.tar.xz"
tar -cJf "$tarball" -C "$workdir" "systempub-$ver"
echo "$tarball"
