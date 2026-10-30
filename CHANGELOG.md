# Changelog

What changes from one OffLok release to the next. Each release's notes on GitHub start with its section here.

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
