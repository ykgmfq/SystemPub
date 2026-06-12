#!/usr/bin/env bash
set -eu

# Refreshes the COPR API token and stores the new config as the
# COPR_CONFIG GitHub Actions secret used by the release workflow.
#
# Requires a valid (unexpired) token in ~/.config/copr.
# If the token has already expired, log in at
# https://copr.fedorainfracloud.org/api/ in a browser, save the
# config block to ~/.config/copr, and run this script again.

repo="ykgmfq/SystemPub"
config="$HOME/.config/copr"

if ! command -v copr-cli > /dev/null; then
    echo "copr-cli not found; it is baked into the devcontainer image." >&2
    echo "Rebuild the devcontainer to get it." >&2
    exit 1
fi

if [ ! -f "$config" ]; then
    echo "No $config found." >&2
    echo "Save the config from https://copr.fedorainfracloud.org/api/ there and rerun." >&2
    exit 1
fi

# Mint a new token with the old one; this rewrites $config in place
# and immediately invalidates the old token.
copr-cli new-api-token

# Verify the new token works before publishing it to GitHub.
user=$(copr-cli whoami)
echo "Token valid for COPR user: $user"

gh secret set COPR_CONFIG --repo "$repo" < "$config"
echo "COPR_CONFIG secret updated for $repo."
grep '# expiration date' "$config"
