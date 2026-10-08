package midi

import (
	"fmt"
	"time"

	"github.com/simonwistow/soundscape/internal/event"
)

// VirtualOutput is an output.Output backed by a Writer: it captures a
// session's note/control events and, on Close, writes them out as a
// Standard MIDI File at path. LiveOutput is the real-time equivalent.
type VirtualOutput struct {
	writer *Writer
	path   string
}

func NewVirtualOutput(path string) *VirtualOutput {
	return &VirtualOutput{writer: NewWriter(), path: path}
}

func (v *VirtualOutput) Send(e event.Event) error {
	switch ev := e.(type) {
	case event.Note:
		v.writer.NoteOn(ev.Channel, ev.Pitch, ev.Velocity)
		go func() {
			time.Sleep(time.Duration(ev.DurationMs) * time.Millisecond)
			v.writer.NoteOff(ev.Channel, ev.Pitch)
		}()
		return nil

	case event.Control:
		v.writer.ControlChange(ev.Channel, ev.Controller, int(ev.Value))
		return nil

	case event.Program:
		v.writer.ProgramChange(ev.Channel, ev.Bank, ev.Program)
		return nil

	default:
		return fmt.Errorf("midi: unsupported event %T (only note/cc-output sounds can reach MIDI)", e)
	}
}

// Close finalizes the MIDI file. It should be called once, after the
// session is done producing events.
func (v *VirtualOutput) Close() error {
	return v.writer.WriteFile(v.path)
}
