package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	paddle "github.com/PaddleHQ/paddle-go-sdk/v5"

	"github.com/atvirokodosprendimai/mailservice/internal/platform/database"
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
		Paddle: paddleMetrics{
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
		"paddle_subscriptions_observed: 11",
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

// unsetenvForTest clears an environment variable for the duration of the
// test and restores its prior value (if any) afterward, so tests can prove a
// var is genuinely optional regardless of what's set in the ambient shell.
func unsetenvForTest(t *testing.T, key string) {
	t.Helper()
	original, wasSet := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unsetenv %s: %v", key, err)
	}
	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv(key, original)
		}
	})
}

func setRequiredPulseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("TURSO_DATABASE_URL", "libsql://example.turso.io")
	t.Setenv("TURSO_AUTH_TOKEN", "turso-secret")
	t.Setenv("PADDLE_PULSE_TOKEN", "paddle-pulse-secret")
	t.Setenv("ADMIN_API_KEY", "admin-secret")
	unsetenvForTest(t, "PUBLIC_BASE_URL")
	unsetenvForTest(t, "PADDLE_PULSE_BASE_URL")
}

func TestLoadPulseConfigRequiresPaddlePulseToken(t *testing.T) {
	setRequiredPulseEnv(t)
	unsetenvForTest(t, "PADDLE_PULSE_TOKEN")

	_, err := loadPulseConfig()
	if err == nil {
		t.Fatal("loadPulseConfig() expected error for missing PADDLE_PULSE_TOKEN")
	}
	if !strings.Contains(err.Error(), "PADDLE_PULSE_TOKEN") {
		t.Fatalf("loadPulseConfig() error = %v, want it to name PADDLE_PULSE_TOKEN", err)
	}
}

// TestLoadPulseConfigDefaultsBaseURLToProduction is a regression check that
// loadPulseConfig's only hard requirement is PADDLE_PULSE_TOKEN, defaulting
// everything else sensibly.
func TestLoadPulseConfigDefaultsBaseURLToProduction(t *testing.T) {
	setRequiredPulseEnv(t)

	cfg, err := loadPulseConfig()
	if err != nil {
		t.Fatalf("loadPulseConfig() returned error: %v", err)
	}
	if cfg.PaddleBearer != "paddle-pulse-secret" {
		t.Fatalf("PaddleBearer = %q, want paddle-pulse-secret", cfg.PaddleBearer)
	}
	if cfg.PaddleBaseURL != paddle.ProductionBaseURL {
		t.Fatalf("PaddleBaseURL = %q, want default %q", cfg.PaddleBaseURL, paddle.ProductionBaseURL)
	}
}

// TestLoadPulseConfigBaseURLConfigurable closes the bug where the pulse Paddle
// client couldn't be pointed at a sandbox: PADDLE_PULSE_BASE_URL must override
// the production default.
func TestLoadPulseConfigBaseURLConfigurable(t *testing.T) {
	setRequiredPulseEnv(t)
	t.Setenv("PADDLE_PULSE_BASE_URL", "https://sandbox-api.paddle.com")

	cfg, err := loadPulseConfig()
	if err != nil {
		t.Fatalf("loadPulseConfig() returned error: %v", err)
	}
	if cfg.PaddleBaseURL != "https://sandbox-api.paddle.com" {
		t.Fatalf("PaddleBaseURL = %q, want sandbox override", cfg.PaddleBaseURL)
	}
}

func TestFetchPaddleSubscriptionsExplicitStatusFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/subscriptions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		gotStatuses := r.URL.Query()["status"]
		wantStatuses := []string{"active", "past_due", "trialing"}
		if len(gotStatuses) != len(wantStatuses) {
			t.Fatalf("status params = %v, want %v", gotStatuses, wantStatuses)
		}
		for i, want := range wantStatuses {
			if gotStatuses[i] != want {
				t.Fatalf("status params = %v, want %v", gotStatuses, wantStatuses)
			}
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer paddle-pulse-secret" {
			t.Fatalf("Authorization = %q, want bearer paddle-pulse-secret", auth)
		}

		w.Header().Set("Content-Type", "application/json")
		// estimated_total of 9 is deliberately higher than an active-only
		// count would be, so an implicit active-only default filter would
		// undercount and fail this assertion.
		_, _ = w.Write([]byte(`{"data":[],"meta":{"request_id":"req_1","pagination":{"per_page":50,"next":"","has_more":false,"estimated_total":9}}}`))
	}))
	defer server.Close()

	sdk, err := paddle.New("paddle-pulse-secret", paddle.WithBaseURL(server.URL), paddle.WithClient(server.Client()))
	if err != nil {
		t.Fatalf("paddle.New failed: %v", err)
	}

	got, err := fetchPaddleSubscriptions(context.Background(), sdk)
	if err != nil {
		t.Fatalf("fetchPaddleSubscriptions returned error: %v", err)
	}
	if got.SubscriptionsObserved != 9 {
		t.Fatalf("SubscriptionsObserved = %d, want 9", got.SubscriptionsObserved)
	}
}

// TestFetchPaddleSubscriptionsBaseURLConfigurable proves the pulse Paddle
// client can be pointed at an arbitrary base URL (standing in for sandbox vs.
// live) rather than a hardcoded endpoint.
func TestFetchPaddleSubscriptionsBaseURLConfigurable(t *testing.T) {
	var sawRequest bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawRequest = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"meta":{"request_id":"req_1","pagination":{"per_page":50,"next":"","has_more":false,"estimated_total":2}}}`))
	}))
	defer server.Close()

	sdk, err := paddle.New("paddle-pulse-secret", paddle.WithBaseURL(server.URL), paddle.WithClient(server.Client()))
	if err != nil {
		t.Fatalf("paddle.New failed: %v", err)
	}

	got, err := fetchPaddleSubscriptions(context.Background(), sdk)
	if err != nil {
		t.Fatalf("fetchPaddleSubscriptions returned error: %v", err)
	}
	if !sawRequest {
		t.Fatal("expected pulse's Paddle SDK to hit the configured base URL, saw no request")
	}
	if got.SubscriptionsObserved != 2 {
		t.Fatalf("SubscriptionsObserved = %d, want 2", got.SubscriptionsObserved)
	}
}

func TestFetchPaddleSubscriptionsRetriesRetryableStatusThenSucceeds(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"type":"api_error","code":"service_unavailable","detail":"try again"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"meta":{"request_id":"req_2","pagination":{"per_page":50,"next":"","has_more":false,"estimated_total":4}}}`))
	}))
	defer server.Close()

	sdk, err := paddle.New("paddle-pulse-secret", paddle.WithBaseURL(server.URL), paddle.WithClient(server.Client()))
	if err != nil {
		t.Fatalf("paddle.New failed: %v", err)
	}

	got, err := fetchPaddleSubscriptions(context.Background(), sdk)
	if err != nil {
		t.Fatalf("fetchPaddleSubscriptions returned error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if got.SubscriptionsObserved != 4 {
		t.Fatalf("SubscriptionsObserved = %d, want 4", got.SubscriptionsObserved)
	}
}

// TestFetchPaddleSubscriptionsRetriesTransportErrorThenSucceeds is a
// regression test for a real behavior bug: transport-level failures (timeout,
// connection reset, DNS failure) must retry unconditionally, since they never
// convert to a decoded API error with a status code to inspect. A prior
// version of fetchPaddleSubscriptionsAttempt required a successful
// errors.As(err, &apiErr) conversion to mark a failure retryable at all —
// but transport-level failures never convert to *paddleerr.Error, so they
// were silently treated as non-retryable, dropping exactly the failure class
// retries exist for on a scheduled runner. This proves retry is the default
// for non-API errors.
func TestFetchPaddleSubscriptionsRetriesTransportErrorThenSucceeds(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"meta":{"request_id":"req_3","pagination":{"per_page":50,"next":"","has_more":false,"estimated_total":6}}}`))
	}))
	defer server.Close()

	// A stub Do that fails on the first call only, then delegates to the real
	// server, so the transport error is transient rather than permanent.
	var doCalls int
	doer := transportErrThenServerDoer{server: server, failFirst: &doCalls}

	sdk, err := paddle.New("paddle-pulse-secret", paddle.WithBaseURL(server.URL), paddle.WithClient(doer))
	if err != nil {
		t.Fatalf("paddle.New failed: %v", err)
	}

	got, err := fetchPaddleSubscriptions(context.Background(), sdk)
	if err != nil {
		t.Fatalf("fetchPaddleSubscriptions returned error: %v", err)
	}
	if doCalls < 2 {
		t.Fatalf("doCalls = %d, want at least 2 (one failed transport attempt, one real request)", doCalls)
	}
	if attempts != 1 {
		t.Fatalf("attempts to real server = %d, want 1", attempts)
	}
	if got.SubscriptionsObserved != 6 {
		t.Fatalf("SubscriptionsObserved = %d, want 6", got.SubscriptionsObserved)
	}
}

// transportErrThenServerDoer implements client.HTTPDoer structurally
// (Do(req) (*http.Response, error)) and fails the first call with a raw
// transport-level error — no HTTP response at all, as with a connection
// reset or DNS failure — before delegating to the real test server. The
// Paddle SDK's HandleError passes such errors through unchanged (see
// internal/response/response_api_error_handler.go in the SDK module), so
// they never convert to *paddleerr.Error.
type transportErrThenServerDoer struct {
	server    *httptest.Server
	failFirst *int
}

func (d transportErrThenServerDoer) Do(req *http.Request) (*http.Response, error) {
	*d.failFirst++
	if *d.failFirst == 1 {
		return nil, errors.New("connection reset by peer")
	}
	return d.server.Client().Do(req)
}

// TestFetchPaddleSubscriptionsDoesNotRetryNonRetryableAPIError confirms the
// narrowing side of the inverted retry logic: a decoded Paddle API error with
// a non-retryable status (e.g. 400) must NOT retry, even though the default
// for unrecognized errors is now retryable.
func TestFetchPaddleSubscriptionsDoesNotRetryNonRetryableAPIError(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"type":"request_error","code":"bad_request","detail":"nope"}}`))
	}))
	defer server.Close()

	sdk, err := paddle.New("paddle-pulse-secret", paddle.WithBaseURL(server.URL), paddle.WithClient(server.Client()))
	if err != nil {
		t.Fatalf("paddle.New failed: %v", err)
	}

	_, err = fetchPaddleSubscriptions(context.Background(), sdk)
	if err == nil {
		t.Fatal("fetchPaddleSubscriptions expected error for 400 response")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 (non-retryable 400 must not retry)", attempts)
	}
}

// TestProbeHealthzRegression confirms probeHealthz's shape is unaffected by
// the pulse Paddle migration.
func TestProbeHealthzRegression(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	got, err := probeHealthz(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatalf("probeHealthz returned error: %v", err)
	}
	if got.ProbeCount != healthProbeSamples {
		t.Fatalf("ProbeCount = %d, want %d", got.ProbeCount, healthProbeSamples)
	}
	if got.Non200 != 0 {
		t.Fatalf("Non200 = %d, want 0", got.Non200)
	}
}

// TestCollectDatabaseMetricsRegression confirms collectDatabaseMetrics'
// query shape is unaffected by the pulse Paddle migration, running it against
// a fully migrated sqlite database rather than a mock.
func TestCollectDatabaseMetricsRegression(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "pulse-regression.db")
	db, err := database.OpenAndMigrate(dsn)
	if err != nil {
		t.Fatalf("OpenAndMigrate failed: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	before := start.Add(-30 * 24 * time.Hour)
	within := start.Add(12 * time.Hour)

	insertMailbox(t, sqlDB, "mbx-active-old", "active", before, nil)
	insertMailbox(t, sqlDB, "mbx-new-active", "active", within, nil)
	insertMailbox(t, sqlDB, "mbx-new-pending", "pending_payment", within, nil)
	insertMailbox(t, sqlDB, "mbx-renewal", "active", before, &within)

	if _, err := sqlDB.Exec(`INSERT INTO support_messages (id, mailbox_id, key_fingerprint, subject, body, created_at)
		VALUES ('sup-1', 'mbx-active-old', 'fp-1', 'help', 'body', ?)`, within); err != nil {
		t.Fatalf("insert support message failed: %v", err)
	}

	metrics, err := collectDatabaseMetrics(context.Background(), sqlDB, start, end)
	if err != nil {
		t.Fatalf("collectDatabaseMetrics returned error: %v", err)
	}
	if metrics.ActivePaidMailboxes != 3 {
		t.Fatalf("ActivePaidMailboxes = %d, want 3", metrics.ActivePaidMailboxes)
	}
	if metrics.NewClaims != 2 {
		t.Fatalf("NewClaims = %d, want 2", metrics.NewClaims)
	}
	if metrics.Activated != 1 {
		t.Fatalf("Activated = %d, want 1", metrics.Activated)
	}
	if metrics.SupportVolume != 1 {
		t.Fatalf("SupportVolume = %d, want 1", metrics.SupportVolume)
	}
	if metrics.RenewalsInWindow != 1 {
		t.Fatalf("RenewalsInWindow = %d, want 1", metrics.RenewalsInWindow)
	}
}

func insertMailbox(t *testing.T, db *sql.DB, id string, status string, createdAt time.Time, paidAt *time.Time) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO mailboxes (
		id, owner_email, imap_host, imap_port, imap_username,
		imap_password, access_token, status, created_at, paid_at
	) VALUES (?, ?, 'imap.example.com', 143, ?, 'secret', ?, ?, ?, ?)`,
		id, id+"@example.com", id, id+"-token", status, createdAt, paidAt)
	if err != nil {
		t.Fatalf("insert mailbox %s failed: %v", id, err)
	}
}
