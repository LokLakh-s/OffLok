# OffLok

Your computer as its own DNS resolver. OffLok runs [Unbound](https://nlnetlabs.nl/unbound) in recursive mode and
points the system DNS at it: names are looked up starting from the root servers, with no internet provider,
Google, Cloudflare or Quad9 in between. Free software under the MIT licence.

> **Status: early.** Linux (tested on Ubuntu 24.04, with systemd-resolved and without it), Windows 10 (version
> 2004 or later) and 11, and macOS 13 or later (Apple silicon and Intel) work, each with an icon in the tray or the
> menu bar and a window. Android follows, on Google Play and F-Droid, then iOS. See [docs/PLAN.md](docs/PLAN.md),
> [docs/ROADMAP.md](docs/ROADMAP.md) and [CHANGELOG.md](CHANGELOG.md).

## What it changes, honestly

- **No one keeps the list of what you look up.** Each question goes to the servers in charge of that name, and
  each of them only sees its own part (QNAME minimisation).
- **Answers are checked on your machine** (DNSSEC), and keep coming when your provider's resolver is down.
- **Queries are not encrypted.** Your provider or the Wi-Fi operator can still read them as they pass. OffLok is
  not a VPN, and it hides nothing about the sites you then connect to.
- **The first lookup of a name is slower**, then it comes from the local cache.

## How it behaves

| The icon would be | What happens |
|---|---|
| green: `On` | Unbound resolves by itself, from the root servers |
| orange: `Fallback` | the root servers are out of reach (a hotel's Wi-Fi, a network that blocks port 53): your DNS-over-TLS resolver if you set one, otherwise the network's DNS (reduced protection) |
| orange: `Strict mode` | out of reach, and strict mode is on: never the network's DNS, public names do not resolve |
| grey: `Off` | the system has its original DNS back |
| red: `Error` | something failed: the original DNS is given back (except in strict mode) and OffLok tries again a minute later |

OffLok never breaks your internet on purpose: every change is self-tested, a failure gives the original DNS
back, and stopping or uninstalling restores the system exactly as it was. Unbound only listens on `127.0.0.1` and
`::1`. Nothing is logged, and what OffLok contacts is listed under [Privacy](#privacy).

## Download

From [the releases](https://github.com/LokLakh-s/OffLok/releases): `offlok-<version>-setup.exe` for Windows,
`offlok-<version>.pkg` for macOS, the `.deb` or `.rpm` packages for Linux (`offlok`, and `offlok-desktop` for the
tray icon and window).

Every release has a `SHA256SUMS` file, signed with OffLok's release key
([`offlok-release-45e724ba.pub.pem`](packaging/release/offlok-release-45e724ba.pub.pem), Ed25519). To check a
download with OpenSSL 3:

```
openssl pkeyutl -verify -pubin -inkey offlok-release-45e724ba.pub.pem -rawin -in SHA256SUMS -sigfile SHA256SUMS.sig
sha256sum --ignore-missing -c SHA256SUMS
```

## Use

```
offlok                          what OffLok is doing
sudo offlok off                 give the system its original DNS back (sudo offlok on to come back)
sudo offlok strict on           never fall back to the network's DNS
sudo offlok dot dns.example.org use this DNS-over-TLS resolver when the root servers are out of reach
sudo offlok access everyone     let every user change settings (administrators only by default)
sudo offlok updates off         stop looking for updates (sudo offlok update installs one now)
```

Blocking trackers and malicious sites is included and off at first:

```
sudo offlok block trackers on   also: malicious, custom (your own lists)
offlok check example.org        is this site blocked, and by which list?
sudo offlok trust example.org   never block this site (sudo offlok untrust to undo)
offlok pause                    let everything through for 10 minutes (offlok pause off to end it)
sudo offlok list add https://example.org/list.txt
```

The lists (ShadowWhisperer, HaGeZi, URLhaus and others, each under its own licence) are downloaded by your
computer once a day, only for the categories switched on; OffLok never ships them.

The command and the window speak English or French, following the system language. On a desktop, the
`offlok-desktop` package adds the tray icon (a dot when OffLok falls back or fails, grey when off) and its window;
it asks for the administrator's password (polkit) when a setting needs it. In strict mode, "Sign in to this
network" (or `offlok signin`) lets the network's DNS through for 5 minutes, the time to open a hotel's sign-in
page.

On Windows, `offlok-<version>-setup.exe` installs everything: the `OffLok` service, Unbound, the tray icon (in
every session) and the `offlok` command, used without `sudo` from a terminal run as administrator. The tray app
shows Windows's own prompt (UAC) when a setting needs an administrator. Uninstalling gives every network
interface its original DNS back.

On macOS, `offlok-<version>.pkg` installs everything: offlokd under launchd, Unbound (built with OpenSSL 3.5),
OffLok in the menu bar (started at every login) and the `offlok` command, used with `sudo`. The package is not
signed yet: macOS refuses to open it at first, and System Settings › Privacy & Security then offers "Open Anyway".
The menu bar app asks macOS for an administrator when a setting needs one. To uninstall, which gives every network
service its original DNS back:

```
sudo /usr/local/libexec/offlok/uninstall.sh
```

Once a day, OffLok looks for a new release; the window then offers it, and `offlok update` installs it. An update
is installed only if `SHA256SUMS` carries the release key's signature and every file matches it.

## Build and test

Needs Go 1.27 and Docker.

```
packaging/linux/build.sh          # binaries in bin/<arch>/, .deb and .rpm in dist/
packaging/linux/build-desktop.sh  # offlok-desktop, in packaging/linux/desktop-build's image (GTK 3, WebKitGTK)
test/integration/run.sh           # the main scenario in a disposable systemd container
test/integration/run.sh package   # the .deb's install and removal scripts
test/integration/run.sh desktop   # the desktop packages on a virtual screen, screenshots in bin/shots/
packaging/windows/build.sh        # the Windows installer, dist/offlok-<version>-setup.exe (built on Linux too)
packaging/macos/build-unbound.sh  # on a Mac: Unbound with OpenSSL linked in, one program for both processors
packaging/macos/build-pkg.sh      # on a Mac, after it: dist/offlok-<version>.pkg
site/build.sh                     # the website, offlok.org, in bin/site/ (French, and English under en/)
```

`test/windows/test.ps1` checks the Windows installer on a disposable Windows machine, and `test/macos/test.sh` the
macOS package on a disposable Mac (the CI uses GitHub's).

The integration tests change nothing on the machine running them: each container has its own network
namespace, resolver and firewall.

## Privacy

OffLok keeps no record of the names you look up and sends nothing about you anywhere. What it contacts:

- **The DNS servers in charge of each name you look up**, starting from the root servers: that is its job.
  Unencrypted, like any DNS: see "What it changes, honestly" above.
- **The blocking lists you switch on**, downloaded once a day from their authors (HaGeZi, URLhaus and the
  others, or the addresses you add).
- **GitHub, once a day, to look for an update**, which lets GitHub see your computer's address and that it runs
  OffLok. The Windows installer says so next to a box that turns it off; the window has a switch, and so does the
  command (`offlok updates off`).
- **Your DNS-over-TLS resolver**, if you set one, when the root servers are out of reach.

## Code signing policy

Windows releases are to be signed through the SignPath Foundation: free code signing provided by
[SignPath.io](https://about.signpath.io), certificate by [SignPath Foundation](https://signpath.org). Until that
starts, the Windows installer is not signed, and Windows warns before running it.

- Every signed file is built by [the release workflow](.github/workflows/release.yml) on GitHub's machines, from
  this repository's source at the release's tag, and checked by the tests before it is published.
- Committers and reviewers: the maintainers of [LokLakh-s](https://github.com/LokLakh-s). Changes proposed by
  anyone else are reviewed by a committer before they are merged.
- Approver, who approves every signing request: a maintainer of [LokLakh-s](https://github.com/LokLakh-s).
- What OffLok sends over the network is listed under [Privacy](#privacy).

## Licence

MIT, see [LICENSE](LICENSE). OffLok uses Unbound (BSD-3-Clause, NLnet Labs), installed from the distribution on
Linux, shipped from NLnet Labs' own build on Windows, and built from NLnet Labs' source with OpenSSL (Apache 2.0)
on macOS.
