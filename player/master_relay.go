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

type MasterBroadcaster struct {
	cmd       *exec.Cmd
	sourceURL string
	Protocol  string // "udp", "tcp", or "http"

	// TCP relay support
	mu    sync.Mutex
	conns map[net.Conn]chan []byte
	l     net.Listener

	tuneMu sync.Mutex // Guard against rapid overlapping tune requests
}

func NewMasterBroadcaster() *MasterBroadcaster {
	return &MasterBroadcaster{
		Protocol: "udp", // default
		conns:    make(map[net.Conn]chan []byte),
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
		conn.SetWriteDeadline(time.Now().Add(1 * time.Hour))
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
			for conn, ch := range m.conns {
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
					_ = conn
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
	for conn, ch := range m.conns {
		close(ch)
		_ = conn.Close()
	}
	m.conns = make(map[net.Conn]chan []byte)
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

func (m *MasterBroadcaster) Stream(ctx context.Context, w io.Writer) error {
	ch := make(chan []byte, 1024)
	dummy := &net.TCPConn{}

	m.mu.Lock()
	m.conns[dummy] = ch
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.conns, dummy)
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
