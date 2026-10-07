// Package output defines the sound-output abstraction: something that can
// realise abstract event.Event values as audio, MIDI, or anything else. The
// theme engine only ever depends on this interface, never on a concrete
// backend.
package output

import (
	"fmt"

	"github.com/simonwistow/soundscape/internal/event"
)

type Output interface {
	Send(event.Event) error
}

// Console is a development backend that just prints events, so a theme can
// be exercised with no audio hardware at all.
type Console struct{}

func NewConsole() *Console {
	return &Console{}
}

func (c *Console) Send(e event.Event) error {
	fmt.Println(e.Describe())
	return nil
}
