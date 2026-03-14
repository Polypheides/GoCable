package network

import (
	"net"
	"strings"
)

// GetLocalIP returns the non-loopback local IP of the host
func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ip := ipnet.IP.To4(); ip != nil {
				ipStr := ip.String()
				// Limit to common domestic/private subnets as requested
				if strings.HasPrefix(ipStr, "192.168.") || strings.HasPrefix(ipStr, "10.10.") {
					return ipStr
				}
			}
		}
	}
	return "127.0.0.1"
}
