package imap

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
)

type Reader struct{}

func NewReader() *Reader {
	return &Reader{}
}

func (r *Reader) ListMessages(ctx context.Context, host string, port int, username string, password string, limit int, unreadOnly bool, includeBody bool) ([]ports.IMAPMessage, error) {
	c, err := r.connectAndLogin(host, port, username, password)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Logout() }()

	if _, err := c.Select("INBOX", true); err != nil {
		return nil, err
	}

	criteria := imap.NewSearchCriteria()
	if unreadOnly {
		criteria.WithoutFlags = []string{imap.SeenFlag}
	}
	uids, err := c.UidSearch(criteria)
	if err != nil {
		return nil, err
	}
	if len(uids) == 0 {
		return []ports.IMAPMessage{}, nil
	}

	sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
	if len(uids) > limit {
		uids = uids[len(uids)-limit:]
	}

	results, err := fetchByUIDs(ctx, c, uids, includeBody)
	if err != nil {
		return nil, err
	}
	reverseMessages(results)
	return results, nil
}

func (r *Reader) GetMessageByUID(ctx context.Context, host string, port int, username string, password string, uid uint32, includeBody bool) (*ports.IMAPMessage, error) {
	c, err := r.connectAndLogin(host, port, username, password)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Logout() }()

	if _, err := c.Select("INBOX", true); err != nil {
		return nil, err
	}

	messages, err := fetchByUIDs(ctx, c, []uint32{uid}, includeBody)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, ports.ErrMessageNotFound
	}
	return &messages[0], nil
}

func reverseMessages(items []ports.IMAPMessage) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func (r *Reader) connectAndLogin(host string, port int, username string, password string) (*client.Client, error) {
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
	if err := c.Login(username, password); err != nil {
		_ = c.Logout()
		return nil, err
	}
	return c, nil
}

func fetchByUIDs(ctx context.Context, c *client.Client, uids []uint32, includeBody bool) ([]ports.IMAPMessage, error) {
	if len(uids) == 0 {
		return []ports.IMAPMessage{}, nil
	}

	seqset := new(imap.SeqSet)
	for _, uid := range uids {
		seqset.AddNum(uid)
	}

	bodySection := &imap.BodySectionName{BodyPartName: imap.BodyPartName{Specifier: imap.TextSpecifier}, Peek: true}
	items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchInternalDate}
	if includeBody {
		items = append(items, bodySection.FetchItem())
	}
	messages := make(chan *imap.Message, len(uids))
	done := make(chan error, 1)

	go func() {
		done <- c.UidFetch(seqset, items, messages)
	}()

	results := make([]ports.IMAPMessage, 0, len(uids))
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
				sort.Slice(results, func(i, j int) bool { return results[i].UID < results[j].UID })
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
			if includeBody {
				if bodyReader := msg.GetBody(bodySection); bodyReader != nil {
					if bodyBytes, readErr := io.ReadAll(bodyReader); readErr == nil {
						entry.Body = strings.TrimSpace(string(bodyBytes))
					}
				}
			}
			results = append(results, entry)
		}
	}
}
