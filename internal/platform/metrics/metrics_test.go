package metrics

import (
	"fmt"
	"testing"
	"time"
)

func TestCounterAddAndSum24h(t *testing.T) {
	counter := NewCounter()
	counter.Add(2)
	counter.Add(5)

	if got := counter.Sum24h(); got != 7 {
		t.Fatalf("Sum24h = %d, want 7", got)
	}

	base := time.Now().UTC()
	for i := 0; i < 24; i++ {
		counter.rotate(base.Add(time.Duration(i+1) * time.Hour))
	}

	if got := counter.Sum24h(); got != 0 {
		t.Fatalf("Sum24h after 24 hourly rotations = %d, want 0", got)
	}
}

func TestHistogramPercentile(t *testing.T) {
	histogram := NewHistogram()
	for _, value := range []int64{1, 5, 10, 25, 50} {
		histogram.Observe(value)
	}

	if got := histogram.Percentile(0.50); got != 10 {
		t.Fatalf("p50 = %d, want 10", got)
	}
	if got := histogram.Percentile(0.95); got != 45 {
		t.Fatalf("p95 = %d, want 45", got)
	}
	if got := histogram.Percentile(0.99); got != 49 {
		t.Fatalf("p99 = %d, want 49", got)
	}
}

func TestTopNOrderingAndLRUEviction(t *testing.T) {
	top := NewTopN()
	top.Inc("first")
	top.Inc("first")
	top.Inc("second")
	top.Inc("second")
	top.Inc("second")

	for i := 0; i < 15; i++ {
		top.Inc(fmt.Sprintf("key-%02d", i))
	}

	snapshot := top.Snapshot()
	if len(snapshot) != 16 {
		t.Fatalf("Snapshot length = %d, want 16", len(snapshot))
	}
	if snapshot[0].Msg != "second" || snapshot[0].Count != 3 {
		t.Fatalf("first snapshot entry = %#v, want second count 3", snapshot[0])
	}
	for _, entry := range snapshot {
		if entry.Msg == "first" {
			t.Fatalf("least recently incremented entry was not evicted: %#v", snapshot)
		}
	}
}

func TestRegistryNilSafety(t *testing.T) {
	var registry *Registry

	registry.Counter("resolve_calls").Add(3)
	registry.Histogram("http_latency_ms").Observe(25)
	registry.TopN("top_errors").Inc("boom")

	snapshot := registry.Snapshot("24h")
	if snapshot["window"] != "24h" {
		t.Fatalf("window = %#v, want 24h", snapshot["window"])
	}
	if snapshot["resolve_calls"] != int64(0) {
		t.Fatalf("resolve_calls = %#v, want 0", snapshot["resolve_calls"])
	}
}
