package bridge

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"

	"docker-tool/internal/functions"
)

const (
	logBatch   = 256  // at most this many lines per LogsMsg, so a chatty container doesn't cost one frame per line
	maxLogLine = 8192 // bytes; a longer line is cut into pieces
)

// LogLine is one line of a container's output.
type LogLine struct {
	Text   string
	Stderr bool
}

// LogStream is an open, followed log stream. The screen pulls lines with
// Next, one LogsMsg at a time, and calls Close when it stops showing them.
type LogStream struct {
	ID     string
	lines  chan LogLine
	cancel context.CancelFunc
	err    error // why the stream ended; written before lines is closed
}

// LogsOpenedMsg answers OpenLogs.
type LogsOpenedMsg struct {
	ID     string
	Stream *LogStream // nil when Err is set
	Err    error
}

// LogsMsg is the next batch of lines from a stream. Done means the stream
// ended (the container stopped, or the stream was closed); Err says why when
// it wasn't a normal end.
type LogsMsg struct {
	Stream *LogStream
	Lines  []LogLine
	Done   bool
	Err    error
}

// OpenLogs opens a container's log stream: its last lines, then everything
// it prints from now on. The stream has no timeout; it lives until Close.
func OpenLogs(cli *client.Client, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		out, err := functions.ContainerLogs(ctx, cli, id)
		if err != nil {
			cancel()
			return LogsOpenedMsg{ID: id, Err: err}
		}
		s := &LogStream{ID: id, lines: make(chan LogLine, logBatch), cancel: cancel}
		go s.pump(ctx, out)
		return LogsOpenedMsg{ID: id, Stream: s}
	}
}

// Next waits for the next lines. Call it again after each LogsMsg until Done.
func (s *LogStream) Next() tea.Cmd {
	return func() tea.Msg {
		l, ok := <-s.lines
		if !ok {
			return LogsMsg{Stream: s, Done: true, Err: s.err}
		}
		batch := []LogLine{l}
		for len(batch) < logBatch {
			select {
			case l, ok := <-s.lines:
				if !ok {
					return LogsMsg{Stream: s, Lines: batch, Done: true, Err: s.err}
				}
				batch = append(batch, l)
			default:
				return LogsMsg{Stream: s, Lines: batch}
			}
		}
		return LogsMsg{Stream: s, Lines: batch}
	}
}

// Close stops the stream. A Next still waiting answers with Done.
func (s *LogStream) Close() {
	if s.cancel != nil {
		s.cancel()
	}
}

// pump reads the stream into lines until it ends or is closed.
func (s *LogStream) pump(ctx context.Context, out io.ReadCloser) {
	defer close(s.lines)
	defer out.Close()

	stdout := &lineWriter{ctx: ctx, ch: s.lines}
	stderr := &lineWriter{ctx: ctx, ch: s.lines, stderr: true}
	br := bufio.NewReader(out)
	var err error
	if multiplexed(br) {
		_, err = stdcopy.StdCopy(stdout, stderr, br)
	} else {
		_, err = io.Copy(stdout, br) // a container with a TTY sends plain text
	}
	stdout.flush()
	stderr.flush()
	if err != nil && ctx.Err() == nil {
		s.err = fmt.Errorf("read logs: %w", err)
	}
}

// multiplexed reports whether the stream uses Docker's stdout/stderr framing.
// Each frame starts with the stream number (0, 1 or 2); text from a TTY
// container practically never starts with those bytes.
func multiplexed(br *bufio.Reader) bool {
	b, err := br.Peek(1)
	return err == nil && b[0] <= 2
}

// lineWriter cuts what it is given into lines and sends each one.
type lineWriter struct {
	ctx    context.Context
	ch     chan<- LogLine
	stderr bool
	buf    []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		if err := w.send(w.buf[:i]); err != nil {
			return 0, err
		}
		w.buf = w.buf[i+1:]
	}
	if len(w.buf) > maxLogLine {
		if err := w.send(w.buf); err != nil {
			return 0, err
		}
		w.buf = nil
	}
	return len(p), nil
}

// flush sends a last line that had no newline.
func (w *lineWriter) flush() {
	if len(w.buf) > 0 {
		_ = w.send(w.buf)
		w.buf = nil
	}
}

func (w *lineWriter) send(b []byte) error {
	select {
	case w.ch <- LogLine{Text: string(bytes.TrimRight(b, "\r")), Stderr: w.stderr}:
		return nil
	case <-w.ctx.Done():
		return w.ctx.Err()
	}
}
