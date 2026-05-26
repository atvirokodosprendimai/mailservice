package metrics

import (
	"container/list"
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	counterBucketCount = 24
	topNMaxEntries     = 16
	maxTopNMessageLen  = 200
)

var histogramBounds = [...]int64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

type Counter struct {
	mu       sync.Mutex
	buckets  [counterBucketCount]atomic.Int64
	current  atomic.Int64
	lastHour atomic.Int64
}

func NewCounter() *Counter {
	counter := &Counter{}
	hour := time.Now().UTC().Unix() / int64(time.Hour/time.Second)
	counter.lastHour.Store(hour)
	counter.current.Store(hour % counterBucketCount)
	return counter
}

func (c *Counter) Add(delta int64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	index := c.current.Load()
	if index < 0 || index >= counterBucketCount {
		index = 0
	}
	c.buckets[index].Add(delta)
}

func (c *Counter) Sum24h() int64 {
	if c == nil {
		return 0
	}
	var total int64
	for i := range c.buckets {
		total += c.buckets[i].Load()
	}
	return total
}

func (c *Counter) rotate(now time.Time) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	nextHour := now.UTC().Unix() / int64(time.Hour/time.Second)
	lastHour := c.lastHour.Load()
	if lastHour == 0 {
		c.lastHour.Store(nextHour)
		c.current.Store(nextHour % counterBucketCount)
		return
	}
	if nextHour <= lastHour {
		return
	}

	elapsed := nextHour - lastHour
	if elapsed > counterBucketCount {
		elapsed = counterBucketCount
	}
	for i := int64(1); i <= elapsed; i++ {
		c.buckets[(lastHour+i)%counterBucketCount].Store(0)
	}
	c.lastHour.Store(nextHour)
	c.current.Store(nextHour % counterBucketCount)
}

type Histogram struct {
	buckets [len(histogramBounds) + 1]atomic.Int64
}

func NewHistogram() *Histogram {
	return &Histogram{}
}

func (h *Histogram) Observe(ms int64) {
	if h == nil {
		return
	}
	if ms < 0 {
		ms = 0
	}
	for i, bound := range histogramBounds {
		if ms <= bound {
			h.buckets[i].Add(1)
			return
		}
	}
	h.buckets[len(histogramBounds)].Add(1)
}

func (h *Histogram) Percentile(p float64) int64 {
	if h == nil {
		return 0
	}
	counts := h.snapshotCounts()
	total := int64(0)
	for _, count := range counts {
		total += count
	}
	if total == 0 {
		return 0
	}

	fraction := p
	if fraction > 1 {
		fraction = fraction / 100
	}
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}

	position := fraction * float64(total-1)
	lowerIndex := int64(math.Floor(position))
	upperIndex := int64(math.Ceil(position))
	lowerValue := h.valueAt(counts, lowerIndex)
	upperValue := h.valueAt(counts, upperIndex)
	if lowerIndex == upperIndex {
		return lowerValue
	}
	weight := position - math.Floor(position)
	return int64(math.Round(float64(lowerValue)*(1-weight) + float64(upperValue)*weight))
}

func (h *Histogram) snapshotCounts() []int64 {
	counts := make([]int64, len(h.buckets))
	for i := range h.buckets {
		counts[i] = h.buckets[i].Load()
	}
	return counts
}

func (h *Histogram) valueAt(counts []int64, index int64) int64 {
	var seen int64
	for i, count := range counts {
		if count <= 0 {
			continue
		}
		seen += count
		if index < seen {
			if i < len(histogramBounds) {
				return histogramBounds[i]
			}
			return histogramBounds[len(histogramBounds)-1]
		}
	}
	return 0
}

type TopNEntry struct {
	Msg   string `json:"msg"`
	Count int64  `json:"count"`
}

type TopN struct {
	mu      sync.Mutex
	entries map[string]*topNItem
	order   *list.List
}

type topNItem struct {
	msg     string
	count   int64
	element *list.Element
}

func NewTopN() *TopN {
	return &TopN{
		entries: map[string]*topNItem{},
		order:   list.New(),
	}
}

func (t *TopN) Inc(msg string) {
	if t == nil {
		return
	}
	msg = truncateMessage(strings.TrimSpace(msg))
	if msg == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.ensureInitialized()
	if item, ok := t.entries[msg]; ok {
		item.count++
		t.order.MoveToBack(item.element)
		return
	}

	item := &topNItem{msg: msg, count: 1}
	item.element = t.order.PushBack(item)
	t.entries[msg] = item
	if len(t.entries) <= topNMaxEntries {
		return
	}

	oldest := t.order.Front()
	if oldest == nil {
		return
	}
	evicted := oldest.Value.(*topNItem)
	delete(t.entries, evicted.msg)
	t.order.Remove(oldest)
}

func (t *TopN) Snapshot() []TopNEntry {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	entries := make([]TopNEntry, 0, len(t.entries))
	for _, item := range t.entries {
		entries = append(entries, TopNEntry{Msg: item.msg, Count: item.count})
	}
	sort.Slice(entries, func(i int, j int) bool {
		if entries[i].Count == entries[j].Count {
			return entries[i].Msg < entries[j].Msg
		}
		return entries[i].Count > entries[j].Count
	})
	return entries
}

func (t *TopN) ensureInitialized() {
	if t.entries == nil {
		t.entries = map[string]*topNItem{}
	}
	if t.order == nil {
		t.order = list.New()
	}
}

func truncateMessage(msg string) string {
	runes := []rune(msg)
	if len(runes) <= maxTopNMessageLen {
		return msg
	}
	return string(runes[:maxTopNMessageLen])
}

type Registry struct {
	mu         sync.Mutex
	counters   map[string]*Counter
	histograms map[string]*Histogram
	topN       map[string]*TopN
}

func NewRegistry(ctx context.Context) *Registry {
	registry := &Registry{
		counters:   map[string]*Counter{},
		histograms: map[string]*Histogram{},
		topN:       map[string]*TopN{},
	}
	go registry.rotateCounters(ctx)
	return registry
}

func (r *Registry) Counter(name string) *Counter {
	if r == nil {
		return &Counter{}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return &Counter{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.counters == nil {
		r.counters = map[string]*Counter{}
	}
	counter, ok := r.counters[name]
	if !ok {
		counter = NewCounter()
		r.counters[name] = counter
	}
	return counter
}

func (r *Registry) Histogram(name string) *Histogram {
	if r == nil {
		return &Histogram{}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return &Histogram{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.histograms == nil {
		r.histograms = map[string]*Histogram{}
	}
	histogram, ok := r.histograms[name]
	if !ok {
		histogram = NewHistogram()
		r.histograms[name] = histogram
	}
	return histogram
}

func (r *Registry) TopN(name string) *TopN {
	if r == nil {
		return &TopN{}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return &TopN{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.topN == nil {
		r.topN = map[string]*TopN{}
	}
	top, ok := r.topN[name]
	if !ok {
		top = NewTopN()
		r.topN[name] = top
	}
	return top
}

func (r *Registry) Snapshot(window string) map[string]any {
	window = strings.TrimSpace(window)
	if window == "" {
		window = "24h"
	}
	if r == nil {
		return map[string]any{
			"window":           window,
			"imap_login":       int64(0),
			"resolve_calls":    int64(0),
			"key_proof_total":  int64(0),
			"key_proof_failed": int64(0),
			"http_p50_ms":      int64(0),
			"http_p95_ms":      int64(0),
			"http_p99_ms":      int64(0),
			"top_errors":       []TopNEntry{},
		}
	}

	latency := r.Histogram("http_latency_ms")
	return map[string]any{
		"window":           window,
		"imap_login":       r.Counter("imap_login").Sum24h(),
		"resolve_calls":    r.Counter("resolve_calls").Sum24h(),
		"key_proof_total":  r.Counter("key_proof_total").Sum24h(),
		"key_proof_failed": r.Counter("key_proof_failed").Sum24h(),
		"http_p50_ms":      latency.Percentile(0.50),
		"http_p95_ms":      latency.Percentile(0.95),
		"http_p99_ms":      latency.Percentile(0.99),
		"top_errors":       r.TopN("top_errors").Snapshot(),
	}
}

func (r *Registry) rotateCounters(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			r.rotateAll(now)
		}
	}
}

func (r *Registry) rotateAll(now time.Time) {
	r.mu.Lock()
	counters := make([]*Counter, 0, len(r.counters))
	for _, counter := range r.counters {
		counters = append(counters, counter)
	}
	r.mu.Unlock()

	for _, counter := range counters {
		counter.rotate(now)
	}
}
