// Package audio holds small pieces of audio plumbing shared by output
// backends (SoundFont synth, sample player) that render audio on their own
// goroutine and feed it to an Oto player.
package audio

import (
	"fmt"
	"sync"
)

// Pipe is a bounded blocking byte queue: a renderer goroutine Writes
// rendered audio as it's produced, and Oto's player goroutine Reads it on
// demand. Write blocks while the queue is full, so the audio device paces
// the renderer and events are heard as soon as the queue allows, rather
// than the renderer running ahead and building up latency. It implements
// io.Reader and io.Writer.
type Pipe struct {
	mu     sync.Mutex
	cond   *sync.Cond
	buf    []byte
	limit  int
	closed bool
}

// NewPipe returns a pipe that holds at most limit bytes before Write blocks.
// A single Write larger than limit is still accepted once the queue is empty.
func NewPipe(limit int) *Pipe {
	p := &Pipe{limit: limit}
	p.cond = sync.NewCond(&p.mu)
	return p
}

func (p *Pipe) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for len(p.buf) > 0 && len(p.buf)+len(b) > p.limit && !p.closed {
		p.cond.Wait()
	}
	if p.closed {
		return 0, fmt.Errorf("audio pipe closed")
	}

	p.buf = append(p.buf, b...)
	p.cond.Broadcast()
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
	// Shift rather than reslice, so the backing array is reused instead of
	// growing forever as the start of the slice walks forward.
	p.buf = p.buf[:copy(p.buf, p.buf[n:])]
	p.cond.Broadcast()
	return n, nil
}

func (p *Pipe) Close() {
	p.mu.Lock()
	p.closed = true
	p.cond.Broadcast()
	p.mu.Unlock()
}
