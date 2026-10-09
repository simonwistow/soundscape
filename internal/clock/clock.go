// Package clock lets the parts of a soundscape that wait for time to pass -
// the engine's scattered events, a synth's note-offs, a MIDI file's timing -
// run on the wall clock when playing live, or on a virtual clock when
// rendering faster than real time.
package clock

import (
	"container/heap"
	"sync"
	"time"
)

// Clock tells the time and runs functions later.
type Clock interface {
	Now() time.Time
	// AfterFunc runs f once d has passed, on its own goroutine for a real
	// clock, or from Advance for a virtual one.
	AfterFunc(d time.Duration, f func())
}

// Real is the wall clock.
type Real struct{}

func (Real) Now() time.Time                      { return time.Now() }
func (Real) AfterFunc(d time.Duration, f func()) { time.AfterFunc(d, f) }

// Virtual is a clock that only moves when Advance moves it, firing the
// functions that come due on the way, in order.
type Virtual struct {
	mu     sync.Mutex
	now    time.Time
	timers timerHeap
	seq    uint64
}

func NewVirtual(start time.Time) *Virtual {
	return &Virtual{now: start}
}

func (v *Virtual) Now() time.Time {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.now
}

func (v *Virtual) AfterFunc(d time.Duration, f func()) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.seq++
	heap.Push(&v.timers, timer{at: v.now.Add(max(d, 0)), seq: v.seq, f: f})
}

// Advance moves the clock forward to t, running every function due by then
// in the order they're due (those due together in the order they were
// scheduled). While each runs, Now is the time it was due, and anything it
// schedules that comes due by t runs too.
func (v *Virtual) Advance(t time.Time) {
	for {
		v.mu.Lock()
		if len(v.timers) == 0 || v.timers[0].at.After(t) {
			if t.After(v.now) {
				v.now = t
			}
			v.mu.Unlock()
			return
		}
		next := heap.Pop(&v.timers).(timer)
		if next.at.After(v.now) {
			v.now = next.at
		}
		v.mu.Unlock()
		next.f()
	}
}

type timer struct {
	at  time.Time
	seq uint64
	f   func()
}

type timerHeap []timer

func (h timerHeap) Len() int { return len(h) }
func (h timerHeap) Less(i, j int) bool {
	if !h[i].at.Equal(h[j].at) {
		return h[i].at.Before(h[j].at)
	}
	return h[i].seq < h[j].seq
}
func (h timerHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *timerHeap) Push(x any)   { *h = append(*h, x.(timer)) }
func (h *timerHeap) Pop() any {
	old := *h
	t := old[len(old)-1]
	*h = old[:len(old)-1]
	return t
}
