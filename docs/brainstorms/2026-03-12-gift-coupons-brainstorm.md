# Brainstorm: Gift Coupons (Clawpal Codes)

**Date:** 2026-03-12
**Status:** Ready for planning

## What We're Building

A promo code system that lets the project owner generate single-use gift codes granting 3 months of free mailbox service.
The immediate use case: distribute ~23 codes to the openclaws community so they can try the service.

### User Flow

1. **Owner** creates 23 single-use 100% discount codes in Polar (via dashboard or API)
2. **Owner** distributes codes to openclaws members
3. **Recipient** claims a mailbox via the existing claim flow, entering the promo code in a new field
4. **Server** creates a Polar checkout session with the discount code applied → $0 checkout
5. **Recipient** completes the $0 Polar checkout (clean paper trail, no payment required)
6. **Polar webhook** fires → mailbox activated for **3 months** instead of the standard 1 month

## Why This Approach

**Polar-native discounts** over server-side code management because:

- Polar already handles code validation, single-use enforcement, and usage tracking
- Minimal server-side changes — just pass the discount code through to checkout creation
- Clean billing paper trail: every "gift" shows up as a $0 order in Polar dashboard
- No new database tables, no new domain entities for codes
- YAGNI: for 23 codes, building our own coupon infrastructure is over-engineering

## Key Decisions

1. **Polar-native discounts** — not server-side code storage.
   Codes are created and validated entirely by Polar.

2. **Single-use codes** — each of the 23 codes works exactly once.
   Created with `max_uses: 1` in Polar.

3. **$0 Polar checkout** — recipients still go through checkout flow.
   Not instant activation.
   This gives a paper trail and consistent UX.

4. **3-month duration** — gift mailboxes get `ExpiresAt = now + 3 months` instead of `now + 1 month`.
   This is handled server-side in `MarkMailboxPaid` based on discount metadata or a separate flag.

5. **Promo code field at claim time** — added to the claim API request.
   The field is optional; existing flow is unchanged when no code is provided.

## Technical Integration Points

### Polar API

- **Create discount:** `POST /v1/discounts/` with `percentage: 100`, `max_uses: 1`, `code: "CLAWPAL-XXXX"`
- **Checkout creation:** `POST /v1/checkouts/` already used — add `discount_id` or pass `allow_discount_codes: true`
- **Checkout links:** support `?discount_code=CODE` query param for pre-filled codes

### Server Changes

- **Claim API** (`/v1/mailboxes/claim`): add optional `promo_code` field to request body
- **PaymentLinkRequest**: extend with optional `DiscountCode string`
- **PolarGateway.CreatePaymentLink**: pass discount code to Polar checkout creation
- **MarkMailboxPaid**: detect $0 / discounted orders and set 3-month expiry instead of 1-month

### Resolved: Duration Signal

**Decision:** Detect from Polar webhook payload.
When the webhook fires, check for discount metadata — if a 100% discount is present, set `ExpiresAt = now + 3 months` instead of the standard 1 month.
No domain model changes required.

## Scope Boundaries

**In scope:**
- Create 23 Polar discount codes (100%, single-use)
- Add promo code field to claim flow
- Pass discount code through to Polar checkout
- Handle 3-month expiry for gift mailboxes
- Basic validation (code format, pass-through to Polar for actual validation)

**Out of scope:**
- Admin UI for managing codes
- Self-service code purchase by "pay pals"
- Multi-tier gift durations (only 3 months)
- Referral/affiliate tracking
- Email notifications about gift redemption
