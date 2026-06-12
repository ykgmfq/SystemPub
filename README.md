# SystemPub

SystemPub monitors your ZFS pools and systemd units, and reports their state to Home Assistant via MQTT.
It is built for [sanoid](https://github.com/jimsalterjrs/sanoid) users who want simple, self-hosted monitoring without running a full monitoring stack or relying on a cloud service.

It tracks ZFS pool health and capacity, sanoid snapshot freshness, and failed systemd units.
The last point makes it easy to notice when `syncoid` has silently stopped syncing snapshots on a backup machine.

All sensors appear in Home Assistant automatically through MQTT autodiscovery — no extra configuration required.

![Screenshot](.github/demo_home_assistant.png)

## Getting started

SystemPub is distributed as an RPM via [COPR](https://copr.fedorainfracloud.org/coprs/adneos/systempub/).
On Fedora CoreOS, download the repository file and layer the package:

```sh
sudo curl -o /etc/yum.repos.d/systempub.repo https://copr.fedorainfracloud.org/coprs/adneos/systempub/repo/fedora-44/adneos-systempub-fedora-44.repo
sudo rpm-ostree install systempub
```

On Ubuntu, install from the [PPA](https://launchpad.net/~ykgmfq/+archive/ubuntu/systempub):

```sh
sudo add-apt-repository ppa:ykgmfq/systempub
sudo apt install systempub
```

Check the [wiki](https://github.com/ykgmfq/SystemPub/wiki) for the full installation guide!
