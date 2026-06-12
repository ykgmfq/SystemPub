#!/usr/bin/env bash
set -eu

ver=${TAG#v}
workdir=$(mktemp -d)
srcdir="$workdir/systempub-$ver"

# Stage the tracked sources only, then vendor the Go dependencies.
# COPR builds run without network access, so the tarball must be self-contained.
mkdir -p "$srcdir"
git archive HEAD | tar -x -C "$srcdir"
pushd "$srcdir"
go mod vendor
popd
tar -czf "$workdir/systempub-$ver.tar.gz" -C "$workdir" "systempub-$ver"

# Build a distro-neutral SRPM; each COPR chroot re-evaluates %dist itself.
srpm_dir=/tmp/srpm
rpmbuild -bs deploy/systempub.spec --define "pkgver $ver" --define "_sourcedir $workdir" --define "_srcrpmdir $srpm_dir" --undefine dist
srpm=$(ls "$srpm_dir"/systempub-"$ver"-*.src.rpm)
echo "Built $srpm"
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "srpm=$srpm" >> "$GITHUB_OUTPUT"
fi
