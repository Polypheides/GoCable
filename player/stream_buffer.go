package player

import (
	"context"
	"io"
	"sync"
)

// StreamHub manages a shared ring buffer of MPEG-TS chunks
type StreamHub struct {
	buffer [][]byte
	size   int
	head   int64 // Monotonic index of the next chunk to be written
	closed bool  // Whether the hub is closed and no longer accepting data
	mu     sync.RWMutex
	cond   *sync.Cond
}

func NewStreamHub(size int) *StreamHub {
	h := &StreamHub{
		buffer: make([][]byte, size),
		size:   size,
		cond:   sync.NewCond(&sync.Mutex{}),
	}
	return h
}

func (h *StreamHub) Write(chunk []byte) {
	h.cond.L.Lock()
	if h.closed {
		h.cond.L.Unlock()
		return
	}
	idx := h.head % int64(h.size)
	// We must copy because the source buffer is reused
	if cap(h.buffer[idx]) >= len(chunk) {
		h.buffer[idx] = h.buffer[idx][:len(chunk)]
	} else {
		h.buffer[idx] = make([]byte, len(chunk))
	}
	copy(h.buffer[idx], chunk)
	h.head++
	h.cond.L.Unlock()
	h.cond.Broadcast()
}

func (h *StreamHub) Close() {
	h.cond.L.Lock()
	h.closed = true
	h.cond.L.Unlock()
	h.cond.Broadcast() // Wake up all waiting readers
}

func (h *StreamHub) Get(pos int64) ([]byte, int64, bool) {
	h.cond.L.Lock()
	defer h.cond.L.Unlock()

	// If we are way behind (paused) or just starting (pos < 0),
	// teleport to the last finished chunk so we have data ready instantly.
	if pos < 0 || h.head-pos > int64(h.size) {
		pos = h.head - 1
		if pos < 0 {
			pos = 0
		}
	}

	// Wait for data if we are at the head
	for h.head <= pos && !h.closed {
		h.cond.Wait()
	}

	if h.closed {
		return nil, pos, false
	}

	idx := pos % int64(h.size)
	chunk := h.buffer[idx]
	if chunk == nil {
		// Safety check if buffer hasn't reached this point yet
		return nil, pos, false
	}
	return chunk, pos + 1, true
}

// Stream pipes data from the hub to the writer until the context is canceled or the hub is closed.
func (h *StreamHub) Stream(ctx context.Context, w io.Writer) error {
	pos := h.LiveIndex() - 20
	if pos < 0 {
		pos = 0
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			chunk, nextPos, ok := h.Get(pos)
			if !ok {
				return nil
			}
			_, err := w.Write(chunk)
			if err != nil {
				return err
			}
			pos = nextPos
		}
	}
}

func (h *StreamHub) LiveIndex() int64 {
	h.cond.L.Lock()
	defer h.cond.L.Unlock()
	return h.head
}
