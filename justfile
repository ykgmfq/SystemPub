version := `awk -F'"' '/version =/ {print $2}' main.go`
export CGO_ENABLED := "0"
export VERSION := version

# test and build
default: test build

# build static binary
build arch="":
    #!/usr/bin/env bash
    set -eu
    output=/tmp/systempub
    if [[ -n "{{arch}}" ]]; then
        output=$output-{{arch}}
        export GOARCH={{arch}}
    fi
    echo Building $output
    go build -o $output -ldflags "-s -w"

# run tests
test:
    go test -v ./...

# build sysext squashfs image for the given architecture (default: amd64)
sysext arch="amd64": (build arch)
    #!/usr/bin/env bash
    set -eu
    staging=$(mktemp -d); trap "rm -rf $staging" EXIT
    mkdir -p $staging/usr/{bin,lib/{systemd/system,sysupdate.d,extension-release.d}}
    cp /tmp/systempub-{{arch}} $staging/usr/bin/systempub
    chmod 755 $staging/usr/bin/systempub
    cp deploy/systempub.service $staging/usr/lib/systemd/system/
    cp deploy/systempub.transfer $staging/usr/lib/sysupdate.d/systempub.transfer
    sys_arch=$([ "{{arch}}" = "amd64" ] && echo "x86-64" || echo "arm64")
    ext_release=$staging/usr/lib/extension-release.d/extension-release.systempub
    printf 'ID=_any\nARCHITECTURE=%s\n' "$sys_arch" > "$ext_release"
    out=/tmp/systempub-{{arch}}-${VERSION}.raw
    mksquashfs $staging $out -noappend -comp zstd -quiet
    echo "Built $out"

# update the devcontianer to the latest Go version
update-devcontainer:
    bash .github/scripts/update-devcontainer.sh
