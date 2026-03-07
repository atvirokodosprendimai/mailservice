package notify

import (
	"context"
	"fmt"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

type SendGridNotifier struct {
	client    *sendgrid.Client
	fromEmail string
	fromName  string
}

func NewSendGridNotifier(apiKey string, fromEmail string, fromName string) *SendGridNotifier {
	return &SendGridNotifier{
		client:    sendgrid.NewSendClient(apiKey),
		fromEmail: fromEmail,
		fromName:  fromName,
	}
}

func (n *SendGridNotifier) SendPaymentLink(ctx context.Context, ownerEmail string, paymentURL string, mailboxID string) error {
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

func (n *SendGridNotifier) SendRecoveryLink(ctx context.Context, ownerEmail string, recoveryURL string) error {
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

func (n *SendGridNotifier) send(_ context.Context, toEmail string, subject string, plainText string, html string) error {
	from := mail.NewEmail(n.fromName, n.fromEmail)
	to := mail.NewEmail("", toEmail)
	message := mail.NewSingleEmail(from, subject, to, plainText, html)

	resp, err := n.client.Send(message)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("sendgrid status %d: %s", resp.StatusCode, resp.Body)
	}

	return nil
}
