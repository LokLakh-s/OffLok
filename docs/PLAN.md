# OffLok — the plan

OffLok turns your computer into its own DNS resolver. Install it, and your machine looks names up by itself,
starting from the root servers, instead of asking your internet provider, Google, Cloudflare or Quad9. Free
software under the MIT licence, for Linux, Windows and macOS first, Android and iOS later.

Decisions of the maintainer, 2026-10-29. This file is the reference: when the code and this plan disagree, one of
them is wrong. Points marked **(to confirm)** are proposals that still wait for the maintainer's answer.

## What it changes, honestly

| | Your provider's DNS | A public resolver (Quad9, Cloudflare…) | OffLok |
|---|---|---|---|
| One third party sees every name you look up | yes | yes | **no one** |
| The answer can be rewritten or filtered upstream | yes | yes | no: DNSSEC is checked on your machine |
| Keeps working when that resolver is down | no | no | yes |
| Queries are encrypted on the wire | no | yes (DoT/DoH) | **no** |

- **No central log.** Each query goes to the servers in charge of that name: the root, then `.org`, then the
  domain's own servers. Each of them sees only its own part. With QNAME minimisation (RFC 9156), the root only
  learns that you asked something under `.org`.
- **Not encrypted.** Those queries travel in plain text, so your provider or the Wi-Fi operator can still read them
  as they pass, even though nobody keeps them in one place. The domain's own servers see your IP address.
- **Not a VPN.** OffLok hides nothing about the sites you connect to: your provider still sees the IP addresses, and
  usually the site names in the TLS handshake.
- **Slower first lookup, faster after.** A name nobody on the machine has asked for yet takes a few round trips.
  After that it comes from the local cache, and prefetching keeps popular names fresh.

## The rules

- **Never break the user's internet.** Every change to the system DNS is followed by a self-test. If the test
  fails, the original settings come back automatically. Uninstalling restores the settings exactly as they were
  before the install.
- **Loopback only.** OffLok's front listens on `127.0.0.1:53` and `[::1]:53`, Unbound on `127.0.0.1:53530` (or a
  spare port, see Windows), and nowhere else. OffLok must never become an open resolver on a café's Wi-Fi.
- **No third-party resolver by default.** It recurses from the root servers. A fallback exists (below), and the
  user chooses it.
- **Nothing phones home.** No telemetry, no analytics, no account. The only connections OffLok makes by itself are:
  - DNS recursion;
  - blocklist downloads, only when blocking is on;
  - update checks (see *Updates*).
- **Nothing is logged.** Query logging stays off in Unbound, and OffLok never writes a looked-up name to disk.
- **We never write our own resolver.** Recursion and DNSSEC are security code, so the engine is Unbound. OffLok's
  front only blocks names and hands everything else to Unbound untouched.

## Decisions (2026-10-29)

| Topic | Decision |
|---|---|
| Name | **OffLok**, chosen over UnbLok, LokOut and LokUp. On 2026-10-29, every `offlok` domain checked (.com, .org, .net, .app, .dev, .fr) and the `offlok` GitHub account were free |
| Repository | **`LokLakh-s/OffLok`**, MIT, copyright "LokLakh". **Private during development** (2026-10-29), public afterwards |
| Language | Code, commits, README, docs and issues in **English**. The app shows **English or French**, following the system language |
| Engine | **Unbound** (NLnet Labs, BSD-3-Clause). Hickory DNS, the only fully MIT-compatible candidate, still labels its recursor experimental (0.26.1) |
| Our code | **Go**: one static binary per OS, cross-compiled, running as a system service |
| Interface | A **tray icon** (green when it resolves on its own, orange in fallback, grey when off or paused, red on error) and a **small window**. The resolver itself runs as a system service, even with nobody logged in |
| Command line | **Yes** (2026-10-29): `offlok status`, `on`, `off`, `strict`, `dot`, `access`, on the same local channel as the tray app |
| Who may change settings | **Pausing is open to everyone; every other setting needs an administrator by default, and an administrator can open settings to everyone** (2026-10-29). Seeing the status is always open |
| Blocking | Ads, trackers and malicious sites: **included, off by default**. Each category has its own switch, plus "Pause blocking" and "Trust this site" |
| How it blocks | **OffLok's own front** (2026-10-29), rather than Unbound's local zones: measured with the lists below (about 470,000 names) on Unbound 1.19, local zones took 190 MB more and 1.8 s per change, cache emptied each time, and RPZ 560 MB and 10 s. The front holds the same names in 12 MB and looks a name up in all lists in 4 µs; pausing and trusting are instant |
| Lists | **A selection of public lists** (2026-10-29, licences checked that day): ads = AdGuard DNS filter (GPL-3.0); trackers = ShadowWhisperer Tracking (Unlicense) and HaGeZi's native trackers for Windows, Apple, Samsung, Xiaomi, Huawei (GPL-3.0); malicious = URLhaus (CC0/MIT), Phishing URL Blocklist (CC BY-SA 4.0), Scam Blocklist (MIT), HaGeZi's Threat Intelligence Feeds mini (GPL-3.0). **Plus the user's own lists**, by their address (AdBlock, hosts or one domain per line), category "your lists" |
| Root servers unreachable | By default, **fall back to the network's DNS and turn the icon orange**. A **"Strict mode"** setting turns this fallback off; signing in to a captive portal stays possible in strict mode. A **personal DoT address** can be set, used before the network's DNS |
| macOS | **No Apple Developer account at first**: Homebrew (built from source) plus an unsigned `.pkg` with instructions. The account ($99 a year) comes once the project takes off, and the same account will serve iOS |
| Mark | **The DNS tree** (2026-10-29, among a power button and a magnifying glass on a globe): an open ring in OffLok's **teal** (among azure, indigo and violet), round a gold tree — the root on top, two branches, four leaves, in `assets/brand/`. **Full inside the ring** (2026-10-30): a very dark teal disk |
| Tray states | **A dot** at the bottom right (2026-10-29, rather than a ring changing colour): orange in fallback, red on error, none when all is well; the whole icon grey when off |
| Window | The design reviewed on screenshots and approved (2026-10-29): the state on top (chip, title, one sentence), "Sign in to this network" when strict mode blocks, the on/off switch, then the settings (strict mode, the encrypted fallback resolver, who may change settings). **On Windows, a flyout out of the tray icon** (2026-10-30): no frame, no taskbar button, closed when it loses the focus |
| Captive portals in strict mode | **"Sign in to this network"** (2026-10-29): the network's DNS for 5 minutes, then strict mode again by itself. **Open to every user**, as pausing (confirmed by the maintainer, 2026-10-29) |
| Order | **Linux → Windows → macOS**, then mobile |
| Windows CI | **GitHub's Windows runners** (2026-10-29), on pushes to main only, while the repository is private (its minutes count double); real trials on a Windows PC |
| Update source | **GitHub Releases** (2026-10-29): the app checks once a day and installs a new version only after checking OffLok's signature. It contacts GitHub (Microsoft) once a day |
| Version | OffLok's own semantic versioning, starting at 0.1.0 (it is not part of the shared Lok version counter) |
| Android | **Google Play, F-Droid and the APK on GitHub** (2026-10-31). The maintainer's Play developer account (`tor@offlok.org`) is going through identity verification. **Google Play without ad blocking, F-Droid and GitHub full** (2026-10-31) |
| Donations | **Liberapay only** (2026-10-31), account `LokLakh`: a "Donate" button on offlok.org, our own, not Liberapay's script |

## Architecture

```
 apps ─► system stub resolver ─► 127.0.0.1:53 / [::1]:53: OffLok's front (offlokd)
                                        │   blocked names: 0.0.0.0 / :: at once
                                        ▼
                                   127.0.0.1:53530: Unbound  (recursion, DNSSEC, cache)
                                        │
             ┌──────────────────────────┼────────────────────────────┐
         normal: root → TLD →     fallback: personal DoT,     local names and VPN domains:
         the domain's servers     then the network's DNS      the network's (or VPN's) DNS
                                  (never in strict mode)

 offlokd (service, root / SYSTEM): writes Unbound's config, runs it, sets the system DNS,
         watches networks, probes, falls back, self-tests, rolls back, restores on uninstall
 offlok  (tray + window, user session) ──── local IPC ────► offlokd
```

### Unbound

offlokd generates the configuration from a template; nobody edits it by hand. The template:

- listens on `127.0.0.1:53530` only, where the front alone asks it, and lets only localhost query it;
- turns on QNAME minimisation, the `harden-*` options and protection against DNS rebinding (`private-address`), with
  exceptions for the local domains;
- keeps the root trust anchor in a writable `auto-trust-anchor-file`, so that Unbound follows key rollovers itself
  (RFC 5011);
- turns on `prefetch`, serve-stale (RFC 8767) and aggressive NSEC (RFC 8198);
- writes no query log.

**Proposed: a local copy of the root zone** (RFC 8806, Unbound's `auth-zone` for `.`), with a fallback to the root
servers. TLD lookups then need no root round trip, and resolution keeps going if the root servers are unreachable
for a while.

On Linux, OffLok runs its own Unbound unit and never touches the distribution's `unbound.service`. A user who
already runs their own Unbound keeps it.

### offlokd

- **System DNS.** It saves the original settings, then points every interface, IPv4 *and* IPv6, at the loopback
  resolver. An IPv6 router advertisement (RDNSS) from the box must not add the provider's resolver next to ours.
- **Networks.** On every network change, it records the DNS servers and search domains the network or VPN provides,
  then re-probes.
- **Probe.** It checks that the root servers answer over UDP and TCP port 53.
  - Yes: **direct** (green).
  - No: **fallback** (orange). It uses the personal DoT address if one is set and reachable, otherwise the network's
    DNS, unless strict mode is on. In fallback it retries the root every few minutes, and at once after a network
    change.
- **Local names** go to the network's DNS:
  - the network's search domains;
  - `home.arpa`, `.lan`, `.home`, `.internal` and similar;
  - reverse zones of private address ranges.

  `.local` belongs to mDNS: OffLok leaves it to the OS.
- **VPNs.** Domains a VPN declares go to the VPN's DNS (details per OS: *To verify*).
- **Self-test** after every change:
  - a name must resolve through `127.0.0.1`;
  - a validly signed name must come back DNSSEC-validated;
  - a name with deliberately broken DNSSEC must fail.

  If any of these goes wrong: restore the saved settings, go red, and tell the user.
- **Off.** "Turn OffLok off" restores the original settings until the user turns it back on. "Pause blocking"
  lasts 10 minutes.

### Captive portals

- **In the default mode**, the network's DNS is used during fallback, so the portal page loads as usual.
- **In strict mode** (decided 2026-10-29): whenever strict mode blocks public names, the window offers **"Sign in
  to this network"** (`offlok signin`). This allows the network's DNS for 5 minutes, to every user, then strict
  mode applies again by itself. Reading the OS's own captive-portal signals, to offer it only then, is still to
  verify.

### Blocking (off by default)

- **The front.** offlokd answers the system on the loopback. A blocked name gets `0.0.0.0` for A, `::` for AAAA
  and no data for other types, with a 10-second TTL; any other query goes to Unbound untouched, DNSSEC bits
  included, over UDP or TCP as it came. Unbound down: SERVFAIL.
- **Categories.** Ads, trackers, malicious sites and the user's own lists each have their own switch (`offlok
  block <category> on|off`, administrators).
- **Lists.** offlokd downloads them over HTTPS once a day, only for the categories switched on, asking the server
  whether they changed first (`If-None-Match`, `If-Modified-Since`). It keeps them on disk, normalized
  (`/var/lib/offlok/lists/`), so that blocking works from the start, network or not; a failed download keeps the
  previous copy. They are never shipped in the package. In memory, each list is a sorted, packed set of names
  (about 28 bytes a name). A listed name blocks itself and everything under it; a list's exception (`@@`) lets a
  name through whatever list blocked it, as AdGuard does. Rules OffLok does not understand (regular expressions,
  modifiers that narrow a rule) are skipped.
- **"Is this site blocked?"** (`offlok check`, open to everyone): the user types or pastes the site that
  misbehaves; OffLok says which list blocks it.
- **"Trust this site"** (`offlok trust`, administrators): the site and what is under it are never blocked; its
  parent stays blocked.
- **"Pause blocking"** (`offlok pause`, open to everyone) lasts 10 minutes. Each change empties systemd-resolved's
  cache, so that it shows at once.

### The app (tray and window)

- Status (direct, fallback, off, error), the on/off switch, strict mode, the personal DoT address, the blocking
  switches, "Trust this site" and the pause.
- English and French, chosen from the OS locale. Translation files are kept separate, so contributors can add
  languages.
- GUI toolkit: **Wails 3.0.0** (MIT, released 2026-10-26): the tray and a window whose face is a web page
  (`desktop/frontend/`, plain HTML, CSS and JavaScript, no bundler), in one program on Linux, Windows and macOS. The
  page can follow the family's look closely, and screenshots of it come without a desktop (`?preview=<state>`).
  On Linux, the `gtk3` build tag: GTK 3 and WebKitGTK 4.1, there from Ubuntu 22.04 and Debian 12 (Wails's default,
  GTK 4 and WebKitGTK 6.0, would leave Ubuntu 22.04 and Mint 21 out). Fyne was the fallback.
- **Its own Go module** (`desktop/`): it links GTK and WebKit with cgo, so the service and the command keep building
  anywhere without them. The window calls `main.App.Status` and `main.App.Send` through Wails's runtime; `App` asks
  offlokd over the local socket, with `interactive` set, so that offlokd can have polkit ask for the password.
- **Its own package** (`offlok-desktop`, amd64 for now): servers keep the small `offlok`. It starts with the session
  in the tray (`/etc/xdg/autostart`, `--hidden`), and from the applications menu with its window; a second launch
  shows the running one's window.
- **GNOME has no tray by default.** Ubuntu ships the AppIndicator extension, and Fedora does not. Without it, the
  window opens from the app launcher.
- **The command line** `offlok` uses the same channel. It is what the integration tests drive.
- **IPC:** a Unix socket (Linux, macOS) or a named pipe (Windows), open to every local user. offlokd reads who is
  calling from the kernel (`SO_PEERCRED`), which the caller cannot forge. Administrator means root for now (`sudo`).
  The tray app sends its requests as interactive, and offlokd then asks polkit (`org.loklakh.offlok.manage`,
  `auth_admin_keep`: the password once, then a few minutes without asking), whose agent in the user's session
  shows the prompt. `offlok --polkit` does the same from a terminal in a desktop session. Opening or closing
  settings to everyone is always an administrator's decision.

## Per platform

### Linux (first)

- **Packages:** `.deb` and `.rpm` (nfpm), depending on the distribution's `unbound` package, which keeps Unbound's
  security fixes in the distribution's hands. AUR later.
- **The distribution's `unbound.service`:** Debian and Ubuntu start it as soon as `unbound` is installed, and it
  takes `127.0.0.1:53`. When its configuration is the package's own, untouched, the install script turns it off
  and the removal script turns it back on. A configured one is someone's resolver: left alone, and offlokd reports
  that the port is taken.
- **System DNS with systemd-resolved** (Ubuntu, Fedora): a drop-in, `/etc/systemd/resolved.conf.d/90-offlok.conf`,
  sets the global DNS to the loopback with the `~.` routing domain. resolved then sends every public name to
  OffLok only, and a link's own servers only get that link's domains (checked on systemd 255 with a packet
  capture, an integration test). resolved 255 cannot reload its configuration (`CanReload=no`): offlokd restarts
  it, and network managers hand it their links' DNS again. Giving the DNS back restarts it **without waiting**:
  while offlokd's unit is stopping, systemd orders that restart after the stop, so waiting would deadlock. The
  links' DNS and domains are read over D-Bus, for the fallback and the local names.
- **System DNS without it:** offlokd writes `/etc/resolv.conf` itself and keeps the original (a file, or the
  symbolic link it was). When something else rewrites it, a DHCP client or NetworkManager, that new content becomes
  the original, and OffLok's file comes back within 5 seconds.
- **Unbound's files:** `/var/lib/unbound/offlok/`, because Ubuntu's AppArmor profile for Unbound only lets it read
  `/etc/unbound/` and `/var/lib/unbound/`. The SELinux labels of `/var/lib/unbound` fit too. The configuration
  stays root's; only `keys/`, where Unbound rewrites the trust anchor on key rollovers, belongs to `unbound`.
- **Service:** one systemd unit, `offlokd.service`. Unbound runs as offlokd's child and dies with it. Whatever way
  offlokd stops, a crash included, `ExecStopPost=offlokd restore` gives the original DNS back before any restart.
- **IPv6:** used when the machine has a route to the IPv6 internet, a unique local address behind NAT66 included.
  The root probe and Unbound always use the same address families: otherwise the probe can see the root over IPv6
  while Unbound, limited to IPv4, cannot (found in the tests, Docker gives exactly that).
- **Tests:** unit tests, then `test/integration/run.sh` in a disposable Ubuntu container with systemd as PID 1
  (33 checks: direct, leaks, rights, fallback, strict mode, return of the root, off and on, crash, stop, the
  `resolv.conf` way) and `run.sh package` (12 checks on the `.deb`'s scripts).

### Windows

- **Installer: NSIS** (zlib licence), built on Linux like everything else (`packaging/windows/build.sh`, in
  `packaging/linux/desktop-build`'s image): Go cross-compiles the three programs, and Wails needs no cgo on
  Windows. It embeds NLnet Labs' **Unbound** (`unbound.exe` and its licence only: a 64-bit executable with OpenSSL
  inside, importing nothing but Windows's own DLLs), pinned in `packaging/windows/unbound.env` with its SHA-256
  checked at every build (1.26.2 in OffLok 0.5.0). English or French, following Windows's language. It needs
  64-bit Windows 10 version 2004 (build 19041) or later, for the API below. Files in `C:\Program Files\OffLok`,
  OffLok's own in `C:\ProgramData\OffLok` (settings, lists, saved DNS, `offlokd.log`, capped at 1 MB).
- **System DNS:** `SetInterfaceDnsSettings` (iphlpapi), per interface, IPv4 and IPv6 apart: `127.0.0.1` and `::1`
  on every interface that is up, a new one (Wi-Fi switch, VPN) set as soon as it shows. Each interface's original
  is saved first: empty means automatic (DHCP, router advertisements), which an empty setting gives back. The
  uninstall restores every saved interface, connected or not. The network's DNS, for the fallback, comes from the
  registry (`DhcpNameServer`, `Dhcpv6DNSServers`) and from the static servers saved.
- **Service:** `OffLok`, started at boot, run as SYSTEM (`golang.org/x/sys/windows/svc`), restarted 2 seconds
  after a crash by the service manager. Unbound runs in a job object that kills it when offlokd goes. Windows has
  no SIGHUP: a new configuration restarts Unbound.
- **Local channel:** the named pipe `\\.\pipe\offlok`, open to signed-in users, closed to the network. offlokd
  reads the caller's token by impersonating it: an administrator is an elevated member of Administrators. The
  tray app, refused, runs the command again through `offlok.exe --raw` elevated, so Windows shows UAC's prompt
  (polkit's place on Linux).
- **OffLok's folder in ProgramData is offlokd's alone.** ProgramData lets every user create folders: a user who
  made `C:\ProgramData\OffLok` (or something in it) first could change what offlokd, as SYSTEM, reads and
  writes there, Unbound's configuration included. At install and at every start, offlokd removes links and
  anything not owned by SYSTEM or the administrators, then sets the folder's permissions: protected, SYSTEM and
  administrators may write, users only read.
- **The command in the PATH:** `offlokd install` adds `C:\Program Files\OffLok` to the machine's PATH,
  `uninstall` takes it out; no other entry changes.
- **Each program carries the product's name and version** (go-winres, at build time), which SignPath requires,
  and the mark as its icon. The window's program also has a manifest for high-DPI screens.
- **Unbound's port:** Windows reserves port ranges for Hyper-V, WSL and Docker at boot, by default anywhere
  between 49152 and 65535, where 53530 is; binding a reserved port fails with "access forbidden". When Unbound
  starts, offlokd takes the first free of 53530, 10530, 20530, 30530 and 40530 (spread below 49152, so one reserved
  range cannot hold them all). The Windows test has another program hold 53530 first. (A range reserved by hand
  with `netsh int ipv4 add excludedportrange` only keeps Windows from handing the port out; a program can still
  take it.)
- **Language:** the command and the tray menu follow Windows's display language (no `LANG` there); the window
  follows WebView2's.
- **Signing:**
  - First releases are unsigned, so SmartScreen warns.
  - Then the **SignPath Foundation**, free for open source, with the certificate in SignPath Foundation's name.
    The maintainer agreed to apply (2026-10-30). The application is a form on signpath.org/apply, which the
    maintainer sends (their name, e-mail and agreement to SignPath's terms); the answers are ready in
    [SIGNPATH.md](SIGNPATH.md). It asks for the project's reputation (users, downloads, mentions): a project with
    none yet may be asked to come back later.
  - What SignPath asks (its terms and documentation, read on 2026-10-30), and where OffLok stands:
    - OSI licence, no proprietary component: yes (MIT; Unbound is BSD-3-Clause).
    - **Already released, in the form to sign:** not yet, nothing is published.
    - **Source in a public repository, builds verifiable from it:** not yet, the repository is private.
    - **Every job leading to the signing request runs on GitHub-hosted runners:** the release workflow must build
      on GitHub's machines, not on a self-hosted one.
    - A "Code signing policy" section on the home page and the download page, with the sentence "Free code
      signing provided by SignPath.io, certificate by SignPath Foundation", the team's roles (authors, reviewers,
      approvers) and a privacy statement.
    - **Privacy:** sending data to a server the user did not choose must be written in a privacy policy, shown
      during installation, and possible to turn off. That covers the update check (GitHub). The root servers and
      the lists the user switches on are what the user asked for.
    - **No system change without a proper warning:** the installer must say, before installing, that every
      interface's DNS will point at OffLok, and that uninstalling gives it back.
    - Product name and version in every signed file (`offlok.exe`, `offlokd.exe`, `offlok-desktop.exe` carry
      none yet), each release approved by hand in SignPath, multi-factor authentication for the whole team.
- **Tests:** a `windows-2025` machine of GitHub's in CI (`test/windows/test.ps1`: silent install, service, system
  DNS, DNSSEC, blocking, the tray app starting, silent uninstall, every interface's DNS as before), the Go tests on
  Windows (the named pipe included), then a real PC for Wi-Fi, VPNs and captive portals. arm64 builds
  are for later: the GUID passed by value to iphlpapi is written for arm64's registers, untested.

### macOS

- **Unsigned `.pkg` first** (0.6.0; the maintainer chose ".pkg, then Homebrew", 2026-10-30, and to ship it unsigned,
  2026-10-31): OffLok.app in the menu bar (LSUIElement, started at every login by a LaunchAgent), offlokd under
  launchd (`org.loklakh.offlokd`), the `offlok` command, and **Unbound built by us** from NLnet Labs' source with
  OpenSSL 3.5 linked in, one program for Apple silicon and Intel, macOS 13 or later (Go 1.27's own minimum). Both
  sources are pinned with their SHA-256 after their signatures are checked (`packaging/macos/sources.env`), and the
  daily watch proposes their security fixes. Unbound drops to a hidden `_offlok` account. The site gives the steps
  to allow the package in System Settings › Privacy & Security.
- **Its icon keeps a tile** (the brand charter: the Dock expects one, and macOS 26 shuts a round icon in a grey
  square): very dark teal, the mark at 72 %, Apple's grid (chosen on a board, 2026-10-31).
- **Updates:** as on Windows, offlokd downloads the new release's `.pkg`, checks it against the signed
  `SHA256SUMS`, and runs macOS's installer; the package restarts offlokd and the menu bar app.
- **Homebrew tap, later:** `loklakh-s/tap`, a formula built from source, with Homebrew's Unbound; then
  `sudo offlok install` sets up the launchd daemon. Installed by Homebrew, OffLok leaves its updates to Homebrew.
- **System DNS:** per network service with `networksetup`; the network's own DNS read in `State:` through
  `scutil`; caches flushed (`dscacheutil`, mDNSResponder). A VPN's split DNS (its match domains) is left to macOS,
  which sends those domains to the VPN's servers by itself: no `/etc/resolver` file is needed.
- **Tests:** GitHub's macOS 26 runners install the package, check launchd, every network service's DNS, DNSSEC,
  blocking, a crash, the fallback and strict mode behind pf, an update from one test release to the next, and the
  uninstall; then a real Mac for Wi-Fi, VPNs and captive portals.

### Android (2026-10-31: Google Play, F-Droid and the APK on GitHub)

No Android app may set the phone's DNS: the "Private DNS" setting takes only a remote DNS-over-TLS hostname. An app
that answers DNS for the whole phone does it through a **local VPN** (`VpnService`), as RethinkDNS and Intra do.

```
 apps ─► Android's resolver ─► 10.111.222.1 (OffLok's address, the only route into the tunnel)
                                   │  the tunnel's packets: UDP queries, IPv4 and IPv6
                                   ▼
          OffLok's Go core, in the app: the front (blocking) ─► libunbound (recursion, DNSSEC, cache)
                                                                    │  plain sockets, outside the tunnel
                                                                    ▼
                                         root → TLD → the domain's servers; fallback: personal DoT, network DNS
```

- **DNS only in the tunnel.** The tunnel's DNS server is one made-up address (`10.111.222.1`, and an `fd` address
  when the network has IPv6), and that address is its only route: browsing and every other connection go on as
  usual, neither slowed down nor seen by OffLok. OffLok leaves itself out of its tunnel.
- **The Go core reads the tunnel.** It takes the UDP queries sent to that address, answers them through the front
  (blocking, pause, trusted sites) and Unbound, and writes the answers back. The tunnel's MTU is large enough for
  any UDP answer, so answers are never truncated. TCP to that address (Android's DNS over TCP, and its "Automatic"
  Private DNS probing port 853) gets a reset at once, so that Android falls back to plain DNS without waiting.
- **Unbound runs inside the app, as libunbound**, not as the `unbound` program: since Android 10 an app may not run
  programs from the folders it can write to, and since Android 12 the system may kill an app's child processes.
  libunbound is the same resolver (the iterator and the validator) and reads the same configuration syntax; its
  answers come back as DNS packets, which the front passes on. Unbound and OpenSSL are the macOS build's versions,
  pinned and signature-checked the same way, built with the NDK.
- **The same core as on computers.** `internal/daemon` gains an Android backend: the system DNS is the tunnel (on =
  establish it, off = close it), and the network's DNS comes from Android's `ConnectivityManager` for the fallback,
  captive portals and local names. Unbound becomes an engine behind one interface: the program on computers,
  libunbound on Android. The core is built with `gomobile bind` into an `.aar` the Kotlin app calls.
- **The app.** A foreground `VpnService`; its notification shows the state as the tray icon does. The window is
  **the computers' window** (`desktop/frontend`) in a WebView: `api.js` talks to the Go core through a JavaScript
  bridge instead of Wails, so one window serves every platform, in English and French. A Quick Settings tile turns
  OffLok on and off. Android's "Always-on VPN" setting starts it at boot.
- **What only a phone needs.**
  - Android asks once for the VPN permission, the first time OffLok is turned on.
  - A Private DNS hostname set in Android's settings sends every query to that server, around any VPN: the window
    says so, with a button to the setting.
  - Android runs one VPN at a time: turning another VPN app on turns OffLok off, and the window says why.
- **Left out on Android:** settings access levels (one user), the command line, and the updater: Google Play and
  F-Droid update the app themselves, and the GitHub APK is updated with F-Droid, IzzyOnDroid or Obtainium.
- **Requirements:** Android 9 or later (API 28, where the Private DNS state can be read), on arm64-v8a,
  armeabi-v7a and x86_64.
- **Package and versions:** `org.loklakh.offlok`. Its version is OffLok's `VERSION`, released with the computers'
  versions; `versionCode` = major × 10,000 + minor × 100 + patch.
- **One signing key everywhere.** A release key (RSA 4096) signs the GitHub APK. F-Droid publishes that same APK
  when its own build matches it byte for byte (a reproducible build: `Binaries` and `AllowedAPKSigningKeys` in its
  metadata). The key is also given to Play App Signing ("provide a copy of your app signing key"), so the app
  updates from any of the three sources. The key lives in a GitHub secret, with a copy the maintainer keeps offline.
- **Google's developer verification** (certified devices, France in 2027): the maintainer's Play Console account
  registers `org.loklakh.offlok` and the release key, which covers the APK of GitHub and F-Droid too. Users then
  install it as today, without the "advanced flow".
- **Distribution.**
  - **GitHub Releases:** a universal APK, in the signed `SHA256SUMS`.
  - **F-Droid:** store texts and screenshots in the repository (`fastlane/metadata/android/en-US` and `fr-FR`), a
    merge request to `fdroiddata` with the build recipe (NDK, Go, OpenSSL and Unbound built from source). Its
    volunteers review it with no announced delay; the app appears 24 to 48 hours after the merge. IzzyOnDroid,
    which takes the APK from GitHub, in the meantime.
  - **Google Play:** an `.aab`, the VpnService declaration with an in-app disclosure, the Data safety form ("no
    data collected"), a privacy policy on offlok.org. A personal account created after 13 November 2023 first runs
    a closed test with 12 testers for 14 days. Play's VpnService policy lists no DNS use, and says that "any
    manipulation of ads that can impact monetization will result in rejection": **the Play version has no ad
    blocking** (decided 2026-10-31): no "Ads" switch, no ad list. Trackers, malicious sites and the user's own lists
    stay, unless Google's review refuses them. **F-Droid and GitHub ship the full app.** Two build flavours of the
    same code, `full` and `play`.
  - **offlok.org:** an Android card with the three sources.
- **Tests:** the Go tests; then an Android emulator on GitHub's Linux runners: install, grant the VPN through
  `appops`, resolve through the tunnel, DNSSEC, blocking, the root cut off, strict mode, off and the phone's DNS as
  before. Then real phones.
- **Limits, said in the app and on the site:** one VPN at a time; a Private DNS hostname bypasses OffLok; a resolver
  running all day costs some battery (to measure).

### iOS (later)

- A Network Extension (packet tunnel) and the Apple Developer account. Memory and permission limits: *To verify*.

## Updates and security

- **Unbound updates on Windows and macOS.** The `.pkg` and the Windows installer ship Unbound, so **OffLok must
  ship Unbound's security fixes**. A scheduled job (`.github/workflows/unbound.yml`) watches NLnet Labs' releases
  every day and prepares a new OffLok release; **the maintainer approves each one** (decided 2026-10-30):
  - a newer `unbound-<version>.zip` (64-bit, no release candidate) is taken only with a good OpenPGP signature by
    NLnet Labs' release key G2 (`2310 1869 0C4D 903E F419 146A A144 323D EAAC DF45`, in
    `packaging/windows/nlnetlabs-releases-g2.asc`, the same on nlnetlabs.nl and keys.openpgp.org) and the SHA-256
    NLnet Labs published; the source tarball the macOS package builds from, the same way;
  - **OpenSSL**, linked into the macOS package's Unbound, likewise (2026-10-31): a newer release of the pinned
    series (3.5, long-term support until 2030-04-08), signed by OpenSSL's key (`B146 647E 45A7 B339 47AB 226B 2A2C
    87D1 6169 2D40`, `packaging/macos/openssl-release.asc`), with the CVEs its notes name
    (`packaging/macos/openssl-update.sh`);
  - `packaging/windows/unbound-update.sh` pins it, adds a patch release to `CHANGELOG.md` with the CVEs NLnet Labs'
    advisories say it fixes (or adds it to the version in progress, when VERSION is not released yet), and the
    job opens a pull request and runs the whole CI and the macOS workflow on its branch; a newer one closes an older
    one's request.
    The repository must let GitHub Actions open pull requests (Settings, Actions, General); when it cannot, an
    issue says so, with the link that opens it;
  - merging it is the approval: once the CI passes on main, the job makes the tag and runs the release workflow
    (what the workflow's own token does starts no workflow, a dispatch excepted). A version in progress ships the
    new Unbound with its own release;
  - GitHub turns a public repository's schedules off after 60 days without activity: the Actions tab's
    "Enable workflow" turns it back on.
- **The app checks for updates** and offers them. Before installing one, it verifies the signature. The check is
  **on by default and can be turned off** (decided 2026-10-30): once a day it asks GitHub, which then sees the
  computer's address and that it runs OffLok. The Windows installer says so next to a ticked box, the window has
  a switch, and so does the command (`offlok updates on|off`).
- **Going public** (decided 2026-10-30): once everything SignPath asks for was ready (signed updates, the release
  workflow on GitHub's runners, the code signing policy, the installer's warnings), the repository went public
  under `LokLakh-s` with release 0.5.0 (2026-10-30). The SignPath application follows, sent by the maintainer.
- **Update source:** GitHub Releases (decided 2026-10-29).
- **Release files:** `SHA256SUMS` and a build provenance attestation. GitHub Actions are pinned by SHA, Dependabot
  is on, and Go builds use `-trimpath`.
- **Release key** (created 2026-10-30, with the maintainer's agreement): Ed25519, key ID `45e724ba` (the first
  8 hex digits of the SHA-256 of its 32 public bytes). The public key is
  [`packaging/release/offlok-release-45e724ba.pub.pem`](../packaging/release/offlok-release-45e724ba.pub.pem),
  and the app carries it. The private key is the `OFFLOK_RELEASE_KEY` secret of the repository's `release`
  environment, which only `v*` tags reach, and a copy is kept off GitHub (backed up,
  encrypted). Each release signs its `SHA256SUMS` into `SHA256SUMS.sig` (64 raw bytes). The app checks the
  signature, then the file's hash, before installing anything. Anyone can check by hand with OpenSSL 3:
  `openssl pkeyutl -verify -pubin -inkey offlok-release-45e724ba.pub.pem -rawin -in SHA256SUMS -sigfile
  SHA256SUMS.sig`, then `sha256sum -c SHA256SUMS`. The app accepts a list of keys, so a new key can be added in one
  release and the old one dropped in a later one.
- **How an update installs** (`internal/update`, `internal/daemon/updates.go`): offlokd reads the latest release's
  `SHA256SUMS` and `SHA256SUMS.sig` at `releases/latest/download/` (no GitHub API, so no rate limit), checks the
  signature, and takes the version from the signed file names, never from GitHub's page: an older release served
  again is no update. It downloads only what this system needs and checks each file's hash before anything runs.
  - **Linux:** the packages installed (`offlok`, and `offlok-desktop` when it is there, `.deb` or `.rpm`, this
    machine's architecture), handed to apt, dnf, zypper or rpm in a transient systemd unit (`systemd-run`): the
    package restarts `offlokd.service`, which must not stop the package manager halfway. offlokd waits for the
    unit, so a failure shows in the status.
  - **Windows:** the installer, run silently by the service (SYSTEM). It stops the service to replace it; the
    running tray app and command are renamed `.old` (Windows lets a running program be renamed, not replaced),
    and the tray app, seeing the new version and its file replaced, starts again from the new file.
  - Test builds (`-tags offlok_testkey`, never released) also trust `test/release/`'s key, which is public, and
    read their releases from `$OFFLOK_UPDATE_BASE`: the update tests (`test/integration/update.sh`,
    `test/windows/update.ps1`) serve made-up releases. Released builds do neither.
- **Releases** (`.github/workflows/release.yml`): a tag `v<VERSION>` builds the packages and the installer on
  GitHub's machines, tests the installer on Windows, then publishes them with `SHA256SUMS`, its signature, the
  public key and, once the repository is public, a build provenance attestation. The notes are the version's
  section of `CHANGELOG.md` (`packaging/release/notes.sh`). Run by hand on a branch, the workflow is a rehearsal
  that publishes nothing.

## Repository

```
cmd/offlokd/          the service
cmd/offlok/           the command line
desktop/              the tray app and window (its own Go module: Wails, cgo), frontend/ the window's page
internal/unbound/     config template, process control
internal/sysdns/      linux/ windows/ darwin/: read, set, save, restore the system DNS
internal/netinfo/     interfaces: loopback addresses, IPv6 route, network change fingerprint
internal/probe/       root reachability, self-test
internal/root/        the root servers' addresses and the root zone's trust anchors
internal/blocklist/   download, convert, allowlist (milestone 4)
internal/ipc/         socket / named pipe, permissions
internal/i18n/        en, fr
internal/daemon/      the decisions: probe, fall back, self-test, roll back; the local channel's commands
internal/settings/    the user's choices on disk
internal/polkit/      asks polkit for an administrator's action (Linux)
assets/brand/         OffLok's marks; scripts/brand-icons.py draws the icons from them
internal/update/      releases: the signed SHA256SUMS, the version in it, downloads checked against it
packaging/            linux/ (nfpm, systemd unit, install scripts), windows/ (NSIS), release/ (the public key,
                      the notes); pkg, homebrew later
test/integration/     disposable systemd containers: run.sh, scenario.sh, package.sh, desktop.sh, update.sh
test/windows/         the installer's test and the update's, on GitHub's Windows machines
test/release/         the update tests' key (public, trusted by test builds alone) and its signing tool
docs/                 this plan, ROADMAP.md; CHANGELOG.md and the README's code signing policy at the root
```

- **Identity:** commits use `LokLakh <contact@loklakh.fr>`.
- **CI:** GitHub-hosted runners on Linux, Windows and macOS. They run the unit tests, the integration
  tests in containers, and an install → resolve → update → uninstall → original-settings-restored test on each
  platform.

## Milestones

1. **This plan**, reviewed by the maintainer, then the repository is created.
2. **Linux core:** offlokd, the Unbound template, resolved and NetworkManager plumbing, probe, self-test,
   rollback, clean uninstall, `.deb`/`.rpm`, CI.
3. **The app on Linux:** tray, window, English and French, fallback, strict mode, personal DoT.
4. **Blocking**, off by default.
5. **Windows:** service, DNS plumbing, an installer with Unbound, runner tests, then real machines. SignPath
   application, after asking the maintainer.
6. **macOS:** launchd, DNS plumbing, an unsigned `.pkg` (0.6.0), then a Homebrew tap.
7. **Android:** the Go core with libunbound in a local VPN, the computers' window in a WebView, emulator tests,
   then GitHub, F-Droid and Google Play.
8. **Later:** iOS, the Apple Developer account and a signed `.pkg`.

## To verify while building (not verified yet)

- ~~**systemd-resolved:** does a global `~.` route keep every public query on the loopback?~~ Yes on systemd 255
  (integration test). Older versions (Ubuntu 22.04: 249, Debian 12: 252) are still to check.
- ~~**AppArmor on a real Ubuntu desktop:** can Unbound read its files and the CA bundle under its profile?~~ Yes
  (a real Ubuntu 24.04 desktop, 2026-10-29): Unbound runs confined by its `unbound` profile, in enforce mode,
  reads `/var/lib/unbound/offlok/` and the CA bundle (`tls-cert-bundle`, read at start), and logs no denial.
- **NetworkManager without systemd-resolved:** it rewrites `/etc/resolv.conf` on every change. OffLok puts its
  file back within 5 seconds: is that window acceptable, or should NetworkManager be told `dns=none`?
- **Fedora and SELinux:** does Unbound, started by offlokd rather than by systemd, run in its own domain?
- **Port 53 on loopback:** conflicts with dnsmasq, libvirt or Docker setups on Linux, and with Windows' Internet
  Connection Sharing.
- **Windows:** which API sets per-interface DNS on Windows 10 and 11, and how it interacts with IPv6 RDNSS, Smart
  Multi-Homed Name Resolution, NRPT rules and VPN clients that force their own DNS.
- **Windows:** whether NLnet Labs' Windows build is signed, and what its licence notices require in our installer.
- **macOS:** how VPN clients override DNS, and whether `/etc/resolver` covers their split domains.
- **Captive portals:** the OS signals OffLok can read on each platform (NetworkManager connectivity state, Windows
  Network List Manager, macOS captive network detection).
- **Android:** whether libunbound applies every option of OffLok's template (prefetch, serve-stale, aggressive
  NSEC, `auto-trust-anchor-file`, forward zones with TLS) read through `ub_ctx_config`.
- **Android:** whether Android's captive portal check uses the network's own DNS while OffLok's tunnel is up, and
  how Android's resolver behaves when TCP to the tunnel's address is reset.
- **Android:** the foreground service type Android 14 and later require of a `VpnService`, and Google Play's
  answer for a VPN that carries DNS only, to an on-phone resolver.
- **Android:** whether F-Droid's build server builds Go, OpenSSL and Unbound for the NDK, and byte for byte the
  same APK as ours.
- **Android:** the battery a resolver running all day costs.
- **VPNs:** a full-tunnel VPN that declares no domain. Should its DNS take over, or should recursion stay?
- **Local root zone:** which sources allow transfers today, and whether to verify the zone with ZONEMD.
- **Blocklists:** which lists, their licences, formats and sizes, and Unbound's memory use with them.
- **Self-test:** a stable test name with deliberately broken DNSSEC.
- **iOS:** Network Extension memory limits, and whether a packet tunnel is the only way.
- **Opportunistic encryption to authoritative servers** (RFC 9539): what Unbound supports today.

## Waiting for the maintainer

- **To confirm:** nothing at the moment.
- **The name:** the maintainer reserved the GitHub organisation `OffLok` (2026-10-30), empty: the project stays
  `LokLakh-s/OffLok` for now. No domain: nothing needs one (`offlok.org` and `offlok.com` were free on
  2026-10-30).
