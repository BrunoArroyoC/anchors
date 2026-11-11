package bridge

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"testing"
	"time"
)

// frame is one chunk of Docker's multiplexed log stream.
func frame(stream byte, s string) []byte {
	h := make([]byte, 8)
	h[0] = stream
	binary.BigEndian.PutUint32(h[4:], uint32(len(s)))
	return append(h, s...)
}

func stream(r io.ReadCloser) *LogStream {
	ctx, cancel := context.WithCancel(context.Background())
	s := &LogStream{ID: "c1", lines: make(chan LogLine, logBatch), cancel: cancel}
	go s.pump(ctx, r)
	return s
}

// drain reads the stream to its end, like the screen does.
func drain(t *testing.T, s *LogStream) ([]LogLine, error) {
	t.Helper()
	var got []LogLine
	for {
		done := make(chan LogsMsg, 1)
		go func() { done <- s.Next()().(LogsMsg) }()
		select {
		case m := <-done:
			got = append(got, m.Lines...)
			if m.Done {
				return got, m.Err
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Next never answered")
		}
	}
}

func TestPumpMultiplexed(t *testing.T) {
	var b bytes.Buffer
	b.Write(frame(1, "listening on :80\r\nGET / 2"))
	b.Write(frame(2, "warn: slow\n"))
	b.Write(frame(1, "00\nlast line, no newline"))
	got, err := drain(t, stream(io.NopCloser(&b)))
	if err != nil {
		t.Fatal(err)
	}
	want := []LogLine{
		{"listening on :80", false},
		{"warn: slow", true},
		{"GET / 200", false},
		{"last line, no newline", false},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestPumpTTY(t *testing.T) {
	got, err := drain(t, stream(io.NopCloser(bytes.NewBufferString("hello\nworld\n"))))
	if err != nil || len(got) != 2 || got[0].Text != "hello" || got[1].Text != "world" || got[0].Stderr {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestCloseEndsAFollowedStream(t *testing.T) {
	pr, pw := io.Pipe()
	s := stream(pr)
	go func() {
		pw.Write(frame(1, "tick\n"))
		<-time.After(5 * time.Second) // a container that keeps running
	}()
	m := s.Next()().(LogsMsg)
	if len(m.Lines) != 1 || m.Lines[0].Text != "tick" {
		t.Fatalf("got %+v", m)
	}
	s.Close()
	pw.CloseWithError(context.Canceled) // the Docker client closes the body when its context is cancelled
	got, err := drain(t, s)
	if err != nil || len(got) != 0 {
		t.Fatalf("after Close: %+v, %v (a closed stream ends quietly)", got, err)
	}
}

func TestBrokenStreamReportsWhy(t *testing.T) {
	pr, pw := io.Pipe()
	s := stream(pr)
	go func() {
		pw.Write(frame(1, "a\n"))
		pw.CloseWithError(io.ErrUnexpectedEOF)
	}()
	_, err := drain(t, s)
	if err == nil {
		t.Fatal("a broken stream should say why it ended")
	}
}
