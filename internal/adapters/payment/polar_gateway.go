package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
)

type PolarConfig struct {
	ServerURL  string
	Token      string
	ProductID  string
	SuccessURL string
	ReturnURL  string
	Client     *http.Client
}

type PolarGateway struct {
	serverURL  string
	token      string
	productID  string
	successURL string
	returnURL  string
	client     *http.Client
}

func NewPolarGateway(cfg PolarConfig) *PolarGateway {
	serverURL := strings.TrimRight(strings.TrimSpace(cfg.ServerURL), "/")
	if serverURL == "" {
		serverURL = "https://api.polar.sh"
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &PolarGateway{
		serverURL:  serverURL,
		token:      strings.TrimSpace(cfg.Token),
		productID:  strings.TrimSpace(cfg.ProductID),
		successURL: strings.TrimSpace(cfg.SuccessURL),
		returnURL:  strings.TrimSpace(cfg.ReturnURL),
		client:     client,
	}
}

func (g *PolarGateway) CreatePaymentLink(ctx context.Context, req ports.PaymentLinkRequest) (*ports.PaymentLink, error) {
	payload := map[string]any{
		"products":             []string{g.productID},
		"customer_email":       req.OwnerEmail,
		"external_customer_id": req.MailboxID,
		"success_url":          g.successURL,
		"metadata": map[string]string{
			"mailbox_id":  req.MailboxID,
			"owner_email": req.OwnerEmail,
		},
	}
	if g.returnURL != "" {
		payload["return_url"] = g.returnURL
	}
	if req.DiscountID != "" {
		payload["discount_id"] = req.DiscountID
	}

	var resp struct {
		ID     string `json:"id"`
		URL    string `json:"url"`
		Status string `json:"status"`
	}
	if err := g.doJSON(ctx, http.MethodPost, "/v1/checkouts/", payload, &resp); err != nil {
		if req.DiscountID != "" && isDiscountError(err) {
			return nil, fmt.Errorf("%w: %v", ports.ErrCouponExhausted, err)
		}
		return nil, err
	}
	if resp.ID == "" || resp.URL == "" {
		return nil, fmt.Errorf("polar create checkout: missing id or url")
	}

	return &ports.PaymentLink{
		SessionID: resp.ID,
		URL:       resp.URL,
	}, nil
}

func (g *PolarGateway) GetPaymentSession(ctx context.Context, sessionID string) (*ports.PaymentSession, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("polar get checkout: missing session id")
	}

	var resp struct {
		ID     string `json:"id"`
		URL    string `json:"url"`
		Status string `json:"status"`
	}
	if err := g.doJSON(ctx, http.MethodGet, "/v1/checkouts/"+sessionID, nil, &resp); err != nil {
		return nil, err
	}
	if resp.ID == "" {
		return nil, fmt.Errorf("polar get checkout: missing id")
	}

	return &ports.PaymentSession{
		SessionID: resp.ID,
		Status:    mapPolarStatus(resp.Status),
		URL:       resp.URL,
	}, nil
}

func (g *PolarGateway) doJSON(ctx context.Context, method string, path string, payload any, into any) error {
	var bodyReader *bytes.Reader
	if payload == nil {
		bodyReader = bytes.NewReader(nil)
	} else {
		body, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, g.serverURL+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &polarAPIError{StatusCode: resp.StatusCode, Method: method, Path: path}
		var body map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&body); err == nil {
			apiErr.Body = body
		}
		return apiErr
	}

	if into == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(into)
}

// polarAPIError is returned by doJSON when Polar responds with a non-2xx status.
type polarAPIError struct {
	StatusCode int
	Method     string
	Path       string
	Body       map[string]any
}

func (e *polarAPIError) Error() string {
	return fmt.Sprintf("polar api %s %s: status %d: %v", e.Method, e.Path, e.StatusCode, e.Body)
}

// isDiscountError checks whether a Polar API error is a validation rejection
// (HTTP 422) which is what Polar returns when a discount_id is invalid or exhausted.
func isDiscountError(err error) bool {
	var apiErr *polarAPIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 422
	}
	return false
}

func mapPolarStatus(status string) ports.PaymentSessionStatus {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "succeeded":
		return ports.PaymentSessionStatusSucceeded
	case "confirmed":
		return ports.PaymentSessionStatusConfirmed
	case "expired":
		return ports.PaymentSessionStatusExpired
	case "failed":
		return ports.PaymentSessionStatusFailed
	default:
		return ports.PaymentSessionStatusOpen
	}
}
