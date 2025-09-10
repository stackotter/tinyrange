//go:build linux || darwin || freebsd || openbsd || windows

package netstack

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/tinyrange/wireguard"

	"github.com/tinyrange/tinyrange/pkg/common"
)

func (ns *NetStack) SetupWireguard(config string, mtu int, guestIp string) error {
	handler := wireguard.NewSimpleFlowHandler()

	if guestIp == "" {
		guestIp = "10.40.0.2"
	}

	wg, err := wireguard.NewFromConfig(guestIp, mtu, config, handler)
	if err != nil {
		return err
	}

	ns.wg = wg

	// Use the connection so it establishes with the server.
	go func() {
		conn, _ := ns.wg.Dial("tcp", "10.40.0.1:8080")
		if conn != nil {
			conn.Close()
		}
	}()

	selfIp := fmt.Sprintf("%d.%d.%d.%d", ns.guestIPv4[0], ns.guestIPv4[1], ns.guestIPv4[2], ns.guestIPv4[3])
	listen1, err := handler.ListenTCPAddr(fmt.Sprintf("%s:0", selfIp))
	if err != nil {
		return err
	}

	listen2, err := handler.ListenTCPAddr(fmt.Sprintf("%s:0", guestIp))
	if err != nil {
		return err
	}

	var listeners []net.Listener
	listeners = append(listeners, listen1, listen2)

	for _, listen := range listeners {
		go func() {
			for {
				conn, err := listen.Accept()
				if err != nil {
					ns.log.Error("failed to accept connection", "err", err)
					return
				}

				ns.log.Debug("accepted connection", "conn", conn.LocalAddr().String())

				go func() {
					defer conn.Close()

					parts := strings.Split(conn.LocalAddr().String(), ":")
					port := parts[len(parts)-1]

					backendAddr := fmt.Sprintf("%s:%s", selfIp, port)
					ns.log.Debug("connecting", "backendAddr", backendAddr)
					backend, err := ns.DialInternalContext(context.Background(), "tcp", backendAddr)
					if err != nil {
						ns.log.Error("failed to dial backend", "err", err)
						return
					}
					defer backend.Close()

					ns.log.Debug("proxying", "backendAddr", backendAddr)
					if err := common.Proxy(backend, conn, 1400); err != nil {
						ns.log.Error("proxy error", "err", err)
					}
					ns.log.Debug("done proxying")
				}()
			}
		}()
	}

	return nil
}
