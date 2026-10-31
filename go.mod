module github.com/loklakh-s/offlok

go 1.27

require (
	github.com/godbus/dbus/v5 v5.2.4
	github.com/miekg/dns v1.1.73
	golang.org/x/sys v0.49.0
)

require (
	golang.org/x/mobile v0.0.0-20261011034528-197770444867 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
)

tool (
	golang.org/x/mobile/cmd/gobind
	golang.org/x/mobile/cmd/gomobile
)
