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
Source0:        %{name}-%{version}.tar.xz

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

%post
%systemd_post systempub.service

%preun
%systemd_preun systempub.service

%postun
%systemd_postun_with_restart systempub.service

%files
%license LICENSE
%doc README.md
%{_bindir}/systempub
%{_unitdir}/systempub.service

%changelog
* Thu Jun 11 2026 Dennis M. Pöpperl <accounts@dm-poepperl.de> - 1.4.0-1
- Switch distribution from systemd-sysext images to an RPM built on COPR
