package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
)

type PolarConfig struct {
	ServerURL  string
	Token      string
	PriceID    string
	SuccessURL string
	ReturnURL  string
}

type PolarGateway struct {
	serverURL  string
	token      string
	priceID    string
	successURL string
	returnURL  string
	client     *http.Client
}

func NewPolarGateway(cfg PolarConfig) *PolarGateway {
	serverURL := strings.TrimRight(strings.TrimSpace(cfg.ServerURL), "/")
	if serverURL == "" {
		serverURL = "https://api.polar.sh"
	}
	return &PolarGateway{
		serverURL:  serverURL,
		token:      strings.TrimSpace(cfg.Token),
		priceID:    strings.TrimSpace(cfg.PriceID),
		successURL: strings.TrimSpace(cfg.SuccessURL),
		returnURL:  strings.TrimSpace(cfg.ReturnURL),
		client:     http.DefaultClient,
	}
}

func (g *PolarGateway) CreatePaymentLink(ctx context.Context, req ports.PaymentLinkRequest) (*ports.PaymentLink, error) {
	payload := map[string]any{
		"product_price_id": g.priceID,
		"customer_email":   req.OwnerEmail,
		"success_url":      g.successURL,
		"metadata": map[string]string{
			"mailbox_id":  req.MailboxID,
			"owner_email": req.OwnerEmail,
		},
	}
	if g.returnURL != "" {
		payload["return_url"] = g.returnURL
	}

	var resp struct {
		ID     string `json:"id"`
		URL    string `json:"url"`
		Status string `json:"status"`
	}
	if err := g.doJSON(ctx, http.MethodPost, "/v1/checkouts/custom/", payload, &resp); err != nil {
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
		var apiErr map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err == nil {
			return fmt.Errorf("polar api %s %s: status %d: %v", method, path, resp.StatusCode, apiErr)
		}
		return fmt.Errorf("polar api %s %s: status %d", method, path, resp.StatusCode)
	}

	if into == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(into)
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
