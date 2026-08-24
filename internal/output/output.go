package output

import "fmt"

type Event interface {
	Describe() string
}

type NoteOn struct {
	Channel    int
	Note       int
	Velocity   int
	DurationMs int
}

func (e NoteOn) Describe() string {
	return fmt.Sprintf("NOTE channel=%d note=%d velocity=%d duration=%dms",
		e.Channel, e.Note, e.Velocity, e.DurationMs)
}

type CC struct {
	Channel    int
	Controller int
	Value      int
}

func (e CC) Describe() string {
	return fmt.Sprintf("CC channel=%d controller=%d value=%d",
		e.Channel, e.Controller, e.Value)
}

type Output interface {
	Send(Event) error
}

type Console struct{}

func NewConsole() *Console {
	return &Console{}
}

func (c *Console) Send(e Event) error {
	fmt.Println(e.Describe())
	return nil
}
