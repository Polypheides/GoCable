//go:build vlc

package player

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// NewLivePlayer returns a VLCPlayer when the 'vlc' build tag is provided.
func NewLivePlayer(master *MasterBroadcaster) Player {
	return &VLCPlayer{master: master}
}

type VLCPlayer struct {
	list   *MediaList
	master *MasterBroadcaster

	cmd  *exec.Cmd
	done chan struct{}
}

func (p *VLCPlayer) Init() error {
	p.done = make(chan struct{})
	return nil
}

func (p *VLCPlayer) Shutdown() error {
	if p.done != nil {
		close(p.done)
		p.done = nil
	}

	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
		p.cmd = nil
	}
	return nil
}

func (p *VLCPlayer) Play(list *MediaList) error {
	p.list = list
	return p.PlayURL(p.list.Current())
}

func findVLCBinary() string {
	if path, err := exec.LookPath("vlc"); err == nil {
		return path
	}

	var fallbackPaths []string
	if runtime.GOOS == "windows" {
		fallbackPaths = []string{
			`vlc.exe`,
			`C:\Program Files\VideoLAN\VLC\vlc.exe`,
			`C:\Program Files (x86)\VideoLAN\VLC\vlc.exe`,
		}
	} else { // Linux
		fallbackPaths = []string{
			"/usr/bin/vlc",
			"/snap/bin/vlc",
			"/var/lib/flatpak/app/org.videolan.VLC/current/active/files/bin/vlc",
		}
	}

	for _, p := range fallbackPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func (p *VLCPlayer) PlayURL(url string) error {
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
		p.cmd = nil
	}

	bin := findVLCBinary()

	// Dynamically override the URL to track the MasterBroadcaster
	masterURL := MasterStreamURL(p.master.Protocol)

	if bin == "" {
		// Headless fallback
		fmt.Printf("[Player] VLC executable not found on host system. Operating in headless mode tracking stream: %s\n", masterURL)
		return nil
	}

	args := []string{
		"--fullscreen",
		"--no-video-title-show",
		"--play-and-exit",
		masterURL,
	}

	p.cmd = exec.Command(bin, args...)

	err := p.cmd.Start()
	if err != nil {
		fmt.Printf("[Player] Failed to launch VLC process securely. Falling back to headless execution: %v\n", err)
		p.cmd = nil
	}

	return nil
}

func (p *VLCPlayer) PlayNext() error {
	return p.PlayURL(p.list.Advance())
}

func (p *VLCPlayer) PlayPrevious() error {
	return p.PlayURL(p.list.Rewind())
}

func (p *VLCPlayer) Next() string {
	return p.list.Next()
}

func (p *VLCPlayer) Current() string {
	return p.list.Current()
}
