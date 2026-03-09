package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type UnsendNotifier struct {
	baseURL   string
	apiKey    string
	fromEmail string
	fromName  string
	client    *http.Client
}

func NewUnsendNotifier(baseURL string, apiKey string, fromEmail string, fromName string) *UnsendNotifier {
	baseURL = strings.TrimSpace(strings.TrimRight(baseURL, "/"))
	if baseURL == "" {
		baseURL = "https://unsend.admin.lt/api"
	}

	return &UnsendNotifier{
		baseURL:   baseURL,
		apiKey:    apiKey,
		fromEmail: fromEmail,
		fromName:  fromName,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (n *UnsendNotifier) SendPaymentLink(ctx context.Context, ownerEmail string, paymentURL string, mailboxID string) error {
	subject := "Action needed: complete mailbox payment"
	plainText := fmt.Sprintf(
		"Mailbox %s is waiting for payment.\n\nOpen this link to complete activation:\n%s\n",
		mailboxID,
		paymentURL,
	)
	html := fmt.Sprintf(
		"<p>Mailbox <strong>%s</strong> is waiting for payment.</p><p><a href=\"%s\">Complete payment</a></p>",
		mailboxID,
		paymentURL,
	)
	return n.send(ctx, ownerEmail, subject, plainText, html)
}

func (n *UnsendNotifier) SendRecoveryLink(ctx context.Context, ownerEmail string, recoveryURL string) error {
	subject := "Account recovery link"
	plainText := fmt.Sprintf(
		"Open this one-time recovery link to restore access:\n%s\n\nThis link expires in 10 minutes.\n",
		recoveryURL,
	)
	html := fmt.Sprintf(
		"<p>Open this one-time recovery link to restore access:</p><p><a href=\"%s\">Recover account access</a></p><p>This link expires in 10 minutes.</p>",
		recoveryURL,
	)
	return n.send(ctx, ownerEmail, subject, plainText, html)
}

func (n *UnsendNotifier) send(ctx context.Context, toEmail string, subject string, plainText string, html string) error {
	from := n.fromEmail
	if n.fromName != "" {
		from = fmt.Sprintf("%s <%s>", n.fromName, n.fromEmail)
	}

	payload := map[string]any{
		"from":    from,
		"to":      []string{toEmail},
		"subject": subject,
		"text":    plainText,
		"html":    html,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := n.baseURL + "/v1/emails"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+n.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("unsend status %d", resp.StatusCode)
	}

	return nil
}
