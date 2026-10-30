package ipc

// DefaultSocket is where offlokd listens on macOS, which has no /run.
const DefaultSocket = "/var/run/offlok/offlok.sock"
