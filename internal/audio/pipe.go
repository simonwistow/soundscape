// Package audio holds small pieces of audio plumbing shared by output
// backends (SoundFont synth, sample player) that render audio on their own
// goroutine and feed it to an Oto player.
package audio

import (
	"fmt"
	"sync"
)

// Pipe is a blocking byte queue: a renderer goroutine Writes rendered audio
// as it's produced, and Oto's player goroutine Reads it on demand. It
// implements io.Reader and io.Writer.
type Pipe struct {
	mu     sync.Mutex
	cond   *sync.Cond
	buf    []byte
	closed bool
}

func NewPipe() *Pipe {
	p := &Pipe{}
	p.cond = sync.NewCond(&p.mu)
	return p
}

func (p *Pipe) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return 0, fmt.Errorf("audio pipe closed")
	}

	p.buf = append(p.buf, b...)
	p.cond.Signal()
	return len(b), nil
}

func (p *Pipe) Read(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for len(p.buf) == 0 && !p.closed {
		p.cond.Wait()
	}

	if len(p.buf) == 0 && p.closed {
		return 0, fmt.Errorf("audio pipe closed")
	}

	n := copy(b, p.buf)
	p.buf = p.buf[n:]
	return n, nil
}

func (p *Pipe) Close() {
	p.mu.Lock()
	p.closed = true
	p.cond.Broadcast()
	p.mu.Unlock()
}
