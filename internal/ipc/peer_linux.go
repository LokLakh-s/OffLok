package ipc

import (
	"errors"
	"net"

	"golang.org/x/sys/unix"
)

// peerOf reads the caller's identity from the kernel (SO_PEERCRED): it cannot be forged by the caller.
func peerOf(conn net.Conn) (Peer, error) {
	c, ok := conn.(*net.UnixConn)
	if !ok {
		return Peer{}, errors.New("not a Unix socket")
	}
	raw, err := c.SyscallConn()
	if err != nil {
		return Peer{}, err
	}
	var cred *unix.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return Peer{}, err
	}
	if credErr != nil {
		return Peer{}, credErr
	}
	return Peer{UID: int(cred.Uid), PID: int(cred.Pid), Admin: cred.Uid == 0}, nil
}
