package torrentclient

import (
	"errors"
	"fmt"
	"net"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Ask Windows which interface routes internet traffic, then read that adapter's
// gateway. This follows route metrics when Ethernet and Wi-Fi are both active.
// GetBestInterfaceEx only reads the routing table; it sends no traffic.
func defaultGateway() (net.IP, error) {
	var index uint32
	if err := windows.GetBestInterfaceEx(&windows.SockaddrInet4{Addr: [4]byte{1, 1, 1, 1}}, &index); err != nil {
		return nil, fmt.Errorf("find the router interface: %w", err)
	}
	size := uint32(15000)
	for attempt := 0; attempt < 3; attempt++ {
		buf := make([]byte, size)
		first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(windows.AF_INET, windows.GAA_FLAG_INCLUDE_GATEWAYS, 0, first, &size)
		if errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("find the router: %w", err)
		}
		for adapter := first; adapter != nil; adapter = adapter.Next {
			if adapter.IfIndex != index {
				continue
			}
			for gateway := adapter.FirstGatewayAddress; gateway != nil; gateway = gateway.Next {
				if ip := gateway.Address.IP().To4(); ip != nil && !ip.IsUnspecified() {
					return append(net.IP(nil), ip...), nil
				}
			}
		}
		return nil, errors.New("no default IPv4 gateway on the internet interface")
	}
	return nil, errors.New("network adapters changed while finding the router")
}
