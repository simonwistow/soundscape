package audio

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type constSource float32

func (c constSource) Render(buf []float32) {
	for i := range buf {
		buf[i] += float32(c)
	}
}

type recordingSink struct {
	mu     sync.Mutex
	blocks [][]float32
	closed bool
	failAt int // fail the nth Write (1-based); 0 never fails
	writes int
}

func (s *recordingSink) Write(frames []float32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		panic("Write after Close")
	}
	s.writes++
	if s.writes == s.failAt {
		return errors.New("disk full")
	}
	s.blocks = append(s.blocks, append([]float32(nil), frames...))
	return nil
}

func (s *recordingSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *recordingSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.blocks)
}

func TestMixerSumsAndClampsSources(t *testing.T) {
	m := NewMixer(44100)
	m.AddSource(constSource(0.25))
	m.AddSource(constSource(0.5))
	quiet := &recordingSink{}
	m.AddSink(quiet)
	m.Start()
	waitFor(t, func() bool { return quiet.count() > 0 })
	m.Close()

	if got := quiet.blocks[0][0]; got != 0.75 {
		t.Errorf("sum = %v, want 0.75", got)
	}

	m = NewMixer(44100)
	m.AddSource(constSource(0.75))
	m.AddSource(constSource(0.75))
	loud := &recordingSink{}
	m.AddSink(loud)
	m.Start()
	waitFor(t, func() bool { return loud.count() > 0 })
	m.Close()

	if got := loud.blocks[0][0]; got != 1 {
		t.Errorf("clamped sum = %v, want 1", got)
	}
}

func TestMixerKeepsPaceWithTheClock(t *testing.T) {
	m := NewMixer(44100)
	sink := &recordingSink{}
	m.AddSink(sink)
	m.Start()
	time.Sleep(500 * time.Millisecond)
	m.Close()

	// 0.5 s is about 43 blocks; allow for scheduling slop either way.
	if n := sink.count(); n < 30 || n > 50 {
		t.Errorf("rendered %d blocks in 0.5 s, want about 43", n)
	}
}

func TestMixerDropsAFailingSinkAndCarriesOn(t *testing.T) {
	m := NewMixer(44100)
	bad := &recordingSink{failAt: 2}
	good := &recordingSink{}
	m.AddSink(bad)
	m.AddSink(good)
	m.Start()
	waitFor(t, func() bool { return good.count() > 5 })
	err := m.Close()

	if bad.count() != 1 {
		t.Errorf("failing sink got %d blocks, want 1 (none after it failed)", bad.count())
	}
	if err == nil || err.Error() != "disk full" {
		t.Errorf("Close() = %v, want the sink's error", err)
	}
}

func TestMixerClosesSinksAfterTheLoopStops(t *testing.T) {
	m := NewMixer(44100)
	sink := &recordingSink{} // panics on a Write after Close
	m.AddSink(sink)
	m.Start()
	waitFor(t, func() bool { return sink.count() > 0 })
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("second Close() = %v", err)
	}
	if !sink.closed {
		t.Error("sink not closed")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestMixerCloseBeforeStart(t *testing.T) {
	m := NewMixer(44100)
	sink := &recordingSink{}
	m.AddSink(sink)

	done := make(chan error)
	go func() { done <- m.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close before Start hung")
	}
	if !sink.closed {
		t.Error("sink not closed")
	}
}
