//go:build windows

package main

import (
 "net"
 "syscall"
)

func enableUDPBroadcast(conn *net.UDPConn) error {
 raw, err := conn.SyscallConn()
 if err != nil { return err }
 var optionErr error
 err = raw.Control(func(fd uintptr) {
  optionErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
 })
 if err != nil { return err }
 return optionErr
}
