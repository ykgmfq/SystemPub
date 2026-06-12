version := `awk -F'"' '/version =/ {print $2}' main.go`
export CGO_ENABLED := "0"

# test and build
default: test build

# build static binary
build arch="":
    #!/usr/bin/env bash
    set -eu
    output=/tmp/systempub
    if [[ -n "{{ arch }}" ]]; then
        output=$output-{{ arch }}
        export GOARCH={{ arch }}
    fi
    echo Building $output
    go build -o $output -ldflags "-s -w"

# run tests
test:
    go test -v ./...

# build the source RPM
srpm:
    TAG=v{{ version }} bash .github/scripts/build-srpm.sh

# rebuild the binary RPM locally from the source RPM
rpm: srpm
    rpmbuild --rebuild /tmp/srpm/systempub-{{ version }}-*.src.rpm

# build an unsigned binary deb locally
deb:
    #!/usr/bin/env bash
    set -eu
    export DEBEMAIL=free-software@dm-poepperl.de
    export DEBFULLNAME="Dennis M. Pöpperl"
    export TAG=v{{ version }}
    tarball=$(bash .github/scripts/build-tarball.sh | tail -1)
    srcdir=$(dirname "$tarball")/systempub-{{ version }}
    cp -r deploy/debian "$srcdir/debian"
    cd "$srcdir"
    dch --create --package systempub --newversion {{ version }}-1 --distribution UNRELEASED "Local build"
    dpkg-buildpackage --build=binary --no-sign -d
    echo "Built $(ls "$srcdir"/../systempub_*.deb)"

# update the devcontianer to the latest Go version
update-devcontainer:
    bash .github/scripts/update-devcontainer.sh
