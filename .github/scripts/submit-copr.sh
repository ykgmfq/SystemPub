#!/usr/bin/env bash
set -eu

mkdir -p ~/.config
printf '%s\n' "$COPR_CONFIG" > ~/.config/copr

# Without --nowait this blocks until all chroots finish and fails on any error.
copr-cli build adneos/systempub "$SRPM"
