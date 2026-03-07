package notify

import (
	"context"
	"log"
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

func (n *LogNotifier) SendRecoveryCode(_ context.Context, ownerEmail string, code string) error {
	n.logger.Printf("send owner recovery code owner=%s code=%s", ownerEmail, code)
	return nil
}
