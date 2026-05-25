package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseWindow(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    time.Duration
		wantErr bool
	}{
		{name: "one hour", value: "1h", want: time.Hour},
		{name: "one day", value: "24h", want: 24 * time.Hour},
		{name: "seven days", value: "7d", want: 7 * 24 * time.Hour},
		{name: "thirty days", value: "30d", want: 30 * 24 * time.Hour},
		{name: "invalid", value: "invalid", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseWindow(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseWindow(%q) expected error", tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseWindow(%q) returned error: %v", tt.value, err)
			}
			if got != tt.want {
				t.Fatalf("parseWindow(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestRenderReport(t *testing.T) {
	report := renderReport(pulseReport{
		GeneratedAt: time.Date(2026, 5, 9, 9, 32, 0, 0, time.UTC),
		WindowLabel: "24h",
		WindowStart: time.Date(2026, 5, 8, 9, 17, 0, 0, time.UTC),
		WindowEnd:   time.Date(2026, 5, 9, 9, 17, 0, 0, time.UTC),
		Database: databaseMetrics{
			ActivePaidMailboxes: 12,
			NewClaims:           4,
			Activated:           3,
			SupportVolume:       2,
			RenewalsInWindow:    1,
		},
		Polar: polarMetrics{
			SubscriptionsObserved: 11,
		},
		Admin: adminMetrics{
			ResolveCalls:        84,
			FailedKeyProofRatio: 0.125,
			Latency: map[string]int64{
				"p50_ms": 10,
				"p95_ms": 25,
				"p99_ms": 50,
			},
			TopErrors: []adminTopError{
				{Msg: "database exploded", Count: 2},
			},
		},
		Health: healthMetrics{
			P50:        100 * time.Millisecond,
			P95:        150 * time.Millisecond,
			P99:        150 * time.Millisecond,
			Non200:     1,
			ProbeCount: 5,
		},
	})

	for _, section := range []string{"## Headlines", "## Usage", "## System", "## Followups"} {
		if !strings.Contains(report, section) {
			t.Fatalf("report missing section %q:\n%s", section, report)
		}
	}

	for _, expected := range []string{
		"active_paid_mailboxes: 12",
		"claim_to_activation: 3/4 (75.0%)",
		"renewals_in_window: 1",
		"support_volume: 2",
		"resolve_calls_per_active_mailbox_per_week: 49.0",
		"failed_key_proof_ratio: 0.1250",
		"healthz p50/p95/p99 (CI-side, not prod-side): 100ms / 150ms / 150ms",
		"top_errors: database exploded (2)",
		"latency_p50_p95_p99: 10ms / 25ms / 50ms",
	} {
		if !strings.Contains(report, expected) {
			t.Fatalf("report missing %q:\n%s", expected, report)
		}
	}

	noDataMarker := "no data — source not connected (prod telemetry unreachable until Coroot/log-shipper/admin-metrics ships)"
	if got := strings.Count(report, noDataMarker); got != 2 {
		t.Fatalf("report has %d no-data markers, want 2:\n%s", got, report)
	}

	lines := strings.Split(strings.TrimSpace(report), "\n")
	if len(lines) < 30 || len(lines) > 40 {
		t.Fatalf("report line count = %d, want 30-40:\n%s", len(lines), report)
	}
}

func TestFetchAdminMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/metrics" {
			t.Fatalf("path = %q, want /admin/metrics", r.URL.Path)
		}
		if r.URL.Query().Get("window") != "24h" {
			t.Fatalf("window = %q, want 24h", r.URL.Query().Get("window"))
		}
		if got := r.Header.Get("Authorization"); got != "Bearer admin-secret" {
			t.Fatalf("Authorization = %q, want bearer admin-secret", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"resolve_calls": 12,
			"failed_key_proof_ratio": 0.25,
			"http_p50_ms": 10,
			"http_p95_ms": 25,
			"http_p99_ms": 50,
			"top_errors": [{"msg":"boom","count":3}]
		}`))
	}))
	defer server.Close()

	got, err := fetchAdminMetrics(context.Background(), server.Client(), server.URL, "admin-secret", "24h")
	if err != nil {
		t.Fatalf("fetchAdminMetrics returned error: %v", err)
	}
	if got.ResolveCalls != 12 {
		t.Fatalf("ResolveCalls = %d, want 12", got.ResolveCalls)
	}
	if got.FailedKeyProofRatio != 0.25 {
		t.Fatalf("FailedKeyProofRatio = %v, want 0.25", got.FailedKeyProofRatio)
	}
	if got.Latency["p50_ms"] != 10 || got.Latency["p95_ms"] != 25 || got.Latency["p99_ms"] != 50 {
		t.Fatalf("Latency = %#v, want p50=10 p95=25 p99=50", got.Latency)
	}
	if len(got.TopErrors) != 1 || got.TopErrors[0].Msg != "boom" || got.TopErrors[0].Count != 3 {
		t.Fatalf("TopErrors = %#v, want boom count 3", got.TopErrors)
	}
}
