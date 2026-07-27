# Legal/policy pages for Paddle domain approval

## What was built

Four new public, unauthenticated HTML pages plus routes, matching the home
page's design system (Georgia serif, warm cream palette, `--bg/--ink/--muted/
--card/--line/--accent` custom properties):

| Route | Purpose |
|---|---|
| `GET /terms` | Terms and Conditions |
| `GET /refund-policy` | Refund Policy |
| `GET /privacy` | Privacy Notice |
| `GET /contact` | Contact |

New file: `internal/adapters/httpapi/legal_pages.go` — handlers, a shared
layout/CSS (`renderLegalPage`, `legalPageTemplate`, `legalPageCSS`), and the
four page bodies as separate constants (`termsBodyHTML`,
`refundPolicyBodyHTML`, `privacyBodyHTML`, `contactBodyHTML`).

Routes registered in `internal/adapters/httpapi/handler.go`'s `Routes()`,
alongside `GET /` and `GET /healthz` — **no** `withAccountToken` or
`withAdminKey` wrapper on any of the four.

Home page footer (`homePageHTMLTemplate` in `handler.go`) updated to link to
all four pages, so a reviewer landing on `/` can find them without already
knowing the URLs.

### Content highlights
- **Paddle named as merchant of record** on `/terms` (§3) and `/refund-policy`
  (§1): "Paddle.com Market Limited ... acting as our reseller and the merchant
  of record for every purchase," handling payment, invoicing, tax, and
  billing enquiries.
- **Pricing publicly visible**: `/terms` §2 states "1 EUR per month per
  mailbox," consistent with the home page lede ("1 EUR/month") and the U6
  checkout page's `paddleCheckoutPriceLabel` ("1 EUR / month").
- **Refund policy** (`/refund-policy`): concrete 14-calendar-day window tied
  to each charge (first payment and each renewal), citing the EU distance-
  selling cooling-off basis, plus a numbered how-to-request process (via
  Paddle directly, or via us as a pass-through).
- **GDPR-aware privacy notice** (`/privacy`): what's collected (billing
  email, mailbox address + key fingerprint, inbound email content, IMAP/API
  access logs), lawful basis per category, retention, data subject rights
  (access/rectification/erasure/restriction/portability/objection), the
  Lithuanian supervisory authority (VDAI) for complaints, and Paddle named
  as an independent controller for billing data.
- **Contact page**: references the existing `POST /v1/support/messages`
  signed-EdProof endpoint for agents with a mailbox, the existing
  `hi@truevipaccess.com` address (already used site-wide) for general
  enquiries, and routes billing/refund questions to Paddle. No invented
  phone number or postal address.
- **Cross-linking**: every page has a top `nav.crumbs` and a bottom
  `footer.site` linking Home / Terms / Refund Policy / Privacy / Contact.
  Home page footer now links to all four as well.
- **CSP**: `default-src 'none'; style-src 'self' 'unsafe-inline'; base-uri
  'none'` — no `script-src` at all (these pages have zero JS), matching the
  strictness precedent set by U6's `paddleContentSecurityPolicy` but tighter
  since no CDN script is needed here.

## Tests

New file: `internal/adapters/httpapi/legal_pages_test.go`. Covers:
- All four routes return 200, `Content-Type: text/html`, with no auth
  wrapper (`TestLegalPagesRequireNoAuthAndReturn200`).
- Strict CSP present, `default-src 'none'`, no `script-src`
  (`TestLegalPagesSetStrictCSPWithNoScriptSrc`).
- Cross-links to `/`, `/terms`, `/refund-policy`, `/privacy`, `/contact`
  present on every page (`TestLegalPagesCrossLinkToEachOtherAndHome`).
- Paddle named as MoR + 1 EUR/month price on `/terms`
  (`TestTermsPageNamesPaddleAsMerchantOfRecord`).
- Paddle named as MoR + 14-day window on `/refund-policy`
  (`TestRefundPolicyNamesPaddleAsMerchantOfRecordAndStatesWindow`).
- GDPR content categories, rights, and Paddle-as-independent-controller
  language on `/privacy`
  (`TestPrivacyPageCoversGDPRRightsAndPaddleAsIndependentController`).
- Support endpoint + contact email present, on `/contact`
  (`TestContactPageReferencesSupportEndpointAndNoInventedContactDetails`).
- Home page still 200s and now links to all four legal pages
  (`TestHomePageLinksToLegalPages`).

### Verification run
```
go build ./...        # clean
go vet ./...           # clean
gofmt -l $(git ls-files '*.go')   # empty output
go test ./... -race    # all packages ok
```
All four commands passed with no output/failures.

## Self-review findings

- No route requires auth — confirmed by reading `Routes()` registration
  (plain `mux.HandleFunc`, not wrapped in `h.withAccountToken` or
  `h.withAdminKey`) and by the `TestLegalPagesRequireNoAuthAndReturn200`
  test.
- No fabricated legal identifiers anywhere in the four pages — every place
  a company registration number, VAT number, registered address, or
  jurisdiction would normally appear is a clearly marked
  `<!-- TODO(legal): ... -->` + `[BRACKETED PLACEHOLDER]` inside a
  visually distinct `.todo` span (dashed border, pink background) rather
  than plain body text, so it can't be missed in a visual pass either.
- Paddle is explicitly named as merchant of record on both `/terms` and
  `/refund-policy`, using the phrase "merchant of record" verbatim (matches
  Paddle's own compliance-check wording).
- 1 EUR/month price is publicly visible on `/terms` with no auth, no
  signup gate.
- All five pages (`/`, `/terms`, `/refund-policy`, `/privacy`, `/contact`)
  cross-link to each other.
- CSP has no `script-src` directive on any of the four new pages; verified
  by test, not just by eyeballing the constant.
- Content is specific to this product throughout (inbound-only mailboxes,
  Ed25519/EdProof key binding, IMAP + HTTP API, no outbound sending) — no
  generic boilerplate that doesn't apply (no SMTP/outbound clauses,
  no claims of certifications, audits, or uptime SLAs the repo doesn't
  document).
- One judgment call: added Paddle's legal entity name as
  "Paddle.com Market Limited" throughout — this is Paddle's own publicly
  documented merchant-of-record entity name (used in their own Buyer Terms/
  Privacy Policy), not something invented for this business, so it did not
  need a TODO placeholder. Flagging it here for visibility anyway in case a
  human wants to double check it's still current before submission.

## TODO placeholders a human must fill in before submitting to Paddle

1. **`/terms` §1** — `[COMPANY LEGAL NAME], [REGISTRATION NUMBER],
   [REGISTERED ADDRESS]` (suggested value if applicable: IT Uoga MB, a
   Lithuanian mažoji bendrija — unverified, from an unrelated payment-
   provider org record).
2. **`/terms` §10** — `[GOVERNING LAW / JURISDICTION]`.
3. **`/privacy` §1** — `[COMPANY LEGAL NAME], [REGISTRATION NUMBER],
   [REGISTERED ADDRESS]` (same entity as #1).
4. **`/contact`** ("Business details" section) — `[COMPANY LEGAL NAME],
   [REGISTRATION NUMBER], [REGISTERED ADDRESS]` (same entity as #1).

All four are wrapped in a visually distinct `<span class="todo">` with an
HTML comment `<!-- TODO(legal): replace before submitting to Paddle for
domain approval -->` immediately preceding the placeholder text, so they
are easy to grep for (`grep -rn "TODO(legal)"`) or spot visually.

## Files changed

- `internal/adapters/httpapi/legal_pages.go` (new)
- `internal/adapters/httpapi/legal_pages_test.go` (new)
- `internal/adapters/httpapi/handler.go` (modified: route registration +
  home page footer cross-links)
