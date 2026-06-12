---
name: refresh-copr-token
description: Refresh, renew, or rotate the COPR API token and update the COPR_CONFIG GitHub Actions secret. Use when the token is about to expire or the release workflow fails at the "Submit COPR build" step with an authentication error or KeyError copr-cli.
---

# Refresh the COPR token

The release workflow submits SRPMs to [COPR](https://copr.fedorainfracloud.org/coprs/adneos/systempub/) using the `COPR_CONFIG` repository secret.
The secret holds the `[copr-cli]` config block for the `adneos` account, whose API token expires every 180 days.
The expiration date is recorded in a comment inside the config itself.

Rotation normally happens automatically: the "Refresh COPR token" workflow (`.github/workflows/refresh-copr.yml`) runs monthly and needs the `SECRETS_PAT` secret, a fine-grained PAT with write access to this repository's Actions secrets.
This skill is the manual path for when that workflow is broken, its PAT has expired, or the token must be replaced from scratch.

All paths below are relative to the repository root.
`copr-cli` is baked into the devcontainer image (see `.devcontainer/Dockerfile`).

## Refresh (agent path)

Run the driver:

```sh
bash .claude/skills/refresh-copr-token/refresh.sh
```

It mints a new token with `copr-cli new-api-token` (which rewrites `~/.config/copr` in place), verifies it with `copr-cli whoami`, and uploads the new config with `gh secret set COPR_CONFIG`.

Prerequisites the driver assumes:

- A valid, unexpired token in `~/.config/copr` (the old token is used to mint the new one).
- `gh` authenticated with access to the repository (`gh auth status` to check).

## Refresh (token already expired)

`new-api-token` authenticates with the old token, so it cannot recover from an expired one.
In that case log in at <https://copr.fedorainfracloud.org/api/> in a browser, copy the displayed config block into `~/.config/copr`, and run the driver again to verify it and update the secret.

## Verify

```sh
copr-cli whoami                           # prints: adneos
gh secret list -R ykgmfq/SystemPub        # shows COPR_CONFIG with a fresh timestamp
grep '# expiration date' ~/.config/copr   # shows the new expiry
```

## Gotchas

- Rotating the token immediately invalidates the old one.
  Do not rotate while a release workflow run is in flight, or its "Submit COPR build" step will fail with the stale secret.
- `pip install copr-cli` does not pull in `rich`, which copr-cli 2.5 imports at startup.
  The devcontainer Dockerfile installs it explicitly; symptom otherwise is `ModuleNotFoundError: No module named 'rich'`.
- `new-api-token` requires `~/.config/copr` to exist, be writable, and contain `login`/`token` keys; it refuses to run for gssapi-only configs.

## Troubleshooting

- CI fails with `KeyError: 'copr-cli'` in the "Submit COPR build" step: the `COPR_CONFIG` secret is empty or missing.
  Run the driver, or set the secret manually with `gh secret set COPR_CONFIG -R ykgmfq/SystemPub < ~/.config/copr`.
- CI fails with `Error: Project adneos/systempub does not exist`: the COPR project was deleted.
  Recreate it: `copr-cli create systempub --chroot fedora-44-x86_64 --chroot fedora-44-aarch64`.
