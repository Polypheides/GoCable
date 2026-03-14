//go:build !vlc

package player

import (
	"time"
)

// NewLivePlayer returns a NullPlayer by default when VLC is not enabled.
func NewLivePlayer(master *MasterBroadcaster) Player {
	return &NullPlayer{}
}

// NullPlayer advances the current item in the MediaList every 30 minutes. It (poorly) mimics the list of media being watched, as if it was on another channel.
type NullPlayer struct {
	list   *MediaList
	ticker *time.Ticker
	done   chan bool
}

func (n *NullPlayer) Init() error {
	return nil
}

func (n *NullPlayer) Play(list *MediaList) error {
	n.list = list
	if n.ticker != nil {
		return nil // Already running
	}
	n.ticker = time.NewTicker(time.Minute * 30)
	n.done = make(chan bool)
	go func() {
		for {
			select {
			case <-n.done:
				return
			case <-n.ticker.C:
				n.PlayNext()
			}
		}
	}()
	return nil
}

func (n *NullPlayer) PlayURL(url string) error {
	// NullPlayer doesn't actually play anything
	return nil
}

// FIX #7: nil guard on list before calling any method on it.

func (n *NullPlayer) PlayNext() error {
	if n.list != nil {
		n.list.Advance()
	}
	return nil
}

func (n *NullPlayer) PlayPrevious() error {
	if n.list != nil {
		n.list.Rewind()
	}
	return nil
}

func (n *NullPlayer) Next() string {
	if n.list != nil {
		return n.list.Next()
	}
	return ""
}

func (n *NullPlayer) Current() string {
	if n.list != nil {
		return n.list.Current()
	}
	return ""
}

func (n *NullPlayer) Shutdown() error {
	if n.ticker != nil {
		n.ticker.Stop()
	}
	if n.done != nil {
		select {
		case n.done <- true:
		default:
		}
	}
	return nil
}
