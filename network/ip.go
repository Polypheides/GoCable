package network

import (
	"net"
	"strconv"
	"strings"
)

// GetLocalIP returns the non-loopback local IPv4 address of the host,
// filtered to RFC 1918 private ranges (192.168.x.x, 10.x.x.x, 172.16-31.x.x).
func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ip := ipnet.IP.To4(); ip != nil {
				ipStr := ip.String()
				if isPrivateIP(ipStr) {
					return ipStr
				}
			}
		}
	}
	return "127.0.0.1"
}

// isPrivateIP checks whether the given IPv4 address falls within RFC 1918 private ranges:
//
//	10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
func isPrivateIP(ipStr string) bool {
	if strings.HasPrefix(ipStr, "192.168.") {
		return true
	}
	if strings.HasPrefix(ipStr, "10.") {
		return true
	}
	// 172.16.0.0/12 covers 172.16.x.x – 172.31.x.x
	if strings.HasPrefix(ipStr, "172.") {
		parts := strings.SplitN(ipStr, ".", 3)
		if len(parts) >= 2 {
			second, err := strconv.Atoi(parts[1])
			if err == nil && second >= 16 && second <= 31 {
				return true
			}
		}
	}
	return false
}
