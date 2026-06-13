# Deployment

Packaging artifacts for the RPM (COPR) and Debian (Launchpad PPA) builds, plus the shared systemd unit.

## Service activation

The service is enabled by default on both distributions.
Express enablement through each distribution's native mechanism, never through manual `systemctl` calls in maintainer scripts.

Let each distribution follow its own convention for starting on install rather than forcing them to match:
Debian starts the service on install, and Fedora only enables it for the next boot.
This asymmetry is intentional; do not try to unify it.

## Configuration fallback

A missing configuration file is not fatal: the binary falls back to built-in defaults, so the service can come up on a fresh install.
A malformed configuration file is still fatal.
