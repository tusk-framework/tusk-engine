package control

import (
	"net"
	"strings"
)

func isLoopbackAddress(address string) bool {
	address = strings.TrimSpace(address)
	if address == "" || strings.EqualFold(address, "localhost") {
		return true
	}

	ip := net.ParseIP(address)

	return ip != nil && ip.IsLoopback()
}
