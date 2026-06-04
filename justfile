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
    if [[ -n "{{ arch }}" ]]; then
        output=$output-{{ arch }}
        export GOARCH={{ arch }}
    fi
    echo Building $output
    go build -o $output -ldflags "-s -w"

# run tests
test:
    go test -v ./...

# package binary into sysext archive (binary must already be at /tmp/systempub-{{ arch }})
package arch="amd64":
    #!/usr/bin/env bash
    set -eu
    staging=$(mktemp -d); trap "rm -rf $staging" EXIT
    mkdir -p $staging/usr/{bin,lib/{systemd/system,extension-release.d}}
    cp /tmp/systempub-{{ arch }} $staging/usr/bin/systempub
    chmod 755 $staging/usr/bin/systempub
    cp deploy/systempub.service $staging/usr/lib/systemd/system/
    sys_arch=$([ "{{ arch }}" = "amd64" ] && echo "x86-64" || echo "arm64")
    ext_release=$staging/usr/lib/extension-release.d/extension-release.systempub
    printf 'ID=_any\nARCHITECTURE=%s\nSYSEXT_SCOPE=system\n' "$sys_arch" > "$ext_release"
    out=/tmp/systempub-{{ arch }}-${VERSION}.tar.xz
    tar -cJf "$out" -C "$staging" .
    echo "Built $out"

# build binary and package into sysext archive
sysext arch="amd64": (build arch) (package arch)

# verify the OCI push flow locally using a temporary OCI image layout
test-push: (sysext "amd64") (sysext "arm64")
    #!/usr/bin/env bash
    set -eu
    layout=$(mktemp -d); trap "rm -rf $layout" EXIT
    tag=v{{ version }}
    for arch in amd64 arm64; do
        config=/tmp/config-${arch}.json
        echo "{\"architecture\":\"${arch}\",\"os\":\"linux\"}" > "$config"
        config_arg="${config}:application/vnd.oci.image.config.v1+json"
        raw=/tmp/systempub-${arch}-{{ version }}.tar.xz
        oras push --oci-layout --disable-path-validation --config "$config_arg" "${layout}:${tag}-${arch}" "$raw"
        echo "${arch} digest: $(oras resolve --oci-layout "${layout}:${tag}-${arch}")"
    done
    oras manifest index create --oci-layout "${layout}:${tag}" "${tag}-amd64" "${tag}-arm64"
    oras manifest fetch --oci-layout "${layout}:${tag}" | jq ".manifests"

# update the devcontianer to the latest Go version
update-devcontainer:
    bash .github/scripts/update-devcontainer.sh
