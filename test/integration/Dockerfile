# A disposable Ubuntu desktop-like system for OffLok's integration tests: systemd as PID 1, systemd-resolved
# owning /etc/resolv.conf, the distribution's Unbound, polkit. Everything runs in the container's own network
# namespace. dns-root-data comes with unbound as a recommended package on a real install; the distribution
# unbound.service needs it. The "desktop" stage adds what offlok-desktop needs and a virtual screen to look at it.
FROM ubuntu:24.04 AS base
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
 && apt-get install -y --no-install-recommends systemd systemd-sysv systemd-resolved dbus unbound dns-root-data dnsutils \
      iproute2 iptables ca-certificates tcpdump polkitd busybox \
 && rm -rf /var/lib/apt/lists/* \
 && systemctl disable unbound.service \
 && systemctl mask systemd-sysctl.service systemd-udevd.service systemd-udev-trigger.service \
      systemd-modules-load.service sys-kernel-config.mount sys-kernel-debug.mount getty.target \
      systemd-timesyncd.service \
 && systemctl enable systemd-resolved.service
# Docker mounts the container's own cgroup tree read-only; systemd needs it writable. With a private cgroup
# namespace the remount only reaches this container's subtree (see run.sh for the exact docker flags).
STOPSIGNAL SIGRTMIN+3
CMD ["/bin/sh", "-c", "mount -o remount,rw /sys/fs/cgroup && exec /sbin/init"]

FROM base AS desktop
RUN apt-get update \
 && apt-get install -y --no-install-recommends libgtk-3-0t64 libwebkit2gtk-4.1-0 xvfb dbus-x11 imagemagick \
      fonts-dejavu-core locales \
 && sed -i 's/^# *fr_FR.UTF-8/fr_FR.UTF-8/' /etc/locale.gen && locale-gen \
 && rm -rf /var/lib/apt/lists/*
