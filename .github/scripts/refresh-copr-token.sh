#!/usr/bin/env bash
set -eu

# Rotates the COPR API token and writes the new config back into the
# COPR_CONFIG repository secret. The token expires after 180 days, so
# the refresh workflow runs this monthly for a comfortable margin.
#
# Requires COPR_CONFIG (the current config) and GH_TOKEN (a PAT with
# write access to the repository's Actions secrets) in the environment.

mkdir -p ~/.config
printf '%s\n' "$COPR_CONFIG" > ~/.config/copr

# Mint a new token with the old one; this rewrites ~/.config/copr in
# place and immediately invalidates the old token.
copr-cli new-api-token

# Verify the new token works before publishing it.
copr-cli whoami

gh secret set COPR_CONFIG --repo "$GITHUB_REPOSITORY" < ~/.config/copr
echo "COPR_CONFIG secret updated for $GITHUB_REPOSITORY."
grep '# expiration date' ~/.config/copr
