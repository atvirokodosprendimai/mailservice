package notify

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
)

var validDomainRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?$`)

type MailgunNotifier struct {
	apiKey    string
	domain    string
	baseURL   string
	fromEmail string
	fromName  string
	client    *http.Client
}

func NewMailgunNotifier(apiKey, domain, baseURL, fromEmail, fromName string) (*MailgunNotifier, error) {
	if !validDomainRe.MatchString(domain) {
		return nil, fmt.Errorf("invalid mailgun domain: %q", domain)
	}

	baseURL = strings.TrimSpace(strings.TrimRight(baseURL, "/"))
	if baseURL == "" {
		baseURL = "https://api.mailgun.net"
	}

	return &MailgunNotifier{
		apiKey:    apiKey,
		domain:    domain,
		baseURL:   baseURL,
		fromEmail: fromEmail,
		fromName:  fromName,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

func (n *MailgunNotifier) SendPaymentLink(ctx context.Context, ownerEmail string, paymentURL string, mailboxID string) error {
	subject := "Action needed: complete mailbox payment"
	body := fmt.Sprintf(
		"<p>Mailbox <strong>%s</strong> is waiting for payment.</p><p><a href=\"%s\">Complete payment</a></p>",
		html.EscapeString(mailboxID),
		html.EscapeString(paymentURL),
	)
	return n.send(ctx, ownerEmail, subject, body)
}

func (n *MailgunNotifier) SendRecoveryLink(ctx context.Context, ownerEmail string, recoveryURL string) error {
	subject := "Account recovery link"
	body := fmt.Sprintf(
		"<p>Open this one-time recovery link to restore access:</p><p><a href=\"%s\">Recover account access</a></p><p>This link expires in 10 minutes.</p>",
		html.EscapeString(recoveryURL),
	)
	return n.send(ctx, ownerEmail, subject, body)
}

func (n *MailgunNotifier) SendSupportMessage(ctx context.Context, params ports.SupportMessageParams) error {
	subject := supportSubject(params)
	htmlBody := supportHTML(params)

	from := n.fromEmail
	if n.fromName != "" {
		from = fmt.Sprintf("%s <%s>", n.fromName, n.fromEmail)
	}

	endpoint := fmt.Sprintf("%s/v3/%s/messages", n.baseURL, n.domain)

	form := url.Values{}
	form.Set("from", from)
	form.Set("to", params.ToEmail)
	form.Set("subject", subject)
	form.Set("html", htmlBody)
	if params.ReplyTo != "" {
		form.Set("h:Reply-To", params.ReplyTo)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("api", n.apiKey)

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("mailgun status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (n *MailgunNotifier) send(ctx context.Context, to, subject, html string) error {
	from := n.fromEmail
	if n.fromName != "" {
		from = fmt.Sprintf("%s <%s>", n.fromName, n.fromEmail)
	}

	endpoint := fmt.Sprintf("%s/v3/%s/messages", n.baseURL, n.domain)

	form := url.Values{}
	form.Set("from", from)
	form.Set("to", to)
	form.Set("subject", subject)
	form.Set("html", html)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("api", n.apiKey)

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("mailgun status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
