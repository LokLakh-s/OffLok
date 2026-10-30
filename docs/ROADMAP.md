# OffLok — roadmap

The milestones of [PLAN.md](PLAN.md), ticked as they land.

## 1. The plan

- [x] Plan reviewed by the maintainer (2026-10-29), private repository `LokLakh-s/OffLok` created

## 2. Linux core (0.1.0)

- [x] Unbound configuration: loopback only, QNAME minimisation, DNSSEC with the root's trust anchors (KSK-2017 and
      KSK-2024, then RFC 5011), serve-stale, no query log; the local names and private reverse zones forwarded to
      the network's DNS; names from DHCP checked before they reach the file
- [x] Unbound runs as offlokd's child; a configuration change reloads it
- [x] System DNS: systemd-resolved (drop-in, global `~.`) and `/etc/resolv.conf` (file or symbolic link kept and
      put back)
- [x] Root probe (authoritative answers only, over the address families Unbound uses), self-test after every
      change, rollback, retry after a minute
- [x] Fallback: the personal DNS-over-TLS resolver, then the network's DNS; strict mode; back to the root by itself
- [x] Local channel with the caller's identity from the kernel; settings for administrators, opened to everyone by
      an administrator
- [x] `offlok` command in English and French
- [x] `.deb` and `.rpm` for amd64 and arm64; the distribution's untouched `unbound.service` turned off at install
      and back on at removal
- [x] Integration tests in disposable systemd containers (33 + 12 checks); CI
- [x] A real Ubuntu 24.04 desktop with AppArmor enforcing (2026-10-29, 0.3.0): Unbound confined by its profile
      and resolving, no denial, systemd-resolved's global DNS on OffLok
- [ ] Fedora with SELinux, NetworkManager without systemd-resolved, systemd 249 and 252

## 3. The app on Linux (0.2.0)

- [x] Mark (the DNS tree, teal), tray states (a dot), window: chosen and approved by the maintainer
- [x] Tray icon and window with Wails 3 (GTK 3, WebKitGTK 4.1), English and French; `offlok-desktop` package
      (session autostart, menu entry, icons)
- [x] Administrator's password through polkit (`offlok --polkit` too); error codes the window translates
- [x] Strict mode: "Sign in to this network" lets the network's DNS through for 5 minutes (`offlok signin`)
- [x] Tests: polkit and sign-in in the main scenario (39 checks), the desktop packages on a virtual screen (9)
- [x] On a real Ubuntu desktop (2026-10-29): the tray icon, the window, the password prompt,
      "Check" naming the AdGuard DNS filter, ads gone while browsing

## 4. Blocking, off by default (0.3.0)

- [x] OffLok's own front before Unbound (12 MB for 470,000 names, where Unbound's local zones took 190 MB)
- [x] Ads, trackers, malicious sites (public lists) and the user's own lists: one switch each; lists downloaded
      once a day, conditionally, kept on disk
- [x] "Is this site blocked?", "Trust this site", "Pause blocking" (10 minutes, open to everyone); the window's
      "Blocking" card, approved on screenshots
- [x] Tests: the real AdGuard DNS filter and a custom list in the main scenario (58 checks), the window with the
      real list on a virtual screen (10)

## 5. Windows (0.4.0)

- [x] offlokd as a Windows service (SYSTEM, started at boot, restarted after a crash); NLnet Labs' Unbound 1.26.2
      in a job object, so it never outlives offlokd
- [x] System DNS per interface, IPv4 and IPv6 apart (`SetInterfaceDnsSettings`), each interface's original saved
      and given back at uninstall, connected or not
- [x] Local channel: a named pipe closed to the network, the caller's token read by impersonation; UAC's prompt
      for the tray app's settings
- [x] NSIS installer built on Linux, English and French, silent mode; Unbound pinned and its SHA-256 checked
- [x] Tests on GitHub's `windows-2025` (Windows Server 2025): the Go tests (the named pipe included), then
      install, service, system DNS, DNSSEC, blocking, the tray app, uninstall and every interface's DNS as before
      (17 checks)
- [x] A real Windows PC (2026-10-30): installed, used, uninstalled, everything working
- [ ] Wi-Fi changes, a VPN and a captive portal on a real PC
- [x] Release key (Ed25519, `45e724ba`), its private half in the `release` environment, `v*` tags only
- [x] Signed updates from GitHub Releases: checked once a day (on at first, can be turned off), the signature and
      every file's hash checked before the system's installer runs (apt, dnf, zypper, rpm, or the Windows
      installer as SYSTEM); the tray app starts again from the new version; tests from one test release to the next
      in a container (14 checks) and on Windows
- [x] Release workflow on GitHub's machines: packages, installer, its Windows test, `SHA256SUMS` signed, the notes
      from `CHANGELOG.md`
- [x] Before going public: `C:\ProgramData\OffLok` locked to SYSTEM and the administrators, `offlok` in the
      PATH, the programs' name and version, the installer's page on the DNS it changes and on the update check,
      Windows's display language for the command and the tray menu
- [ ] Unbound's security fixes shipped by OffLok: a scheduled job that watches NLnet Labs' releases
- [x] Public under `LokLakh-s`, 0.5.0 the first release on GitHub Releases (2026-10-30); the GitHub name `OffLok`
      reserved
- [ ] SignPath Foundation: the application, ready in [SIGNPATH.md](SIGNPATH.md), sent by the maintainer
- [ ] arm64 (the GUID passed by value to iphlpapi is written for arm64's registers, untested)

## 6. macOS

- [ ] launchd daemon, DNS per network service, Homebrew tap, unsigned `.pkg`

## 7. Later

- [ ] Android (local VPN), iOS (Network Extension), Apple Developer account, signed `.pkg`
