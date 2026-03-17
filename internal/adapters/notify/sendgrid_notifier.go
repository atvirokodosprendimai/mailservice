package notify

import (
	"context"
	"fmt"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
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

func (n *SendGridNotifier) SendSupportMessage(ctx context.Context, params ports.SupportMessageParams) error {
	subject := supportSubject(params)
	plainText := supportPlainText(params)
	htmlBody := supportHTML(params)

	from := mail.NewEmail(n.fromName, n.fromEmail)
	to := mail.NewEmail("", params.ToEmail)
	message := mail.NewSingleEmail(from, subject, to, plainText, htmlBody)
	if params.ReplyTo != "" {
		message.SetReplyTo(mail.NewEmail("", params.ReplyTo))
	}

	resp, err := n.client.Send(message)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("sendgrid status %d: %s", resp.StatusCode, resp.Body)
	}
	return nil
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
