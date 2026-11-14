# OffLok's build image on Linux, for every step of the CI and the releases (its Go is the one go.mod's toolchain
# line names). Builds offlok-desktop (Wails, GTK 3, WebKitGTK 4.1): Go plus the development files of the libraries
# it links.
# Debian 12's versions are older than Ubuntu 24.04's, so the binary runs on both (and on Ubuntu 22.04's). Also the
# Windows installer (packaging/windows/build.sh): NSIS, unzip.
FROM golang:1.27.4-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea
RUN apt-get update \
 && apt-get install -y --no-install-recommends libgtk-3-dev libwebkit2gtk-4.1-dev pkg-config nsis unzip \
 && rm -rf /var/lib/apt/lists/*
