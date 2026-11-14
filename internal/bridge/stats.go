package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"docker-tool/internal/functions"
)

// Stats is one sample of a container's resource usage, worked out the way
// `docker stats` does it.
type Stats struct {
	At         time.Time
	HasCPU     bool    // false for the first sample: there is no earlier reading to compare with
	CPUPercent float64 // of one core: a container busy on two full cores shows 200
	MemUsage   uint64  // bytes, without page cache the kernel can take back
	MemLimit   uint64  // bytes; the host's memory when the container has no limit
	MemPercent float64
}

// StatsStream is an open stream of samples. The screen pulls them with
// Next, one StatsMsg at a time, and calls Close when it stops showing them.
type StatsStream struct {
	ID     string
	ch     chan Stats
	cancel context.CancelFunc
	err    error // why the stream ended; written before ch is closed
}

// StatsOpenedMsg answers OpenStats.
type StatsOpenedMsg struct {
	ID     string
	Stream *StatsStream // nil when Err is set
	Err    error
}

// StatsMsg is the next sample from a stream. Done means the stream ended
// (the container stopped, or the stream was closed); Err says why when it
// wasn't a normal end.
type StatsMsg struct {
	Stream *StatsStream
	Stats  Stats
	Done   bool
	Err    error
}

// OpenStats opens a container's live stats. The stream has no timeout; it
// lives until Close.
func OpenStats(cli *client.Client, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		out, err := functions.ContainerStats(ctx, cli, id)
		if err != nil {
			cancel()
			return StatsOpenedMsg{ID: id, Err: err}
		}
		s := &StatsStream{ID: id, ch: make(chan Stats, 1), cancel: cancel}
		go s.pump(ctx, out)
		return StatsOpenedMsg{ID: id, Stream: s}
	}
}

// Next waits for the next sample. Call it again after each StatsMsg until
// Done.
func (s *StatsStream) Next() tea.Cmd {
	return func() tea.Msg {
		st, ok := <-s.ch
		if !ok {
			return StatsMsg{Stream: s, Done: true, Err: s.err}
		}
		return StatsMsg{Stream: s, Stats: st}
	}
}

// Close stops the stream. A Next still waiting answers with Done.
func (s *StatsStream) Close() {
	if s.cancel != nil {
		s.cancel()
	}
}

// pump decodes samples until the stream ends or is closed. When the screen
// hasn't taken the last sample yet, the new one replaces it: an old reading
// is worth nothing once a newer one is here.
func (s *StatsStream) pump(ctx context.Context, r io.ReadCloser) {
	defer close(s.ch)
	defer r.Close()

	dec := json.NewDecoder(r)
	for {
		var v container.StatsResponse
		if err := dec.Decode(&v); err != nil {
			if ctx.Err() == nil && !errors.Is(err, io.EOF) {
				s.err = fmt.Errorf("read stats: %w", err)
			}
			return
		}
		st := toStats(v)
		select {
		case s.ch <- st:
			continue
		default:
		}
		select {
		case <-s.ch: // drop the sample nobody took
		default:
		}
		select {
		case s.ch <- st:
		case <-ctx.Done():
			return
		}
	}
}

// toStats works out the percentages from a raw sample.
func toStats(v container.StatsResponse) Stats {
	st := Stats{At: v.Read}

	// CPU: the container's share of the time all CPUs spent since the
	// previous sample, times the number of CPUs.
	cpu, pre := v.CPUStats, v.PreCPUStats
	if pre.SystemUsage > 0 && cpu.SystemUsage > pre.SystemUsage && cpu.CPUUsage.TotalUsage >= pre.CPUUsage.TotalUsage {
		cores := float64(cpu.OnlineCPUs)
		if cores == 0 {
			cores = float64(len(cpu.CPUUsage.PercpuUsage))
		}
		used := float64(cpu.CPUUsage.TotalUsage - pre.CPUUsage.TotalUsage)
		total := float64(cpu.SystemUsage - pre.SystemUsage)
		st.HasCPU = true
		st.CPUPercent = used / total * cores * 100
	}

	// Memory: like docker stats, page cache the kernel can reclaim doesn't
	// count as used. The counter is "inactive_file" on cgroup v2 and
	// "total_inactive_file" on cgroup v1.
	mem := v.MemoryStats
	used := mem.Usage
	for _, k := range []string{"inactive_file", "total_inactive_file"} {
		if c, ok := mem.Stats[k]; ok && c < used {
			used -= c
			break
		}
	}
	st.MemUsage, st.MemLimit = used, mem.Limit
	if mem.Limit > 0 {
		st.MemPercent = float64(used) / float64(mem.Limit) * 100
	}
	return st
}
