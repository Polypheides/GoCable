package player

import (
	"fmt"
)

const MasterPort = 4999

func formatListenURL(protocol string, port int) string {
	if protocol == "http" {
		return fmt.Sprintf("tcp://127.0.0.1:%d", port)
	}
	if protocol == "tcp" {
		return fmt.Sprintf("tcp://127.0.0.1:%d", port)
	}
	return fmt.Sprintf("udp://@127.0.0.1:%d", port)
}
