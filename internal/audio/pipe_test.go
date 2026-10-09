package audio

import (
	"io"
	"testing"
	"time"
)

func TestPipeWriteBlocksWhenFull(t *testing.T) {
	p := NewPipe(8)
	if _, err := p.Write(make([]byte, 8)); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		p.Write(make([]byte, 4))
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Write returned while the pipe was full")
	case <-time.After(50 * time.Millisecond):
	}

	if _, err := io.ReadFull(p, make([]byte, 4)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Write still blocked after a Read made room")
	}
}

func TestPipeAcceptsOversizedWriteWhenEmpty(t *testing.T) {
	p := NewPipe(4)
	if n, err := p.Write(make([]byte, 16)); n != 16 || err != nil {
		t.Fatalf("Write = %d, %v; want 16, nil", n, err)
	}
}

func TestPipeCloseUnblocksWriter(t *testing.T) {
	p := NewPipe(4)
	p.Write(make([]byte, 4))

	errc := make(chan error)
	go func() {
		_, err := p.Write(make([]byte, 4))
		errc <- err
	}()
	time.Sleep(20 * time.Millisecond)
	p.Close()

	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("Write after Close returned nil error")
		}
	case <-time.After(time.Second):
		t.Fatal("Close didn't unblock a waiting Write")
	}
}

func TestPipePreservesOrder(t *testing.T) {
	p := NewPipe(4)
	go func() {
		for i := 0; i < 100; i++ {
			p.Write([]byte{byte(i)})
		}
	}()
	b := make([]byte, 1)
	for i := 0; i < 100; i++ {
		if _, err := io.ReadFull(p, b); err != nil {
			t.Fatal(err)
		}
		if b[0] != byte(i) {
			t.Fatalf("byte %d = %d", i, b[0])
		}
	}
}
