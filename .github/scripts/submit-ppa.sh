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

# Launchpad permanently rejects re-uploads of a version it has accepted.
# Bump PPA_REVISION when a fixed package for the same release must go out.
# Such a re-upload cannot run through this script if the repository moved,
# because the orig tarball must stay byte-identical to the accepted one:
# download it from the PPA, unpack it, copy deploy/debian in, and build
# with debuild -S -d -sd so the orig is referenced but not re-uploaded.
revision=${PPA_REVISION:-1}
debversion="$ver-1ppa$revision~ubuntu$ubuntu_version"

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
