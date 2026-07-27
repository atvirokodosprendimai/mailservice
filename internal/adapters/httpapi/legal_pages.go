package httpapi

import (
	"fmt"
	"html"
	"io"
	"net/http"
)

// legalPagesContentSecurityPolicy locks these pages down as hard as
// possible: they are pure static content with no JavaScript at all, so
// unlike paddleContentSecurityPolicy (which must allow Paddle's checkout
// script), script-src is omitted entirely and covered by default-src
// 'none'. style-src needs 'unsafe-inline' for the inline <style> block,
// matching this codebase's existing template convention (see
// homePageHTMLTemplate, paddleCheckoutHTMLSource).
const legalPagesContentSecurityPolicy = "default-src 'none'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"base-uri 'none'"

// legalPagesLastUpdated is a single source of truth for the "last updated"
// stamp shown on every legal page, so the four pages can't drift out of
// sync with each other after an edit.
const legalPagesLastUpdated = "2026-07-27"

func (h *Handler) handleTerms(w http.ResponseWriter, _ *http.Request) {
	writeLegalPage(w, renderLegalPage(
		"Terms and Conditions",
		"Terms and conditions for TrueVIP Access Mailbox — inbound-only email mailboxes for AI agents.",
		"Terms and Conditions",
		termsBodyHTML,
	))
}

func (h *Handler) handleRefundPolicy(w http.ResponseWriter, _ *http.Request) {
	writeLegalPage(w, renderLegalPage(
		"Refund Policy",
		"Refund policy for TrueVIP Access Mailbox: a 14-day refund window and how to request one via Paddle.",
		"Refund Policy",
		refundPolicyBodyHTML,
	))
}

func (h *Handler) handlePrivacyNotice(w http.ResponseWriter, _ *http.Request) {
	writeLegalPage(w, renderLegalPage(
		"Privacy Notice",
		"Privacy notice for TrueVIP Access Mailbox: what data we collect, why, and your GDPR rights.",
		"Privacy Notice",
		privacyBodyHTML,
	))
}

func (h *Handler) handleContact(w http.ResponseWriter, _ *http.Request) {
	writeLegalPage(w, renderLegalPage(
		"Contact",
		"How to reach TrueVIP Access Mailbox for support, billing, and general enquiries.",
		"Contact",
		contactBodyHTML,
	))
}

func writeLegalPage(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Security-Policy", legalPagesContentSecurityPolicy)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, body)
}

// renderLegalPage wraps page-specific bodyHTML in the shared layout: a
// crumbs nav, an h1, the "last updated" stamp, and the cross-linking
// footer. bodyHTML is developer-authored static content (not user input),
// so it is inserted as-is; title/description come from the same source but
// are still escaped defensively since they land in attribute/text context.
func renderLegalPage(title, description, heading, bodyHTML string) string {
	return fmt.Sprintf(legalPageTemplate,
		html.EscapeString(title),
		html.EscapeString(description),
		legalPageCSS,
		html.EscapeString(heading),
		legalPagesLastUpdated,
		bodyHTML,
	)
}

const legalPageTemplate = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s — TrueVIP Access</title>
  <meta name="description" content="%s">
  <style>%s</style>
</head>
<body>
  <main>
    <nav class="crumbs">
      <a href="/">Home</a><a href="/terms">Terms</a><a href="/refund-policy">Refund Policy</a><a href="/privacy">Privacy</a><a href="/contact">Contact</a>
    </nav>
    <h1>%s</h1>
    <p class="updated">Last updated: %s</p>
%s
  </main>
  <footer class="site">
    <hr>
    <a href="/">Home</a> &middot; <a href="/terms">Terms</a> &middot; <a href="/refund-policy">Refund Policy</a> &middot; <a href="/privacy">Privacy</a> &middot; <a href="/contact">Contact</a>
    <br><br>
    Contact: <a href="mailto:hi@truevipaccess.com">hi@truevipaccess.com</a> &middot; AGPL v3.0
  </footer>
</body>
</html>
`

// legalPageCSS reuses the home page's warm cream palette and Georgia
// serif typography (see homePageHTMLTemplate) so these pages read as the
// same product rather than a bolted-on legal afterthought.
const legalPageCSS = `
    :root {
      color-scheme: light;
      --bg: #f4efe4;
      --ink: #17222d;
      --muted: #566575;
      --card: #fffaf0;
      --line: #d8cdb7;
      --accent: #a23b2a;
      --accent-ink: #fffaf0;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: Georgia, "Times New Roman", serif;
      background: var(--bg);
      color: var(--ink);
    }
    main {
      max-width: 760px;
      margin: 0 auto;
      padding: 48px 20px 72px;
    }
    nav.crumbs {
      margin-bottom: 28px;
      font: 600 0.85rem/1.4 ui-monospace, SFMono-Regular, Menlo, monospace;
    }
    nav.crumbs a {
      color: var(--accent);
      text-decoration: none;
      margin-right: 16px;
    }
    nav.crumbs a:hover { text-decoration: underline; }
    h1 {
      margin: 0 0 8px;
      font-size: clamp(1.8rem, 4vw, 2.6rem);
      letter-spacing: -0.02em;
    }
    .updated {
      margin: 0 0 28px;
      color: var(--muted);
      font: 500 0.85rem/1.5 ui-monospace, SFMono-Regular, Menlo, monospace;
    }
    h2 { font-size: 1.25rem; margin: 32px 0 10px; }
    h3 { font-size: 1.05rem; margin: 20px 0 8px; }
    p, li { line-height: 1.65; }
    ul, ol { padding-left: 22px; }
    .card {
      padding: 18px 20px;
      border: 1px solid var(--line);
      border-radius: 16px;
      background: var(--card);
      margin: 18px 0;
    }
    code {
      padding: 0.1em 0.35em;
      border-radius: 6px;
      background: #f0e7d5;
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
      font-size: 0.92em;
    }
    a { color: var(--accent); }
    .todo {
      display: inline-block;
      border: 1px dashed var(--accent);
      background: #fdeceb;
      padding: 2px 6px;
      border-radius: 6px;
    }
    footer.site {
      max-width: 760px;
      margin: 0 auto;
      padding: 0 20px 48px;
      text-align: center;
      font-size: 0.85rem;
      color: var(--muted);
    }
    footer.site hr {
      border: none;
      border-top: 1px solid var(--line);
      margin-bottom: 18px;
    }
    footer.site a { text-decoration: none; }
`

const termsBodyHTML = `
    <h2>1. Who operates this service</h2>
    <p>TrueVIP Access Mailbox ("the Service") is operated by
      <span class="todo"><!-- TODO(legal): replace before submitting to Paddle for domain approval -->
      [COMPANY LEGAL NAME], registration number [REGISTRATION NUMBER], registered address [REGISTERED ADDRESS]</span>
      ("we", "us"). Our current expectation is that this will be <strong>IT Uoga MB</strong>, a Lithuanian
      mažoji bendrija (small partnership) — but that name came from an unrelated payment-provider account
      record, not a verified company register filing, so it must be confirmed by a human before this page
      is published.</p>

    <h2>2. The service</h2>
    <p>The Service provides one inbound-only email mailbox per customer, bound to an Ed25519 key ("EdProof").
      Each mailbox includes 100 MB of storage and read access over IMAP and a JSON HTTP API. The Service does
      not provide SMTP submission, outbound relay, or any way to send mail from a mailbox.</p>
    <p>Pricing is <strong>1 EUR per month per mailbox</strong>, billed monthly in advance.</p>

    <h2>3. Merchant of record</h2>
    <p>All orders placed on this site are processed by <strong>Paddle.com Market Limited</strong> (or its
      applicable local Paddle entity, together "Paddle"), acting as our reseller and the merchant of record
      for every purchase. Paddle is responsible for processing your payment, issuing invoices and receipts,
      calculating and remitting any applicable sales tax or VAT, and handling billing-related enquiries and
      disputes. Your order is also subject to Paddle's own
      <a href="https://www.paddle.com/legal/checkout-buyer-terms">Buyer Terms</a>.</p>

    <h2>4. Account and mailbox identity</h2>
    <p>Your mailbox identity is the Ed25519 key you register, not a username, login account, or the billing
      email address used at checkout. Anyone able to sign a challenge with the matching private key can
      access the mailbox, so you are responsible for keeping that key secure. We cannot recover a mailbox if
      the key is lost.</p>

    <h2>5. Acceptable use</h2>
    <ul>
      <li>The Service is for receiving mail addressed to your assigned mailbox only.</li>
      <li>You may not use, or attempt to use, the Service to send, relay, or forward outbound mail — it has
        no such capability.</li>
      <li>You may not use the Service in furtherance of spam, phishing, fraud, or any unlawful activity.</li>
      <li>We may suspend or terminate a mailbox we reasonably believe is being used in violation of these
        Terms.</li>
    </ul>

    <h2>6. Availability</h2>
    <p>We aim to keep the Service available and mail delivery timely, but we do not offer an uptime
      service-level agreement. The Service is provided on an "as is" basis.</p>

    <h2>7. Cancellation and refunds</h2>
    <p>You may stop paying at any time; your mailbox stays usable until the end of the period already paid
      for, then expires. See our <a href="/refund-policy">Refund Policy</a> for the refund window and how to
      request one through Paddle.</p>

    <h2>8. Changes to these terms</h2>
    <p>We may update these Terms from time to time. The "Last updated" date above reflects the current
      version.</p>

    <h2>9. Contact</h2>
    <p>Questions about these Terms: see our <a href="/contact">Contact</a> page. Questions about a specific
      charge or invoice: contact Paddle directly, since Paddle is the merchant of record for your purchase.</p>

    <h2>10. Governing law</h2>
    <p><span class="todo"><!-- TODO(legal): confirm governing law / jurisdiction before publishing -->
      [GOVERNING LAW / JURISDICTION]</span></p>
`

const refundPolicyBodyHTML = `
    <h2>1. Paddle handles your billing</h2>
    <p>All payments for TrueVIP Access Mailbox are processed by Paddle.com Market Limited ("Paddle"), which
      acts as the merchant of record for every purchase made on this site. Paddle issues your invoice, appears
      on your card or bank statement, and is the right first point of contact for anything to do with a
      charge, invoice, or refund.</p>

    <h2>2. Refund window</h2>
    <p>You may request a full refund of your most recent monthly charge within <strong>14 calendar days</strong>
      of that charge, consistent with the EU/EEA distance-selling cooling-off period for consumers (Directive
      2011/83/EU as implemented in your country of residence). This applies both to your first payment and to
      each subsequent monthly renewal charge, individually.</p>
    <p>Outside that 14-day window, a charge already collected for a billing period is non-refundable, because
      the mailbox and its storage were provisioned and made available for that period. You can still cancel at
      any time to stop future charges — see below.</p>

    <h2>3. How to request a refund</h2>
    <ol>
      <li>Contact Paddle directly using the receipt/invoice email they sent you, or via Paddle's own support
        at <a href="https://www.paddle.com/help">paddle.com/help</a> — quote your order or transaction ID.</li>
      <li>Alternatively, contact us via the <a href="/contact">Contact</a> page and we will pass the request
        to Paddle on your behalf.</li>
    </ol>
    <p>Approved refunds are issued by Paddle to the original payment method, typically within a few business
      days.</p>

    <h2>4. Cancelling future charges</h2>
    <p>Cancelling stops future renewal charges; it does not by itself refund a period you have already paid
      for and are still within the 14-day window for (see above). Once a mailbox's paid period ends without
      renewal, it expires.</p>

    <h2>5. Faults and service issues</h2>
    <p>If the Service was materially unavailable, or failed to deliver mail to your mailbox, during a paid
      period, contact us via the <a href="/contact">Contact</a> page. We will assess a discretionary refund or
      credit for that period, coordinated with Paddle where a refund is due.</p>
`

const privacyBodyHTML = `
    <h2>1. Who is responsible for your data</h2>
    <p>The data controller for TrueVIP Access Mailbox is
      <span class="todo"><!-- TODO(legal): replace before submitting to Paddle for domain approval -->
      [COMPANY LEGAL NAME], [REGISTRATION NUMBER], [REGISTERED ADDRESS], an EU (Lithuanian) entity</span>
      ("we", "us"). For payment processing, Paddle.com Market Limited ("Paddle") acts as an independent data
      controller in its own right — see Section 6.</p>

    <h2>2. What we collect</h2>
    <ul>
      <li><strong>Billing email</strong> — the address you give when claiming a mailbox, used to send your
        payment link and mailbox notifications, and passed to Paddle to process your order.</li>
      <li><strong>Mailbox address and key fingerprint</strong> — your assigned mailbox address and the
        fingerprint of the Ed25519 key that identifies it. We never see or store your private key.</li>
      <li><strong>Inbound email content</strong> — the body and headers of mail delivered to your mailbox, so
        you can read it over IMAP or the HTTP API.</li>
      <li><strong>IMAP/API access logs</strong> — login timestamps and source IPs, kept for authentication and
        abuse/security monitoring.</li>
    </ul>
    <p>We do not read or use your inbound mail content for advertising or profiling, and the Service has no
      capability to send mail on your behalf.</p>

    <h2>3. Why we process it (lawful basis)</h2>
    <ul>
      <li><strong>Performance of a contract</strong> (GDPR Art. 6(1)(b)) — provisioning and operating your
        mailbox, billing you for it.</li>
      <li><strong>Legitimate interests</strong> (Art. 6(1)(f)) — access logging for account security and abuse
        prevention.</li>
      <li><strong>Legal obligation</strong> (Art. 6(1)(c)) — accounting and tax records Paddle keeps for your
        payments.</li>
    </ul>

    <h2>4. Retention</h2>
    <p>Mailbox content and access credentials are retained for as long as your mailbox stays active, plus a
      limited grace period after expiry to allow reactivation before deletion. Access logs are retained only
      as long as needed for security and abuse investigation, then deleted or aggregated. Billing records are
      retained by Paddle for as long as applicable tax law requires.</p>

    <h2>5. Your rights</h2>
    <p>Under the GDPR you have the right to access the personal data we hold about you, correct inaccurate
      data, request erasure, restrict or object to processing, and receive a copy of your data in a portable
      format. To exercise any of these, use our <a href="/contact">Contact</a> page. Requests relating
      specifically to payment/billing data held by Paddle should also be directed to Paddle, since Paddle acts
      as an independent controller for that data.</p>
    <p>You also have the right to lodge a complaint with your local data protection authority — in Lithuania,
      the <a href="https://vdai.lrv.lt/">State Data Protection Inspectorate (Valstybinė duomenų apsaugos
      inspekcija)</a>.</p>

    <h2>6. Paddle as an independent controller</h2>
    <p>Paddle processes your billing details (name, billing address, payment instrument, transaction history)
      as an independent data controller for payment processing, fraud prevention, and tax compliance. Paddle's
      own <a href="https://www.paddle.com/legal/privacy">Privacy Policy</a> governs that processing.</p>

    <h2>7. International transfers</h2>
    <p>Paddle may process billing data outside the EEA under its own documented safeguards (such as Standard
      Contractual Clauses) — see Paddle's Privacy Policy for details. We do not otherwise transfer inbound mail
      content outside the EEA.</p>

    <h2>8. Security</h2>
    <p>Mailbox access requires possession of the private key matching your registered Ed25519 public key; the
      Service does not use passwords for this. We apply reasonable technical and organisational measures to
      protect stored mail and access credentials, but no service can guarantee absolute security.</p>

    <h2>9. Changes</h2>
    <p>We may update this notice from time to time; the "Last updated" date above reflects the current
      version.</p>
`

const contactBodyHTML = `
    <h2>Agents with an existing mailbox</h2>
    <p>If you already have a claimed mailbox, the fastest way to reach us is the signed support endpoint built
      into the API:</p>
    <div class="card">
      <p><code>POST /v1/support/messages</code></p>
      <p>Send a JSON body with your <code>edproof</code>, a fresh <code>challenge</code>/<code>signature</code>
        pair (see the <a href="/docs/agent-api-skill.md">agent API skill doc</a>), a <code>subject</code>, and
        a <code>body</code>. Your message is tied to your mailbox, so we have relevant account context
        immediately.</p>
    </div>

    <h2>Everyone else</h2>
    <p>General enquiries, press, and anything not tied to a specific mailbox:
      <a href="mailto:hi@truevipaccess.com">hi@truevipaccess.com</a>.</p>

    <h2>Billing and payment questions</h2>
    <p>Paddle.com Market Limited is the merchant of record for every purchase on this site and handles
      billing, invoices, and payment-related enquiries directly. If your question is about a specific charge,
      refund, or invoice, contact Paddle via the receipt email they sent you, or
      <a href="https://www.paddle.com/help">paddle.com/help</a>. See our
      <a href="/refund-policy">Refund Policy</a> for the refund process.</p>

    <h2>Business details</h2>
    <p><span class="todo"><!-- TODO(legal): replace before submitting to Paddle for domain approval -->
      [COMPANY LEGAL NAME], [REGISTRATION NUMBER], [REGISTERED ADDRESS]</span></p>
`
