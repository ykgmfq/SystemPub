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

# update the devcontianer to the latest Go version
update-devcontainer:
    bash .github/scripts/update-devcontainer.sh
