package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/platform/database"
)

const (
	defaultWindow        = "24h"
	defaultOutputDir     = "docs/pulse-reports/"
	defaultPublicBaseURL = "https://truevipaccess.com"

	polarSubscriptionsURL = "https://api.polar.sh/v1/subscriptions"
	pendingMetricNote     = "no data — source not connected (prod telemetry unreachable until Coroot/log-shipper/admin-metrics ships)"

	httpClientTimeout      = 10 * time.Second
	queryTrailingBuffer    = 15 * time.Minute
	polarRetryBackoff      = 1 * time.Second
	polarMaxAttempts       = 3
	healthProbeSamples     = 5
	dayHours               = 24
	percentileScale        = 100
	claimConversionDecimal = 100
)

type pulseConfig struct {
	DatabaseURL   string
	DatabaseAuth  string
	PolarBearer   string
	PolarOrgID    string
	PublicBaseURL string
	AdminAPIKey   string
}

type databaseMetrics struct {
	ActivePaidMailboxes int64
	NewClaims           int64
	Activated           int64
	SupportVolume       int64
	RenewalsInWindow    int64
}

type polarMetrics struct {
	SubscriptionsObserved int
}

type adminTopError struct {
	Msg   string `json:"msg"`
	Count int64  `json:"count"`
}

type adminMetrics struct {
	ImapLogin                   int64 `json:"imap_login"`
	ImapLoginAvailable          bool  `json:"-"`
	ImapMessageFetched          int64 `json:"imap_message_fetched"`
	ImapMessageFetchedAvailable bool  `json:"-"`
	ResolveCalls                int64
	FailedKeyProofRatio         float64
	Latency                     map[string]int64
	TopErrors                   []adminTopError
}

type healthMetrics struct {
	P50        time.Duration
	P95        time.Duration
	P99        time.Duration
	Non200     int
	ProbeCount int
}

type pulseReport struct {
	GeneratedAt time.Time
	WindowLabel string
	WindowStart time.Time
	WindowEnd   time.Time
	Database    databaseMetrics
	Polar       polarMetrics
	Admin       adminMetrics
	Health      healthMetrics
}

func main() {
	if err := run(context.Background(), os.Args[1:], log.Default()); err != nil {
		log.Fatalf("pulse: %v", err)
	}
}

func run(ctx context.Context, args []string, logger *log.Logger) error {
	fs := flag.NewFlagSet("pulse", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	windowFlag := fs.String("window", defaultWindow, "lookback window, for example 24h or 7d")
	outFlag := fs.String("out", defaultOutputDir, "output directory for markdown reports")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	window, err := parseWindow(*windowFlag)
	if err != nil {
		return fmt.Errorf("parse window: %w", err)
	}

	cfg, err := loadPulseConfig()
	if err != nil {
		return err
	}

	db, err := database.OpenTurso(cfg.DatabaseURL, cfg.DatabaseAuth)
	if err != nil {
		return fmt.Errorf("open turso: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("database handle: %w", err)
	}
	defer sqlDB.Close()

	now := time.Now().UTC()
	windowEnd := now.Add(-queryTrailingBuffer)
	windowStart := windowEnd.Add(-window)

	dbMetrics, err := collectDatabaseMetrics(ctx, sqlDB, windowStart, windowEnd)
	if err != nil {
		return fmt.Errorf("collect database metrics: %w", err)
	}

	client := &http.Client{Timeout: httpClientTimeout}
	polar, err := fetchPolarSubscriptions(ctx, client, cfg.PolarBearer, cfg.PolarOrgID)
	if err != nil {
		return fmt.Errorf("fetch polar subscriptions: %w", err)
	}

	admin, err := fetchAdminMetrics(ctx, client, cfg.PublicBaseURL, cfg.AdminAPIKey, *windowFlag)
	if err != nil {
		return fmt.Errorf("fetch admin metrics: %w", err)
	}

	health, err := probeHealthz(ctx, client, cfg.PublicBaseURL)
	if err != nil {
		return fmt.Errorf("probe healthz: %w", err)
	}

	report := renderReport(pulseReport{
		GeneratedAt: now,
		WindowLabel: *windowFlag,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		Database:    dbMetrics,
		Polar:       polar,
		Admin:       admin,
		Health:      health,
	})

	path, err := writeReport(ctx, *outFlag, now, report)
	if err != nil {
		return fmt.Errorf("write report: %w", err)
	}

	logger.Printf("wrote pulse report %s", path)
	return nil
}

func loadPulseConfig() (pulseConfig, error) {
	databaseURL, err := requiredEnv("TURSO_DATABASE_URL")
	if err != nil {
		return pulseConfig{}, err
	}
	databaseAuth, err := requiredEnv("TURSO_AUTH_TOKEN")
	if err != nil {
		return pulseConfig{}, err
	}
	polarBearer, err := requiredEnv("POLAR_PULSE_TOKEN")
	if err != nil {
		return pulseConfig{}, err
	}
	polarOrgID, err := requiredEnv("POLAR_ORGANIZATION_ID")
	if err != nil {
		return pulseConfig{}, err
	}
	adminAPIKey, err := requiredEnv("ADMIN_API_KEY")
	if err != nil {
		return pulseConfig{}, err
	}

	publicBaseURL := strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL"))
	if publicBaseURL == "" {
		publicBaseURL = defaultPublicBaseURL
	}

	return pulseConfig{
		DatabaseURL:   databaseURL,
		DatabaseAuth:  databaseAuth,
		PolarBearer:   polarBearer,
		PolarOrgID:    polarOrgID,
		PublicBaseURL: publicBaseURL,
		AdminAPIKey:   adminAPIKey,
	}, nil
}

func requiredEnv(name string) (string, error) {
	value, ok := os.LookupEnv(name)
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return "", fmt.Errorf("required environment variable %s is missing", name)
	}
	return value, nil
}

func parseWindow(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if strings.HasSuffix(value, "h") {
		hours, err := strconv.Atoi(strings.TrimSuffix(value, "h"))
		if err != nil || hours <= 0 {
			return 0, fmt.Errorf("invalid hour window %q", value)
		}
		return time.Duration(hours) * time.Hour, nil
	}
	if strings.HasSuffix(value, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(value, "d"))
		if err != nil || days <= 0 {
			return 0, fmt.Errorf("invalid day window %q", value)
		}
		return time.Duration(days*dayHours) * time.Hour, nil
	}
	return 0, fmt.Errorf("invalid window %q", value)
}

func collectDatabaseMetrics(ctx context.Context, db *sql.DB, start time.Time, end time.Time) (databaseMetrics, error) {
	var metrics databaseMetrics
	if err := queryCount(ctx, db, &metrics.ActivePaidMailboxes,
		"SELECT COUNT(*) FROM mailboxes WHERE status = 'active'"); err != nil {
		return databaseMetrics{}, fmt.Errorf("active paid mailboxes: %w", err)
	}
	if err := queryCount(ctx, db, &metrics.NewClaims,
		"SELECT COUNT(*) FROM mailboxes WHERE created_at >= ? AND created_at < ?", start, end); err != nil {
		return databaseMetrics{}, fmt.Errorf("new claims: %w", err)
	}
	if err := queryCount(ctx, db, &metrics.Activated,
		"SELECT COUNT(*) FROM mailboxes WHERE created_at >= ? AND created_at < ? AND status = 'active'", start, end); err != nil {
		return databaseMetrics{}, fmt.Errorf("activated claims: %w", err)
	}
	if err := queryCount(ctx, db, &metrics.SupportVolume,
		"SELECT COUNT(*) FROM support_messages WHERE created_at >= ? AND created_at < ?", start, end); err != nil {
		return databaseMetrics{}, fmt.Errorf("support volume: %w", err)
	}
	if err := queryCount(ctx, db, &metrics.RenewalsInWindow,
		"SELECT COUNT(*) FROM mailboxes WHERE paid_at >= ? AND paid_at < ? AND status = 'active'", start, end); err != nil {
		return databaseMetrics{}, fmt.Errorf("renewals in window: %w", err)
	}
	return metrics, nil
}

func queryCount(ctx context.Context, db *sql.DB, destination *int64, statement string, args ...any) error {
	if err := db.QueryRowContext(ctx, statement, args...).Scan(destination); err != nil {
		return fmt.Errorf("query count: %w", err)
	}
	return nil
}

func fetchPolarSubscriptions(ctx context.Context, client *http.Client, bearer string, orgID string) (polarMetrics, error) {
	var lastErr error
	for attempt := 1; attempt <= polarMaxAttempts; attempt++ {
		metrics, retry, err := fetchPolarSubscriptionsAttempt(ctx, client, bearer, orgID)
		if err == nil {
			return metrics, nil
		}
		lastErr = err
		if !retry || attempt == polarMaxAttempts {
			break
		}
		if err := waitPolarRetry(ctx, attempt); err != nil {
			return polarMetrics{}, err
		}
	}
	if lastErr == nil {
		lastErr = errors.New("polar subscriptions failed")
	}
	return polarMetrics{}, lastErr
}

func fetchPolarSubscriptionsAttempt(ctx context.Context, client *http.Client, bearer string, orgID string) (polarMetrics, bool, error) {
	endpoint := polarSubscriptionsURL + "?organization_id=" + url.QueryEscape(orgID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return polarMetrics{}, false, fmt.Errorf("create polar request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)

	resp, err := client.Do(req)
	if err != nil {
		return polarMetrics{}, true, fmt.Errorf("do polar request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
		return polarMetrics{}, true, fmt.Errorf("polar subscriptions status %d", resp.StatusCode)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return polarMetrics{}, false, fmt.Errorf("polar subscriptions status %d", resp.StatusCode)
	}

	var payload any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return polarMetrics{}, false, fmt.Errorf("decode polar subscriptions: %w", err)
	}

	return polarMetrics{SubscriptionsObserved: countPolarSubscriptions(payload)}, false, nil
}

func waitPolarRetry(ctx context.Context, attempt int) error {
	timer := time.NewTimer(time.Duration(attempt) * polarRetryBackoff)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("wait polar retry: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

func countPolarSubscriptions(payload any) int {
	switch value := payload.(type) {
	case []any:
		return len(value)
	case map[string]any:
		for _, key := range []string{"items", "data", "results"} {
			if items, ok := value[key].([]any); ok {
				return len(items)
			}
		}
	}
	return 0
}

func fetchAdminMetrics(ctx context.Context, client *http.Client, baseURL string, adminKey string, window string) (adminMetrics, error) {
	endpoint, err := url.JoinPath(baseURL, "admin/metrics")
	if err != nil {
		return adminMetrics{}, fmt.Errorf("admin metrics url: %w", err)
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return adminMetrics{}, fmt.Errorf("parse admin metrics url: %w", err)
	}
	query := parsed.Query()
	query.Set("window", window)
	parsed.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return adminMetrics{}, fmt.Errorf("create admin metrics request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+adminKey)

	resp, err := client.Do(req)
	if err != nil {
		return adminMetrics{}, fmt.Errorf("do admin metrics request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return adminMetrics{}, fmt.Errorf("admin metrics status %d", resp.StatusCode)
	}

	var payload struct {
		ImapLogin           int64           `json:"imap_login"`
		ImapMessageFetched  int64           `json:"imap_message_fetched"`
		ResolveCalls        int64           `json:"resolve_calls"`
		FailedKeyProofRatio float64         `json:"failed_key_proof_ratio"`
		HTTPP50MS           int64           `json:"http_p50_ms"`
		HTTPP95MS           int64           `json:"http_p95_ms"`
		HTTPP99MS           int64           `json:"http_p99_ms"`
		TopErrors           []adminTopError `json:"top_errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return adminMetrics{}, fmt.Errorf("decode admin metrics: %w", err)
	}

	return adminMetrics{
		ImapLogin:                   payload.ImapLogin,
		ImapLoginAvailable:          true,
		ImapMessageFetched:          payload.ImapMessageFetched,
		ImapMessageFetchedAvailable: true,
		ResolveCalls:                payload.ResolveCalls,
		FailedKeyProofRatio:         payload.FailedKeyProofRatio,
		Latency: map[string]int64{
			"p50_ms": payload.HTTPP50MS,
			"p95_ms": payload.HTTPP95MS,
			"p99_ms": payload.HTTPP99MS,
		},
		TopErrors: payload.TopErrors,
	}, nil
}

func probeHealthz(ctx context.Context, client *http.Client, publicBaseURL string) (healthMetrics, error) {
	healthURL, err := url.JoinPath(publicBaseURL, "healthz")
	if err != nil {
		return healthMetrics{}, fmt.Errorf("healthz url: %w", err)
	}

	samples := make([]time.Duration, 0, healthProbeSamples)
	non200 := 0
	for i := 0; i < healthProbeSamples; i++ {
		duration, statusCode, err := probeHealthzOnce(ctx, client, healthURL)
		samples = append(samples, duration)
		if err != nil {
			non200++
			continue
		}
		if statusCode != http.StatusOK {
			non200++
		}
	}

	return healthMetrics{
		P50:        percentileDuration(samples, 50),
		P95:        percentileDuration(samples, 95),
		P99:        percentileDuration(samples, 99),
		Non200:     non200,
		ProbeCount: len(samples),
	}, nil
}

func probeHealthzOnce(ctx context.Context, client *http.Client, healthURL string) (time.Duration, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("create healthz request: %w", err)
	}

	start := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(start)
	if err != nil {
		return duration, 0, fmt.Errorf("do healthz request: %w", err)
	}
	defer resp.Body.Close()

	return duration, resp.StatusCode, nil
}

func percentileDuration(samples []time.Duration, percentile int) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(i int, j int) bool {
		return sorted[i] < sorted[j]
	})
	index := int(math.Ceil(float64(percentile)*float64(len(sorted))/percentileScale)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index].Round(time.Millisecond)
}

func renderReport(report pulseReport) string {
	conversion := conversionPercent(report.Database.Activated, report.Database.NewClaims)
	resolveCallsPerMailbox := formatResolveCallsPerActiveMailbox(report.Admin.ResolveCalls, report.Database.ActivePaidMailboxes)

	var b strings.Builder
	fmt.Fprintf(&b, "# Pulse Report\n")
	fmt.Fprintf(&b, "Generated: %s UTC\n", report.GeneratedAt.UTC().Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "Window: %s (%s to %s UTC, 15m trailing buffer)\n\n",
		report.WindowLabel,
		report.WindowStart.UTC().Format("2006-01-02 15:04"),
		report.WindowEnd.UTC().Format("2006-01-02 15:04"))

	fmt.Fprintf(&b, "## Headlines\n")
	fmt.Fprintf(&b, "- Active paid mailboxes: %d; new claim activation: %.1f%%.\n", report.Database.ActivePaidMailboxes, conversion)
	fmt.Fprintf(&b, "- Renewals: %d; support messages: %d in the window.\n", report.Database.RenewalsInWindow, report.Database.SupportVolume)
	fmt.Fprintf(&b, "- Healthz is CI-side, not prod-side: p50 %s, p95 %s, p99 %s; non-200 probes: %d/%d.\n\n",
		report.Health.P50, report.Health.P95, report.Health.P99, report.Health.Non200, report.Health.ProbeCount)

	fmt.Fprintf(&b, "## Usage\n")
	fmt.Fprintf(&b, "- active_paid_mailboxes: %d\n", report.Database.ActivePaidMailboxes)
	fmt.Fprintf(&b, "- claim_to_activation: %d/%d (%.1f%%)\n", report.Database.Activated, report.Database.NewClaims, conversion)
	fmt.Fprintf(&b, "- renewals_in_window: %d\n", report.Database.RenewalsInWindow)
	fmt.Fprintf(&b, "- support_volume: %d\n", report.Database.SupportVolume)
	if report.Admin.ImapLoginAvailable {
		fmt.Fprintf(&b, "- imap_login: %d\n", report.Admin.ImapLogin)
	} else {
		fmt.Fprintf(&b, "- imap_login: %s\n", pendingMetricNote)
	}
	if report.Admin.ImapMessageFetchedAvailable {
		fmt.Fprintf(&b, "- imap_message_fetched: %d\n", report.Admin.ImapMessageFetched)
	} else {
		fmt.Fprintf(&b, "- imap_message_fetched: %s\n", pendingMetricNote)
	}
	fmt.Fprintf(&b, "- resolve_calls_per_active_mailbox_per_week: %s\n", resolveCallsPerMailbox)
	fmt.Fprintf(&b, "- failed_key_proof_ratio: %.4f\n\n", report.Admin.FailedKeyProofRatio)

	fmt.Fprintf(&b, "## System\n")
	fmt.Fprintf(&b, "- healthz p50/p95/p99 (CI-side, not prod-side): %s / %s / %s\n", report.Health.P50, report.Health.P95, report.Health.P99)
	fmt.Fprintf(&b, "- healthz non_200: %d/%d\n", report.Health.Non200, report.Health.ProbeCount)
	fmt.Fprintf(&b, "- polar_subscriptions_observed: %d\n", report.Polar.SubscriptionsObserved)
	fmt.Fprintf(&b, "- top_errors: %s\n", formatTopErrors(report.Admin.TopErrors))
	fmt.Fprintf(&b, "- latency_p50_p95_p99: %dms / %dms / %dms\n\n",
		report.Admin.Latency["p50_ms"],
		report.Admin.Latency["p95_ms"],
		report.Admin.Latency["p99_ms"])

	fmt.Fprintf(&b, "## Followups\n")
	fmt.Fprintf(&b, "1. Ship prod telemetry for pending IMAP metrics.\n")
	fmt.Fprintf(&b, "2. Compare Polar subscription count against active paid mailboxes for billing drift.\n")
	fmt.Fprintf(&b, "3. Investigate any healthz non-200 probes from this CI-side sample.\n")
	fmt.Fprintf(&b, "4. Add IMAP log shipping before treating mailbox read activity as prod-side truth.\n")
	fmt.Fprintf(&b, "5. Review support volume changes when the next report lands.\n")
	return b.String()
}

func formatResolveCallsPerActiveMailbox(resolveCalls int64, activePaidMailboxes int64) string {
	if activePaidMailboxes <= 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.1f", float64(resolveCalls)*7/float64(activePaidMailboxes))
}

func formatTopErrors(errors []adminTopError) string {
	if len(errors) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(errors))
	for _, item := range errors {
		parts = append(parts, fmt.Sprintf("%s (%d)", item.Msg, item.Count))
	}
	return strings.Join(parts, "; ")
}

func conversionPercent(activated int64, newClaims int64) float64 {
	denominator := newClaims
	if denominator < 1 {
		denominator = 1
	}
	return float64(activated) / float64(denominator) * claimConversionDecimal
}

func writeReport(ctx context.Context, outDir string, now time.Time, report string) (string, error) {
	select {
	case <-ctx.Done():
		return "", fmt.Errorf("write report canceled: %w", ctx.Err())
	default:
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}

	filename := now.UTC().Format("2006-01-02_15-04") + ".md"
	path := filepath.Join(outDir, filename)
	if err := os.WriteFile(path, []byte(report), 0o644); err != nil {
		return "", fmt.Errorf("write output file: %w", err)
	}
	return path, nil
}
