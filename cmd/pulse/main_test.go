package main

import (
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
		"healthz p50/p95/p99 (CI-side, not prod-side): 100ms / 150ms / 150ms",
	} {
		if !strings.Contains(report, expected) {
			t.Fatalf("report missing %q:\n%s", expected, report)
		}
	}

	noDataMarker := "no data — source not connected (prod telemetry unreachable until Coroot/log-shipper/admin-metrics ships)"
	if got := strings.Count(report, noDataMarker); got < 6 {
		t.Fatalf("report has %d no-data markers, want at least 6:\n%s", got, report)
	}

	lines := strings.Split(strings.TrimSpace(report), "\n")
	if len(lines) < 30 || len(lines) > 40 {
		t.Fatalf("report line count = %d, want 30-40:\n%s", len(lines), report)
	}
}
