#!/usr/bin/env bash
set -eu

# Builds a signed source package and uploads it to the Launchpad PPA.
# Launchpad builds the binaries offline for the targeted series,
# so the tarball carries vendored dependencies.

export DEBEMAIL="free-software@dm-poepperl.de"
export DEBFULLNAME="Dennis M. Pöpperl"

series=resolute
ubuntu_version=26.04
ver=${TAG#v}
debversion="$ver-1ppa1~ubuntu$ubuntu_version"

tarball=$(bash .github/scripts/build-tarball.sh | tail -1)
workdir=$(dirname "$tarball")
srcdir="$workdir/systempub-$ver"

# dpkg-source expects the upstream tarball under the Debian naming scheme.
mv "$tarball" "$workdir/systempub_$ver.orig.tar.xz"

# The changelog is generated here so the version stays tag-driven,
# like the spec used for the RPM.
cp -r deploy/debian "$srcdir/debian"
pushd "$srcdir"
dch --create --package systempub --newversion "$debversion" --distribution "$series" "Release $TAG, see https://github.com/ykgmfq/SystemPub/releases/tag/$TAG"

# Sign with the only secret key in the keyring.
# The build dependencies only matter on the Launchpad builder,
# so -d skips checking them on the submitting machine.
keyid=$(gpg --list-secret-keys --with-colons | awk -F: '/^fpr/ {print $10; exit}')
debuild -S -d -k"$keyid"
popd

dput ppa:ykgmfq/systempub "$workdir/systempub_${debversion}_source.changes"
