module github.com/loklakh-s/offlok

go 1.27

toolchain go1.27.4

require (
	github.com/godbus/dbus/v5 v5.2.4
	github.com/miekg/dns v1.1.73
	golang.org/x/sys v0.50.0
)

require (
	golang.org/x/mobile v0.0.0-20261011034528-197770444867 // indirect
	golang.org/x/mod v0.43.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
)

tool (
	golang.org/x/mobile/cmd/gobind
	golang.org/x/mobile/cmd/gomobile
)
