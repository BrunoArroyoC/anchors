package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
)

func sample(total, system uint64, preTotal, preSystem uint64, mem uint64, stats map[string]uint64) container.StatsResponse {
	var v container.StatsResponse
	v.CPUStats.CPUUsage.TotalUsage, v.CPUStats.SystemUsage, v.CPUStats.OnlineCPUs = total, system, 4
	v.PreCPUStats.CPUUsage.TotalUsage, v.PreCPUStats.SystemUsage = preTotal, preSystem
	v.MemoryStats.Usage, v.MemoryStats.Limit, v.MemoryStats.Stats = mem, 1000, stats
	return v
}

func TestToStats(t *testing.T) {
	first := toStats(sample(500, 10_000, 0, 0, 400, nil))
	if first.HasCPU {
		t.Errorf("the first sample has no earlier reading, so no CPU: %+v", first)
	}

	// 200 of 1000 units of all CPUs' time, on 4 CPUs: 80% of one core.
	st := toStats(sample(700, 11_000, 500, 10_000, 400, map[string]uint64{"inactive_file": 150}))
	if !st.HasCPU || math.Abs(st.CPUPercent-80) > 1e-9 {
		t.Errorf("cpu: got %+v, want 80%%", st)
	}
	if st.MemUsage != 250 || math.Abs(st.MemPercent-25) > 1e-9 {
		t.Errorf("memory without reclaimable cache: got %d (%.1f%%), want 250 (25%%)", st.MemUsage, st.MemPercent)
	}

	v1 := toStats(sample(700, 11_000, 500, 10_000, 400, map[string]uint64{"total_inactive_file": 100}))
	if v1.MemUsage != 300 {
		t.Errorf("cgroup v1 cache: got %d, want 300", v1.MemUsage)
	}
}

func TestStatsStream(t *testing.T) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.Encode(sample(500, 10_000, 0, 0, 400, nil))
	enc.Encode(sample(700, 11_000, 500, 10_000, 400, nil))

	ctx, cancel := context.WithCancel(context.Background())
	s := &StatsStream{ID: "c1", ch: make(chan Stats, 1), cancel: cancel}
	go s.pump(ctx, io.NopCloser(&b))

	var got []Stats
	for {
		m := s.Next()().(StatsMsg)
		if m.Done {
			if m.Err != nil {
				t.Fatal(m.Err)
			}
			break
		}
		got = append(got, m.Stats)
	}
	// The screen may miss the first sample (a newer one replaces it), never the last.
	if len(got) == 0 || !got[len(got)-1].HasCPU {
		t.Fatalf("got %+v", got)
	}
}

func TestStatsCloseEndsQuietly(t *testing.T) {
	pr, pw := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	s := &StatsStream{ID: "c1", ch: make(chan Stats, 1), cancel: cancel}
	go s.pump(ctx, pr)
	go json.NewEncoder(pw).Encode(sample(500, 10_000, 0, 0, 400, nil))
	if m := s.Next()().(StatsMsg); m.Done {
		t.Fatalf("wanted a sample first, got %+v", m)
	}
	s.Close()
	pw.CloseWithError(context.Canceled) // the Docker client closes the body when its context is cancelled
	done := make(chan StatsMsg, 1)
	go func() { done <- s.Next()().(StatsMsg) }()
	select {
	case m := <-done:
		if !m.Done || m.Err != nil {
			t.Fatalf("after Close: %+v (a closed stream ends quietly)", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Next never answered after Close")
	}
}
