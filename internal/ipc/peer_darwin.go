package ipc

import (
	"errors"
	"net"

	"golang.org/x/sys/unix"
)

// peerOf reads the caller's identity from the kernel (LOCAL_PEERCRED, LOCAL_PEERPID): it cannot be forged.
func peerOf(conn net.Conn) (Peer, error) {
	c, ok := conn.(*net.UnixConn)
	if !ok {
		return Peer{}, errors.New("not a Unix socket")
	}
	raw, err := c.SyscallConn()
	if err != nil {
		return Peer{}, err
	}
	var cred *unix.Xucred
	var pid int
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if credErr == nil {
			pid, credErr = unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
		}
	}); err != nil {
		return Peer{}, err
	}
	if credErr != nil {
		return Peer{}, credErr
	}
	return Peer{UID: int(cred.Uid), PID: pid, Admin: cred.Uid == 0}, nil
}
