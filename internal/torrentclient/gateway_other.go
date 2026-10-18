//go:build !windows

package torrentclient

import (
	"fmt"
	"net"
	"os"
)

func defaultGateway() (net.IP, error) {
	b, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return nil, fmt.Errorf("find the router: %w", err)
	}
	return parseDefaultGateway(string(b))
}
