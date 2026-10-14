//go:build !darwin && !linux

package main

import (
	"errors"
	"net"
)

func verifyIabPeer(_ net.Conn, _ iabIdentity) error {
	return errors.New("AUTH_UNAVAILABLE: IAB socket peer verification requires macOS or Linux")
}
