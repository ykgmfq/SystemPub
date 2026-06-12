#!/usr/bin/env bash
set -eu

ver=${TAG#v}
tarball=$(bash .github/scripts/build-tarball.sh | tail -1)
workdir=$(dirname "$tarball")

# Write the release version into the spec.
# The spec travels inside the SRPM and COPR re-evaluates it there,
# so it must carry a literal version instead of a macro.
spec="$workdir/systempub.spec"
sed "s/^Version:.*/Version:        $ver/" deploy/systempub.spec > "$spec"

# Build a distro-neutral SRPM; each COPR chroot re-evaluates %dist itself.
srpm_dir=/tmp/srpm
rpmbuild -bs "$spec" --define "_sourcedir $workdir" --define "_srcrpmdir $srpm_dir" --undefine dist
srpm=$(ls "$srpm_dir"/systempub-"$ver"-*.src.rpm)
echo "Built $srpm"
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "srpm=$srpm" >> "$GITHUB_OUTPUT"
fi
