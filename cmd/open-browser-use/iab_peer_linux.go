//go:build linux

package main

import (
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"net"
	"os"
	"strings"
)

func verifyIabPeer(conn net.Conn, expected iabIdentity) error {
	c, ok := conn.(*net.UnixConn)
	if !ok {
		return errors.New("AUTH_UNAVAILABLE: IAB needs a Unix socket")
	}
	raw, err := c.SyscallConn()
	if err != nil {
		return err
	}
	var peer *unix.Ucred
	var peerErr error
	if err = raw.Control(func(fd uintptr) { peer, peerErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED) }); err != nil {
		return err
	}
	if peerErr != nil {
		return errors.New("AUTH_UNAVAILABLE: cannot read IAB peer")
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", peer.Pid))
	if err != nil {
		return errors.New("IAB server identity unavailable")
	}
	last := strings.LastIndexByte(string(data), ')')
	if last < 0 {
		return errors.New("IAB server identity unavailable")
	}
	fields := strings.Fields(string(data[last+1:]))
	if len(fields) < 20 {
		return errors.New("IAB server identity unavailable")
	}
	if int(peer.Pid) != expected.PID || peer.Uid != expected.UID || fields[19] != expected.StartTime {
		return errors.New("IAB server identity mismatch")
	}
	return nil
}
