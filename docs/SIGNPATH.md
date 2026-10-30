# SignPath Foundation application

The answers to [SignPath Foundation's application form](https://signpath.org/apply), read on 2026-10-30, ready to
be pasted. The maintainer agreed to apply (2026-10-30). The maintainer sends the form: it asks for their name and
e-mail address and for their agreement to SignPath's Code of Conduct and to the processing of their data, behind a
reCAPTCHA.

**Before sending:** the repository is public, release 0.5.0 is published (the releases page mentions SignPath),
and the README carries the code signing policy and the privacy section.

| Field | Answer |
|---|---|
| Project Name | OffLok |
| Repository URL | https://github.com/LokLakh-s/OffLok |
| Homepage URL | https://github.com/LokLakh-s/OffLok |
| Download URL | https://github.com/LokLakh-s/OffLok/releases |
| Privacy Policy URL | https://github.com/LokLakh-s/OffLok#privacy |
| Wikipedia URL | (none) |
| Category | Other |
| Maintainer Type | Individual maintainer(s) |
| Build System | GitHub Actions |
| First Name, Last Name, Email | the maintainer's own |
| Company Name | (empty) |
| Primary Discovery Channel | AI / LLM tools |

**Tagline**

> Your computer as its own DNS resolver: names looked up from the root servers, with no provider in between.

**Description**

> OffLok makes a computer resolve DNS by itself. It installs and runs Unbound (NLnet Labs) in recursive mode on
> the loopback, points every network interface's DNS at it, and keeps it working: each change is self-tested,
> anything that fails gives the system its original DNS back, and uninstalling restores every interface as it
> was. When the root servers are out of reach (a hotel's Wi-Fi), it falls back to the user's DNS-over-TLS
> resolver or, unless strict mode is on, to the network's DNS. It can block ads, trackers and malicious sites with
> public lists, off by default. Windows and Linux today, macOS next; MIT licence, a tray icon and a window in
> English and French. The Windows installer ships NLnet Labs' official Unbound build, so OffLok also delivers
> Unbound's security fixes through signed updates.

**Reputation** (to complete with what exists by then: release downloads, stars, posts, discussions)

> OffLok is a new project: its first public release, 0.5.0, came out on <date>. Its builds and tests are public
> (GitHub Actions: unit tests, integration tests in disposable systemd containers, install, update and uninstall
> tests on Windows). <Links to the announcement, discussions, download numbers.>

SignPath accepts at its discretion; a project with no users yet may be asked to come back later.

**Code of Conduct and data processing:** the two required boxes are the maintainer's to tick; the box for other
communications is optional.
