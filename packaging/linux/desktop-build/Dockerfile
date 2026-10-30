# Builds offlok-desktop (Wails, GTK 3, WebKitGTK 4.1): Go plus the development files of the libraries it links.
# Debian 12's versions are older than Ubuntu 24.04's, so the binary runs on both (and on Ubuntu 22.04's). Also the
# Windows installer (packaging/windows/build.sh): NSIS, unzip.
FROM golang:1.27.3-bookworm@sha256:972b4110ba0f47539a975c31e52c64765e1ff19f2d74469ae4a595c6676e80a5
RUN apt-get update \
 && apt-get install -y --no-install-recommends libgtk-3-dev libwebkit2gtk-4.1-dev pkg-config nsis unzip \
 && rm -rf /var/lib/apt/lists/*
