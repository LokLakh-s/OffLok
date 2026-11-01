# Changelog

What changes from one OffLok release to the next. Each release's notes on GitHub start with its section here.

## 0.6.0

- **macOS**, 13 or later, Apple silicon and Intel: `offlok-0.6.0.pkg` installs offlokd under launchd, Unbound
  1.26.2 built with OpenSSL 3.5.9, OffLok in the menu bar with its window, and the `offlok` command. Every network
  service points at OffLok; `sudo /usr/local/libexec/offlok/uninstall.sh` gives each its original DNS back. The
  package is not signed yet: macOS asks to allow it once, in System Settings › Privacy & Security.
- **Android**, 9 or later: `offlok-0.6.0.apk` makes the phone its own DNS, through a VPN that carries DNS queries
  only. Unbound 1.26.2 and OpenSSL 3.5.9 run inside the app; DNSSEC is checked on the phone; the fallback, strict
  mode and blocking work as on computers, in the computers' window, with a Quick Settings tile. The app says when
  Android's Private DNS setting or another VPN app gets around it. F-Droid and Google Play come next.
- **Updates on macOS**, as on Windows: OffLok installs a new release's package itself, once its signature checks.
- **Unbound's and OpenSSL's security fixes reach the Mac and Android** as Unbound's reach Windows: a daily watch
  checks their signatures and proposes a new OffLok release.
- **offlok.org:** the Mac's download and how to open it, a "Donate" button (Liberapay), and a picture when a link to
  it is shared.

## 0.5.1

- **Windows: the window opens out of the tray icon**, as Windows's own flyouts do: no frame and no taskbar button,
  next to the icon, closed by a click elsewhere or by Escape. UAC's prompt for one of its settings leaves it open.
- **The app has its icon**: the window and the taskbar showed none on Windows.
- **The mark is full inside its ring**, a very dark teal behind the gold tree: in the app, the tray, the installer,
  the packages and the website.

## 0.5.0

The first release.

- **Your computer as its own DNS resolver.** Unbound, recursive from the root servers, listening on the loopback
  only: QNAME minimisation, DNSSEC with the root's trust anchors, serve-stale, no query log. Local names and
  private reverse zones still go to the network's DNS.
- **Never breaks your internet on purpose.** Every change is self-tested; a failure gives the system its original
  DNS back and OffLok tries again a minute later. Turning OffLok off or uninstalling it gives every connection its
  original DNS back.
- **When the root servers are out of reach** (a hotel's Wi-Fi, a network that blocks port 53): your
  DNS-over-TLS resolver if you set one, otherwise the network's DNS, shown in orange. Strict mode never falls back;
  "Sign in to this network" lets the network's DNS through for 5 minutes, for a captive portal.
- **Blocking, off at first:** ads, trackers, malicious sites and your own lists, one switch each; "Is this site
  blocked?", "Trust this site", a 10-minute pause.
- **A tray icon and a window**, in English and French, and the `offlok` command. Settings are for administrators
  unless an administrator opens them to everyone.
- **Linux:** `.deb` and `.rpm` packages for amd64 and arm64, with systemd-resolved or `/etc/resolv.conf`; the
  desktop app for amd64 (GTK 3, WebKitGTK 4.1).
- **Windows 10 (version 2004) and 11, 64-bit:** an installer with NLnet Labs' Unbound 1.26.2, a service, the DNS
  of every network interface, IPv4 and IPv6. Unbound takes a spare port when Windows reserved its usual one
  (Hyper-V, WSL, Docker).
- **Updates:** once a day OffLok looks for a new release on GitHub (it can be turned off). An update is installed
  only once its signature by OffLok's release key and every file's SHA-256 check out.
