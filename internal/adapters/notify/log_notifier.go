package notify

import (
	"context"
	"log"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
)

type LogNotifier struct {
	logger *log.Logger
}

func NewLogNotifier(logger *log.Logger) *LogNotifier {
	return &LogNotifier{logger: logger}
}

func (n *LogNotifier) SendPaymentLink(_ context.Context, ownerEmail string, paymentURL string, mailboxID string) error {
	n.logger.Printf("send owner payment email owner=%s mailbox=%s payment_url=%s", ownerEmail, mailboxID, paymentURL)
	return nil
}

func (n *LogNotifier) SendRecoveryLink(_ context.Context, ownerEmail string, recoveryURL string) error {
	n.logger.Printf("send owner recovery link owner=%s recovery_url=%s", ownerEmail, recoveryURL)
	return nil
}

func (n *LogNotifier) SendSupportMessage(_ context.Context, params ports.SupportMessageParams) error {
	n.logger.Printf("send support message to=%s from_mailbox=%s subject=%q", params.ToEmail, params.MailboxID, params.Subject)
	return nil
}
