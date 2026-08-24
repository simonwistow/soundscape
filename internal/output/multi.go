package output

import "example.com/fastly-soundscape/internal/event"

// Multi fans an event out to several backends, e.g. a theme that plays
// birds through a SoundFont while a river runs through the sample player.
// Each backend independently ignores event types it doesn't understand, so
// callers don't need to know which backend handles what.
type Multi struct {
	outputs []Output
}

func NewMulti(outputs ...Output) *Multi {
	return &Multi{outputs: outputs}
}

func (m *Multi) Send(e event.Event) error {
	var firstErr error
	handled := false
	for _, o := range m.outputs {
		if err := o.Send(e); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		handled = true
	}
	if handled || len(m.outputs) == 0 {
		return nil
	}
	return firstErr
}
