# The binary is built static and stripped, so no debuginfo can be extracted.
%global debug_package %{nil}

Name:           systempub
# The version is substituted in by build-srpm.sh; the spec inside the SRPM
# must carry a literal version because COPR re-evaluates it without macros.
Version:        0
Release:        1%{?dist}
Summary:        Publish ZFS pool and systemd unit state to MQTT for Home Assistant
License:        GPL-3.0-only
URL:            https://github.com/ykgmfq/SystemPub
Source0:        %{name}_%{version}.orig.tar.xz

BuildRequires:  golang >= 1.26
BuildRequires:  systemd-rpm-macros

%description
SystemPub monitors ZFS pools and systemd units and reports their state
to Home Assistant via MQTT autodiscovery.

%prep
%autosetup

%build
export CGO_ENABLED=0
export GOPROXY=off
go build -mod=vendor -ldflags "-s -w" -o systempub .

%check
go test -mod=vendor ./...

%install
install -Dpm 0755 systempub %{buildroot}%{_bindir}/systempub
install -Dpm 0644 deploy/systempub.service %{buildroot}%{_unitdir}/systempub.service
install -Dpm 0644 deploy/systempub-firedrill.service %{buildroot}%{_unitdir}/systempub-firedrill.service
install -Dpm 0644 deploy/systempub-firedrill.timer %{buildroot}%{_unitdir}/systempub-firedrill.timer
# Ship a preset so %%systemd_post enables the service on first install but leaves the fire drill timer off.
install -Dpm 0644 deploy/85-systempub.preset %{buildroot}%{_presetdir}/85-systempub.preset

%post
%systemd_post systempub.service systempub-firedrill.timer

%preun
%systemd_preun systempub.service systempub-firedrill.timer

%postun
%systemd_postun_with_restart systempub.service
%systemd_postun systempub-firedrill.timer

%files
%license LICENSE
%doc README.md
%{_bindir}/systempub
%{_unitdir}/systempub.service
%{_unitdir}/systempub-firedrill.service
%{_unitdir}/systempub-firedrill.timer
%{_presetdir}/85-systempub.preset
