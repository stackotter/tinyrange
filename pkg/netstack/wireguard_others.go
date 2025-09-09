//go:build !(linux || darwin || freebsd || openbsd || windows)

package netstack

import (
	"fmt"
	"runtime"
)

func (ns *NetStack) SetupWireguard(config string, mtu int, guestIp string) error {
	return fmt.Errorf("wireguard is not supported on this platform %s", runtime.GOOS)
}
