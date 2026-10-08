package midi

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/simonwistow/soundscape/internal/event"
	"gitlab.com/gomidi/midi/v2/drivers"
	"gitlab.com/gomidi/midi/v2/drivers/rtmididrv"
)

// LiveOutput is an output.Output that sends note/control events to a MIDI
// port in real time: either an existing port (a synth, a DAW, the macOS IAC
// Driver) or a virtual port it creates for other software to connect to.
type LiveOutput struct {
	driver *rtmididrv.Driver
	port   drivers.Out

	mu       sync.Mutex
	sounding map[[2]int]int // (channel, pitch) -> notes still held
	channels map[int]bool   // channels used, to silence on Close
	closed   bool
}

// Ports lists the names of the MIDI output ports on this system.
func Ports() ([]string, error) {
	drv, err := rtmididrv.New()
	if err != nil {
		return nil, err
	}
	defer drv.Close()
	outs, err := drv.Outs()
	if err != nil {
		return nil, fmt.Errorf("midi: listing ports: %w", systemError(err))
	}
	names := make([]string, len(outs))
	for i, o := range outs {
		names[i] = o.String()
	}
	return names, nil
}

// OpenPort connects to the existing MIDI output port called name. A name
// that matches no port exactly may be part of one, as long as it's part of
// only one, so "IAC" finds "IAC Driver Bus 1".
func OpenPort(name string) (*LiveOutput, error) {
	drv, err := rtmididrv.New()
	if err != nil {
		return nil, err
	}
	outs, err := drv.Outs()
	if err != nil {
		drv.Close()
		return nil, fmt.Errorf("midi: listing ports: %w", systemError(err))
	}

	port, err := findPort(outs, name)
	if err != nil {
		drv.Close()
		return nil, err
	}
	if err := port.Open(); err != nil {
		drv.Close()
		return nil, fmt.Errorf("midi: opening %q: %w", port.String(), err)
	}
	return newLiveOutput(drv, port), nil
}

// OpenVirtual creates a MIDI port called name that other software sees as
// an input it can play from. Not every platform has virtual ports: macOS
// (CoreMIDI) and Linux (ALSA) do, Windows doesn't.
func OpenVirtual(name string) (*LiveOutput, error) {
	drv, err := rtmididrv.New()
	if err != nil {
		return nil, err
	}
	port, err := drv.OpenVirtualOut(name)
	if err != nil {
		drv.Close()
		return nil, fmt.Errorf("midi: creating virtual port %q: %w", name, systemError(err))
	}
	return newLiveOutput(drv, port), nil
}

// systemError explains a failure to reach the system's MIDI layer at all,
// since RtMidi's own message for it can be unhelpful (or garbled).
func systemError(err error) error {
	return fmt.Errorf("can't reach the system's MIDI service (on Linux, the ALSA sequencer, /dev/snd/seq): %w", err)
}

func newLiveOutput(drv *rtmididrv.Driver, port drivers.Out) *LiveOutput {
	return &LiveOutput{
		driver:   drv,
		port:     port,
		sounding: make(map[[2]int]int),
		channels: make(map[int]bool),
	}
}

func findPort(outs []drivers.Out, name string) (drivers.Out, error) {
	var partial []drivers.Out
	for _, o := range outs {
		if o.String() == name {
			return o, nil
		}
		if strings.Contains(o.String(), name) {
			partial = append(partial, o)
		}
	}
	if len(partial) == 1 {
		return partial[0], nil
	}

	names := make([]string, len(outs))
	for i, o := range outs {
		names[i] = fmt.Sprintf("%q", o.String())
	}
	available := "none"
	if len(names) > 0 {
		available = strings.Join(names, ", ")
	}
	if len(partial) > 1 {
		return nil, fmt.Errorf("midi: %q matches more than one port (available: %s)", name, available)
	}
	return nil, fmt.Errorf("midi: no output port called %q (available: %s)", name, available)
}

// String names the port, for logging.
func (l *LiveOutput) String() string {
	return l.port.String()
}

func (l *LiveOutput) Send(e event.Event) error {
	switch ev := e.(type) {
	case event.Note:
		key := [2]int{int(clampNibble(ev.Channel)), int(clampByte(ev.Pitch))}

		l.mu.Lock()
		defer l.mu.Unlock()
		if l.closed {
			return nil
		}
		l.channels[key[0]] = true
		if err := l.send(0x90|byte(key[0]), byte(key[1]), clampByte(ev.Velocity)); err != nil {
			return err
		}
		l.sounding[key]++
		time.AfterFunc(time.Duration(ev.DurationMs)*time.Millisecond, func() { l.release(key) })
		return nil

	case event.Control:
		l.mu.Lock()
		defer l.mu.Unlock()
		if l.closed {
			return nil
		}
		ch := clampNibble(ev.Channel)
		l.channels[int(ch)] = true
		return l.send(0xB0|ch, clampByte(ev.Controller), clampByte(int(ev.Value)))

	case event.Program:
		l.mu.Lock()
		defer l.mu.Unlock()
		if l.closed {
			return nil
		}
		ch := clampNibble(ev.Channel)
		if err := l.send(0xB0|ch, 0x00, clampByte(ev.Bank)); err != nil { // bank select
			return err
		}
		return l.send(0xC0|ch, clampByte(ev.Program))

	default:
		return fmt.Errorf("midi: unsupported event %T (only note/cc-output sounds can reach MIDI)", e)
	}
}

// release ends one of the notes held at key. If the same pitch was struck
// again on the same channel before this one ended, the note-off waits for
// the last of them, so a short note doesn't cut off a longer one.
func (l *LiveOutput) release(key [2]int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.sounding[key] == 0 {
		return
	}
	l.sounding[key]--
	if l.sounding[key] > 0 {
		return
	}
	delete(l.sounding, key)
	_ = l.send(0x80|byte(key[0]), byte(key[1]), 0)
}

// send must be called with mu held: the port isn't safe for concurrent use.
func (l *LiveOutput) send(msg ...byte) error {
	return l.port.Send(msg)
}

// Close ends any notes still sounding, so nothing is left hanging on the
// receiving instrument, and closes the port. It's safe to call more than
// once.
func (l *LiveOutput) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	for key := range l.sounding {
		_ = l.send(0x80|byte(key[0]), byte(key[1]), 0)
	}
	for ch := range l.channels {
		_ = l.send(0xB0|byte(ch), 123, 0) // all notes off
	}
	l.closed = true
	return l.driver.Close()
}
