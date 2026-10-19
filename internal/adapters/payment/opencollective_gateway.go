package payment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
)

const (
	ocPageSize    = 100
	ocMaxPages    = 20
	ocTimeout     = 30 * time.Second
	ocOrdersQuery = `query ($slug: String!, $limit: Int!, $offset: Int!) {
  account(slug: $slug) {
    orders(limit: $limit, offset: $offset, filter: INCOMING) {
      nodes { publicId frequency tier { slug } tags transactions { publicId type kind createdAt isRefunded } }
    }
  }
}`
)

// OpenCollectiveConfig contains the checkout and read-only order API settings.
type OpenCollectiveConfig struct {
	CheckoutURL   string
	APIURL        string
	PersonalToken string
	ProjectSlug   string
	TierSlug      string
	Client        *http.Client
}

// OpenCollectiveGateway creates tagged checkouts and polls settled orders.
type OpenCollectiveGateway struct {
	checkoutURL *url.URL
	apiURL      string
	token       string
	projectSlug string
	tierSlug    string
	client      *http.Client
}

// NewOpenCollectiveGateway validates the checkout and API endpoints.
func NewOpenCollectiveGateway(cfg OpenCollectiveConfig) (*OpenCollectiveGateway, error) {
	checkout, err := url.Parse(cfg.CheckoutURL)
	if err != nil || checkout.Scheme != "https" || checkout.Host != "opencollective.com" || !strings.Contains(checkout.Path, "/contribute/") || !strings.HasSuffix(checkout.Path, "/checkout") {
		return nil, fmt.Errorf("invalid Open Collective checkout URL")
	}
	api, err := url.Parse(cfg.APIURL)
	if err != nil || (api.Scheme != "https" && !(api.Scheme == "http" && (api.Hostname() == "localhost" || api.Hostname() == "127.0.0.1"))) || api.Host == "" || api.User != nil || api.RawQuery != "" || api.Fragment != "" {
		return nil, fmt.Errorf("invalid Open Collective API URL")
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: ocTimeout}
	}
	return &OpenCollectiveGateway{checkoutURL: checkout, apiURL: cfg.APIURL, token: cfg.PersonalToken, projectSlug: cfg.ProjectSlug, tierSlug: cfg.TierSlug, client: client}, nil
}

func checkoutTag(mailboxID string) string {
	sum := sha256.Sum256([]byte("truevipaccess-checkout:" + mailboxID))
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:])
	return "tva-" + strings.ToLower(encoded[:16])
}

// CreatePaymentLink returns a tagged checkout URL without making a network call.
func (g *OpenCollectiveGateway) CreatePaymentLink(_ context.Context, req ports.PaymentLinkRequest) (*ports.PaymentLink, error) {
	if req.MailboxID == "" {
		return nil, fmt.Errorf("mailbox ID is required")
	}
	tag := checkoutTag(req.MailboxID)
	checkout := *g.checkoutURL
	query := checkout.Query()
	query.Set("tags", tag)
	checkout.RawQuery = query.Encode()
	return &ports.PaymentLink{SessionID: tag, URL: checkout.String()}, nil
}

// GetPaymentSession checks whether this provider issued the session tag.
func (g *OpenCollectiveGateway) GetPaymentSession(_ context.Context, sessionID string) (*ports.PaymentSession, error) {
	// Tags are deterministic per mailbox, so a tva- checkout stays reusable; any other ID is a retired provider's session.
	if !strings.HasPrefix(sessionID, "tva-") {
		return nil, ports.ErrPaymentSessionNotFound
	}
	return &ports.PaymentSession{SessionID: sessionID, Status: ports.PaymentSessionStatusOpen}, nil
}

type ocOrdersResponse struct {
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Data struct {
		Account *struct {
			Orders struct {
				Nodes []struct {
					PublicID  string `json:"publicId"`
					Frequency string `json:"frequency"`
					Tier      *struct {
						Slug string `json:"slug"`
					} `json:"tier"`
					Tags         []string `json:"tags"`
					Transactions []struct {
						PublicID   string `json:"publicId"`
						Type       string `json:"type"`
						Kind       string `json:"kind"`
						CreatedAt  string `json:"createdAt"`
						IsRefunded *bool  `json:"isRefunded"`
					} `json:"transactions"`
				} `json:"nodes"`
			} `json:"orders"`
		} `json:"account"`
	} `json:"data"`
}

// ListPaidCharges reads all monthly orders on the configured tier. Price is deliberately ignored.
func (g *OpenCollectiveGateway) ListPaidCharges(ctx context.Context) ([]ports.PaymentCharge, error) {
	if g.token == "" || g.projectSlug == "" || g.tierSlug == "" {
		return nil, fmt.Errorf("Open Collective reconciliation requires token, project slug and tier slug")
	}
	var charges []ports.PaymentCharge
	for page := 0; page < ocMaxPages; page++ {
		batch, err := g.fetchPage(ctx, page*ocPageSize)
		if err != nil {
			return nil, err
		}
		for _, order := range batch.Data.Account.Orders.Nodes {
			if order.Tier == nil || order.Tier.Slug != g.tierSlug || order.Frequency != "MONTHLY" || order.PublicID == "" {
				continue
			}
			// One order pays for exactly one mailbox; several tva- tags would let one
			// €1 contribution activate many, so an ambiguous order credits none.
			tag, ok := singleMailboxTag(order.Tags)
			if !ok {
				continue
			}
			for _, transaction := range order.Transactions {
				// A nullable refund flag is not evidence that the charge remains settled.
				if transaction.Type != "CREDIT" || transaction.Kind != "CONTRIBUTION" || transaction.IsRefunded == nil || *transaction.IsRefunded || transaction.PublicID == "" {
					continue
				}
				chargedAt, err := time.Parse(time.RFC3339, transaction.CreatedAt)
				if err != nil {
					return nil, fmt.Errorf("invalid Open Collective transaction time: %w", err)
				}
				charges = append(charges, ports.PaymentCharge{Tag: tag, ChargeID: transaction.PublicID, ChargedAt: chargedAt.UTC()})
			}
		}
		if len(batch.Data.Account.Orders.Nodes) < ocPageSize {
			sort.Slice(charges, func(i, j int) bool { return charges[i].ChargedAt.Before(charges[j].ChargedAt) })
			return charges, nil
		}
	}
	return nil, fmt.Errorf("Open Collective orders exceeded %d pages", ocMaxPages)
}

func (g *OpenCollectiveGateway) fetchPage(ctx context.Context, offset int) (ocOrdersResponse, error) {
	var result ocOrdersResponse
	body, err := json.Marshal(map[string]any{"query": ocOrdersQuery, "variables": map[string]any{"slug": g.projectSlug, "limit": ocPageSize, "offset": offset}})
	if err != nil {
		return result, fmt.Errorf("encode Open Collective query: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.apiURL, bytes.NewReader(body))
	if err != nil {
		return result, fmt.Errorf("create Open Collective request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Personal-Token", g.token)
	req.Header.Set("User-Agent", "truevipaccess-reconcile/1 (+https://truevipaccess.com)")
	resp, err := g.client.Do(req)
	if err != nil {
		return result, fmt.Errorf("Open Collective request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return result, fmt.Errorf("read Open Collective response: %w", err)
	}
	// A proxy or bot wall answers with HTML; name it instead of failing on the first '<'.
	if !bytes.HasPrefix(bytes.TrimSpace(respBody), []byte("{")) {
		return result, fmt.Errorf("Open Collective HTTP %d, content-type %q, server %q, cf-ray %q: %.300s",
			resp.StatusCode, resp.Header.Get("Content-Type"), resp.Header.Get("Server"), resp.Header.Get("Cf-Ray"), respBody)
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return result, fmt.Errorf("decode Open Collective response (HTTP %d): %w", resp.StatusCode, err)
	}
	if len(result.Errors) != 0 {
		return result, fmt.Errorf("Open Collective rejected orders query: %s", result.Errors[0].Message)
	}
	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("Open Collective HTTP %d", resp.StatusCode)
	}
	if result.Data.Account == nil {
		return result, fmt.Errorf("Open Collective account unavailable")
	}
	return result, nil
}

// singleMailboxTag returns the order's only tva- tag; zero or several is not attributable.
func singleMailboxTag(tags []string) (string, bool) {
	found := ""
	for _, tag := range tags {
		if !strings.HasPrefix(tag, "tva-") {
			continue
		}
		if found != "" {
			return "", false
		}
		found = tag
	}
	return found, found != ""
}
