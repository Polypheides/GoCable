package player

import (
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"sync"
	"time"
)

// httpClientKey is a sentinel type used as a map key for HTTP streaming clients
// in MasterBroadcaster, avoiding use of zero-value *net.TCPConn which can panic
// on Close() on some platforms. (Fix #6)
type httpClientKey struct{}

type MasterBroadcaster struct {
	cmd       *exec.Cmd
	sourceURL string
	Protocol  string // "udp", "tcp", or "http"

	// TCP relay support
	mu    sync.Mutex
	conns map[any]chan []byte // key is net.Conn or httpClientKey
	l     net.Listener

	tuneMu sync.Mutex // Guard against rapid overlapping tune requests
}

func NewMasterBroadcaster() *MasterBroadcaster {
	return &MasterBroadcaster{
		Protocol: "udp", // default
		conns:    make(map[any]chan []byte),
	}
}

func (m *MasterBroadcaster) Tune(sourceURL string) error {
	m.tuneMu.Lock()
	defer m.tuneMu.Unlock()

	m.stopFFmpeg()

	time.Sleep(250 * time.Millisecond)
	m.sourceURL = sourceURL
	return m.start()
}

func (m *MasterBroadcaster) start() error {
	if m.sourceURL == "" {
		return nil
	}

	outputURL := "-" // ALWAYS output to stdout pipe

	switch m.Protocol {
	case "tcp", "http":
		if m.l == nil {
			var err error
			m.l, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", MasterPort))
			if err != nil {
				return err
			}
			go m.acceptLoop()
		}
	case "udp":
		// No permanent UDP listener needed for relaying to a standard port
	}

	args := []string{
		"-fflags", "+genpts+igndts+discardcorrupt+nobuffer",
		"-analyzeduration", "1000000",
		"-probesize", "1000000",
		"-avoid_negative_ts", "make_zero",
		"-i", m.sourceURL,
		"-map", "0:v",
		"-map", "0:a?",
		"-sn",
		"-c", "copy",
		"-f", "mpegts",
		"-mpegts_flags", "resend_headers+initial_discontinuity",
		"-pat_period", "0.1",
		"-y", outputURL,
	}

	m.cmd = exec.Command("ffmpeg", args...)

	stdout, err := m.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	go m.relayLoop(stdout)

	return m.cmd.Start()
}

func (m *MasterBroadcaster) acceptLoop() {
	for {
		conn, err := m.l.Accept()
		if err != nil {
			return
		}

		ch := make(chan []byte, 1024)
		m.mu.Lock()
		m.conns[conn] = ch
		m.mu.Unlock()

		go m.connSender(conn, ch)
	}
}

func (m *MasterBroadcaster) connSender(conn net.Conn, ch chan []byte) {
	defer func() {
		conn.Close()
		m.mu.Lock()
		delete(m.conns, conn)
		m.mu.Unlock()
	}()

	for buf := range ch {
		// FIX #9: 5-second deadline instead of 1 hour to release dead connections promptly.
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, err := conn.Write(buf)
		if err != nil {
			return
		}
	}
}

func (m *MasterBroadcaster) relayLoop(r io.Reader) {
	for {
		buf := make([]byte, 188*10)
		n, err := r.Read(buf)
		if n > 0 {
			m.mu.Lock()
			packet := make([]byte, n)
			copy(packet, buf[:n])
			for key, ch := range m.conns {
				select {
				case ch <- packet:
				default:
				mWipeLoop:
					for {
						select {
						case _, ok := <-ch:
							if !ok {
								break mWipeLoop
							}
						default:
							break mWipeLoop
						}
					}
					select {
					case ch <- packet:
					default:
					}
					_ = key
				}
			}
			m.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (m *MasterBroadcaster) Stop() error {
	m.stopFFmpeg()
	if m.l != nil {
		_ = m.l.Close()
		m.l = nil
	}
	m.mu.Lock()
	for key, ch := range m.conns {
		close(ch)
		if conn, ok := key.(net.Conn); ok {
			_ = conn.Close()
		}
	}
	m.conns = make(map[any]chan []byte)
	m.mu.Unlock()
	return nil
}

func (m *MasterBroadcaster) stopFFmpeg() {
	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
		_ = m.cmd.Wait()
		m.cmd = nil
	}
}

// Stream registers an HTTP streaming client using a typed sentinel key (not a
// zero-value *net.TCPConn) and pipes data to w until ctx is cancelled. (Fix #6)
func (m *MasterBroadcaster) Stream(ctx context.Context, w io.Writer) error {
	ch := make(chan []byte, 1024)
	key := httpClientKey{}

	m.mu.Lock()
	m.conns[key] = ch
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.conns, key)
		m.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case buf, ok := <-ch:
			if !ok {
				return nil
			}
			_, err := w.Write(buf)
			if err != nil {
				return err
			}
		}
	}
}

func MasterStreamURL(protocol string) string {
	return formatListenURL(protocol, MasterPort)
}
