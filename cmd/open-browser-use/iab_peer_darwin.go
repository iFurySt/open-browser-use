//go:build darwin

package main

import (
	"errors"
	"fmt"
	"github.com/ifuryst/open-browser-use/internal/wire"
	"golang.org/x/sys/unix"
	"net"
	"unsafe"
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
	var token [32]byte
	length := uint32(len(token))
	var syscallErr error
	err = raw.Control(func(fd uintptr) {
		_, _, e := unix.Syscall6(unix.SYS_GETSOCKOPT, fd, 0, 6, uintptr(unsafe.Pointer(&token[0])), uintptr(unsafe.Pointer(&length)), 0)
		if e != 0 {
			syscallErr = e
		}
	})
	if err != nil || syscallErr != nil || length != 32 {
		return errors.New("AUTH_UNAVAILABLE: cannot read IAB kernel peer")
	}
	pid := int(wire.NativeEndian().Uint32(token[20:24]))
	uid := wire.NativeEndian().Uint32(token[4:8])
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return errors.New("IAB server identity unavailable")
	}
	start := fmt.Sprintf("%d:%d", info.Proc.P_starttime.Sec, info.Proc.P_starttime.Usec)
	if pid != expected.PID || uid != expected.UID || start != expected.StartTime {
		return errors.New("IAB server identity mismatch")
	}
	return nil
}
