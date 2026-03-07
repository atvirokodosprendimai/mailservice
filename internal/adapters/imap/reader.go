package imap

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
)

type Reader struct{}

func NewReader() *Reader {
	return &Reader{}
}

func (r *Reader) ListMessages(ctx context.Context, host string, port int, username string, password string, limit int) ([]ports.IMAPMessage, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))

	var c *client.Client
	var err error
	if port == 993 {
		c, err = client.DialTLS(addr, &tls.Config{ServerName: host})
	} else {
		c, err = client.Dial(addr)
	}
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = c.Logout()
	}()

	if err := c.Login(username, password); err != nil {
		return nil, err
	}

	mbox, err := c.Select("INBOX", true)
	if err != nil {
		return nil, err
	}

	if mbox.Messages == 0 {
		return []ports.IMAPMessage{}, nil
	}

	from := uint32(1)
	if uint32(limit) < mbox.Messages {
		from = mbox.Messages - uint32(limit) + 1
	}

	seqset := new(imap.SeqSet)
	seqset.AddRange(from, mbox.Messages)

	items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchInternalDate}
	messages := make(chan *imap.Message, limit)
	done := make(chan error, 1)

	go func() {
		done <- c.Fetch(seqset, items, messages)
	}()

	results := make([]ports.IMAPMessage, 0, limit)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case msg, ok := <-messages:
			if !ok {
				err := <-done
				if err != nil {
					return nil, err
				}
				reverseMessages(results)
				return results, nil
			}

			entry := ports.IMAPMessage{UID: msg.Uid, Date: msg.InternalDate}
			if msg.Envelope != nil {
				entry.Subject = msg.Envelope.Subject
				if len(msg.Envelope.From) > 0 {
					a := msg.Envelope.From[0]
					entry.From = fmt.Sprintf("%s@%s", a.MailboxName, a.HostName)
				}
			}
			results = append(results, entry)
		}
	}
}

func reverseMessages(items []ports.IMAPMessage) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}
