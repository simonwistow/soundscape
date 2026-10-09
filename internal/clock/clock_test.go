package clock

import (
	"reflect"
	"testing"
	"time"
)

func TestVirtualFiresInOrder(t *testing.T) {
	start := time.Unix(1000, 0)
	v := NewVirtual(start)
	var got []string
	at := func(name string) func() {
		return func() { got = append(got, name+"@"+v.Now().Sub(start).String()) }
	}
	v.AfterFunc(3*time.Second, at("c"))
	v.AfterFunc(1*time.Second, at("a"))
	v.AfterFunc(2*time.Second, at("b1"))
	v.AfterFunc(2*time.Second, at("b2"))
	v.AfterFunc(10*time.Second, at("late"))

	v.Advance(start.Add(5 * time.Second))

	want := []string{"a@1s", "b1@2s", "b2@2s", "c@3s"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("fired %v, want %v", got, want)
	}
	if now := v.Now(); !now.Equal(start.Add(5 * time.Second)) {
		t.Errorf("Now() = %v after Advance, want start+5s", now.Sub(start))
	}
}

func TestVirtualRunsWhatTimersSchedule(t *testing.T) {
	start := time.Unix(0, 0)
	v := NewVirtual(start)
	var got []time.Duration
	v.AfterFunc(time.Second, func() {
		got = append(got, v.Now().Sub(start))
		v.AfterFunc(time.Second, func() { got = append(got, v.Now().Sub(start)) })
		v.AfterFunc(time.Minute, func() { got = append(got, v.Now().Sub(start)) })
	})

	v.Advance(start.Add(5 * time.Second))

	if want := []time.Duration{time.Second, 2 * time.Second}; !reflect.DeepEqual(got, want) {
		t.Errorf("fired at %v, want %v (and not the one a minute out)", got, want)
	}
}

func TestVirtualNeverGoesBack(t *testing.T) {
	start := time.Unix(100, 0)
	v := NewVirtual(start)
	v.Advance(start.Add(-time.Second))
	if !v.Now().Equal(start) {
		t.Errorf("Advance into the past moved the clock to %v", v.Now())
	}
	fired := false
	v.AfterFunc(-time.Second, func() { fired = true })
	v.Advance(start)
	if !fired {
		t.Error("a timer for the past didn't fire at the next Advance")
	}
}
