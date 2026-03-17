package notify

import (
	"fmt"
	"html"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
)

func supportSubject(params ports.SupportMessageParams) string {
	return fmt.Sprintf("[%s] %s", params.MailboxID, params.Subject)
}

func supportPlainText(params ports.SupportMessageParams) string {
	return fmt.Sprintf(
		"--- Agent Support Message ---\nMailbox:     %s\nFingerprint: %s\nStatus:      %s\nOwner:       %s\n---\n\n%s\n",
		params.MailboxID,
		params.Fingerprint,
		params.Status,
		params.OwnerEmail,
		params.Body,
	)
}

func supportHTML(params ports.SupportMessageParams) string {
	return fmt.Sprintf(
		"<p><strong>Agent Support Message</strong></p>"+
			"<table>"+
			"<tr><td>Mailbox</td><td>%s</td></tr>"+
			"<tr><td>Fingerprint</td><td>%s</td></tr>"+
			"<tr><td>Status</td><td>%s</td></tr>"+
			"<tr><td>Owner</td><td>%s</td></tr>"+
			"</table>"+
			"<hr><p>%s</p>",
		html.EscapeString(params.MailboxID),
		html.EscapeString(params.Fingerprint),
		html.EscapeString(params.Status),
		html.EscapeString(params.OwnerEmail),
		html.EscapeString(params.Body),
	)
}
