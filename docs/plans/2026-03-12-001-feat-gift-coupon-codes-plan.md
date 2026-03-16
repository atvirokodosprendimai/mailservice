---
title: "feat: Add gift coupon codes for 3-month free mailbox"
type: feat
status: completed
date: 2026-03-12
origin: docs/brainstorms/2026-03-12-gift-coupons-brainstorm.md
---

# feat: Add gift coupon codes for 3-month free mailbox

## Overview

Add a promo code system that lets the owner distribute a gift code granting 3 months of free mailbox service.
Immediate goal: a single code ("OPENCLAWS") usable by up to 23 openclaws community members.
Codes are Polar-native discounts — created in Polar, validated by Polar, passed through the claim flow as `discount_id` on checkout creation.
(See brainstorm: `docs/brainstorms/2026-03-12-gift-coupons-brainstorm.md`)

## Problem Statement / Motivation

The project needs a way to onboard early adopters from the openclaws community without requiring payment.
The current system has no free tier and no promo code support — every mailbox requires a paid checkout.
Gift codes let the owner fund trial access while maintaining a clean billing paper trail through Polar's $0 checkout.

## Proposed Solution

**Single Polar discount with `max_uses: 23`**, stored as a config value (`POLAR_GIFT_DISCOUNT_ID`).

### User Flow

```
1. Owner creates a 100% Polar discount (code: "OPENCLAWS", max_uses: 23)
2. Owner shares the code word with openclaws members
3. Recipient: POST /v1/mailboxes/claim {billing_email, edproof, challenge, signature, coupon_code: "OPENCLAWS"}
4. Server: validates coupon_code matches config → creates Polar checkout with discount_id pre-applied
5. Recipient: completes $0 Polar checkout (discount already applied, no payment needed)
6. Polar webhook fires → MarkMailboxPaid → mailbox active for 3 months
7. After 3 months: mailbox expires, user can re-claim with normal payment (1 month)
```

### Why Single Discount with max_uses

- For 23 codes, individual single-use codes add mapping complexity with no real benefit
- One code ("OPENCLAWS") is easier to distribute and remember
- Polar enforces the usage cap (max_uses: 23)
- If individual tracking is needed later, create per-user codes — the server-side plumbing is the same
- (See brainstorm: key decision #2 — originally 23 unique codes, simplified to single multi-use)

## Technical Considerations

### Architecture: Extension Points (file:line)

| What | Where | Change |
|------|-------|--------|
| Claim request struct | `internal/adapters/httpapi/handler.go:658` | Add `CouponCode string` field |
| Claim handler | `internal/adapters/httpapi/handler.go:665` | Pass coupon code to service |
| ClaimMailbox service | `internal/core/service/mailbox_service.go:74` | Accept coupon, validate, set GrantedMonths |
| PaymentLinkRequest | `internal/core/ports/ports.go:74` | Add `DiscountID string` field |
| PolarGateway.CreatePaymentLink | `internal/adapters/payment/polar_gateway.go:52` | Include `discount_id` in checkout payload |
| Mailbox domain | `internal/domain/mailbox.go:13` | Add `GrantedMonths int` field |
| MarkMailboxPaid | `internal/core/service/mailbox_service.go:276` | Use `mailbox.GrantedMonths` instead of hardcoded 1 |
| MockGateway | `internal/adapters/payment/stripe_gateway.go:109` | Accept and ignore DiscountID |
| StripeGateway | `internal/adapters/payment/stripe_gateway.go` | Return error if DiscountID provided |
| Config | `internal/platform/config/config.go` | Add `PolarGiftDiscountID` |

### Duration Signal: GrantedMonths on Mailbox

The brainstorm decided "detect from Polar webhook," but storing `GrantedMonths` on the Mailbox at claim time is simpler and more robust:
- Service sets `GrantedMonths = 3` when coupon is valid
- `MarkMailboxPaid` reads `mailbox.GrantedMonths` (default 0 → treated as 1 for backward compatibility)
- No dependency on Polar returning discount metadata in webhook/session
- Business logic stays in the service layer, not in the adapter

### Coupon Validation Strategy

**Minimal server-side + Polar enforcement:**
1. Config: `POLAR_GIFT_DISCOUNT_ID` (UUID) and `POLAR_GIFT_COUPON_CODE` (e.g., "OPENCLAWS")
2. At claim: if `coupon_code` matches `POLAR_GIFT_COUPON_CODE` (case-insensitive), use the discount_id
3. If no match, return 422 "invalid coupon code"
4. **Per-user dedup**: check if the claiming key fingerprint already has a mailbox with `GrantedMonths > 1` → if yes, return 409 "coupon already used by this key"
5. Polar enforces max_uses at checkout creation — if exhausted, checkout creation fails → return 410 "coupon expired or exhausted"
6. No local redemption tracking table — dedup uses existing mailbox data, Polar tracks global usage

This avoids a coupon database table, race conditions, and atomic redemption logic for a 23-code use case.
Per-user enforcement uses the existing mailbox lookup by key fingerprint — no new queries.

### Security

- Coupon code validation is case-insensitive string match (no brute-force risk — it's a known code shared publicly)
- SEC-003 (1 MB body limit) and SEC-004 (strict JSON) already cover the claim endpoint
- No new secrets — `POLAR_GIFT_DISCOUNT_ID` is not secret (it's a UUID, not a token)
- The coupon doesn't bypass auth — ed25519 proof is still required

## System-Wide Impact

- **Interaction graph**: Claim handler → ClaimMailbox (+ coupon validation) → CreatePaymentLink (+ discount_id) → Polar API → webhook → MarkMailboxPaid (uses GrantedMonths)
- **Error propagation**: Polar discount rejection → CreatePaymentLink error → claim handler returns 410/422. Normal error wrapping chain.
- **State lifecycle risks**: If coupon is accepted but user never completes checkout → mailbox stays `pending_payment` with GrantedMonths=3. No harm — GrantedMonths only takes effect on activation.
- **API surface parity**: Only the claim endpoint changes. Webhook handler and success redirect are unchanged (they call MarkMailboxPaid which reads GrantedMonths from the mailbox).
- **Integration test scenarios**: (1) Claim with valid coupon → $0 checkout → 3-month activation. (2) Claim with invalid coupon → 422 error. (3) Claim without coupon → normal 1-month flow unchanged. (4) Re-claim expired gifted mailbox without coupon → normal payment, 1 month. (5) Coupon exhausted (max_uses reached) → Polar rejects → 410. (6) Same key claims with coupon twice → 409 "already used".

## Acceptance Criteria

- [x] `POST /v1/mailboxes/claim` accepts optional `coupon_code` field
- [x] Valid coupon code creates a Polar checkout with 100% discount pre-applied
- [x] $0 checkout completion activates mailbox for 3 months (not 1)
- [x] Invalid coupon code returns 422 with clear error message
- [x] Same key fingerprint using coupon twice returns 409 "coupon already used by this key"
- [x] Exhausted coupon (Polar rejects discount) returns 410
- [x] Claim without coupon code works exactly as before (1 month)
- [x] Re-claim of expired gifted mailbox (no coupon) follows normal payment flow
- [x] Re-claim of expired mailbox WITH coupon code returns 409 if coupon was already used by this key
- [x] `GrantedMonths` field on Mailbox domain persisted to database
- [x] MockGateway handles DiscountID gracefully for tests
- [x] Config: `POLAR_GIFT_DISCOUNT_ID` and `POLAR_GIFT_COUPON_CODE` env vars
- [x] All deploy workflows updated with new env vars
- [x] Unit tests for coupon validation logic, GrantedMonths in MarkMailboxPaid
- [x] Notification email unchanged (same payment link, Polar shows $0)

## Success Metrics

- 23 openclaws members can claim mailboxes with "OPENCLAWS" code
- Each gifted mailbox shows ExpiresAt = claim_date + 3 months
- Polar dashboard shows $0 orders for each gifted checkout
- Existing non-coupon claim flow is unaffected

## Dependencies & Risks

| Risk | Mitigation |
|------|-----------|
| Polar rejects `discount_id` at checkout creation | Test in Polar sandbox first; verify API contract |
| Config vars missing from deploy workflows | Checklist: add to ALL deploy-*.yml files (learnings: `docs/solutions/integration-issues/missing-edproof-hmac-secret-in-smoke-deploy.md`) |
| GrantedMonths=0 for existing mailboxes after schema change | Default 0 → treated as 1 in MarkMailboxPaid (backward compatible) |
| Polar sandbox webhook latency (up to 90s) | Existing smoke test already handles this (learnings: sandbox testing pattern) |

## Implementation Phases

### Phase 1: Domain & Port Layer

**Goal:** Extend the core types to support discount codes and variable grant duration.

- [x] Add `GrantedMonths int` field to `domain.Mailbox` (`internal/domain/mailbox.go`)
  - Default 0 (backward compatible: 0 treated as 1 month in MarkMailboxPaid)
- [x] Add `DiscountID string` field to `ports.PaymentLinkRequest` (`internal/core/ports/ports.go`)
- [x] Add coupon sentinel errors to ports: `ErrCouponInvalid`, `ErrCouponExhausted`, `ErrCouponAlreadyUsed`
- [x] Add `PolarGiftDiscountID string` and `PolarGiftCouponCode string` to config struct (`internal/platform/config/config.go`)
  - Both optional — feature disabled when empty
- [x] Add database migration: `ALTER TABLE mailboxes ADD COLUMN granted_months INTEGER DEFAULT 0`
- [x] Write unit tests:
  - `mailbox_test.go`: GrantedMonths field preserved through create/update
  - `config_test.go`: optional gift config loading

### Phase 2: Service Layer

**Goal:** Wire coupon validation into ClaimMailbox and variable duration into MarkMailboxPaid.

- [x] Extend `ClaimMailbox` signature: add `couponCode string` parameter
  - If `couponCode` is non-empty and config has gift settings:
    - Normalize: `strings.TrimSpace(strings.ToUpper(couponCode))`
    - Compare to `config.PolarGiftCouponCode` (also uppercased)
    - If no match: return `ErrCouponInvalid`
    - **Per-user dedup**: check if existing mailbox for this key fingerprint has `GrantedMonths > 1` → return `ErrCouponAlreadyUsed`
    - If match + not already used: set `PaymentLinkRequest.DiscountID = config.PolarGiftDiscountID` and `mailbox.GrantedMonths = 3`
  - If `couponCode` is non-empty but gift config is empty: return `ErrCouponInvalid`
  - If `couponCode` is empty: normal flow, no discount
- [x] Update re-claim branch (existing mailbox, not usable): same coupon logic when creating new payment link
- [x] Update `MarkMailboxPaid` (`internal/core/service/mailbox_service.go:276`):
  - Read `mailbox.GrantedMonths`
  - `months := mailbox.GrantedMonths; if months == 0 { months = 1 }`
  - `nextExpiry := base.AddDate(0, months, 0)`
- [x] Write unit tests:
  - `mailbox_service_test.go`: ClaimMailbox with valid coupon → DiscountID passed, GrantedMonths=3
  - `mailbox_service_test.go`: ClaimMailbox with invalid coupon → ErrCouponInvalid
  - `mailbox_service_test.go`: ClaimMailbox with coupon already used by same key → ErrCouponAlreadyUsed
  - `mailbox_service_test.go`: ClaimMailbox without coupon → normal flow
  - `mailbox_service_test.go`: MarkMailboxPaid with GrantedMonths=3 → ExpiresAt is +3 months
  - `mailbox_service_test.go`: MarkMailboxPaid with GrantedMonths=0 → ExpiresAt is +1 month (backward compat)

### Phase 3: Adapter Layer

**Goal:** Pass discount_id through Polar checkout and handle errors.

- [x] Update `PolarGateway.CreatePaymentLink` (`internal/adapters/payment/polar_gateway.go`):
  - If `req.DiscountID != ""`: add `"discount_id": req.DiscountID` to checkout payload
  - Handle Polar error when discount is exhausted/invalid → wrap as `ErrCouponExhausted`
- [x] Update `MockGateway.CreatePaymentLink`:
  - Accept and ignore DiscountID (log it for test visibility)
- [x] Update `StripeGateway.CreatePaymentLink`:
  - If `req.DiscountID != ""`: return `fmt.Errorf("discount codes not supported with Stripe gateway")`
- [x] Update claim handler (`internal/adapters/httpapi/handler.go`):
  - Add `CouponCode string \`json:"coupon_code,omitempty"\`` to `claimMailboxRequest`
  - Pass `req.CouponCode` to `ClaimMailbox`
  - Map `ErrCouponInvalid` → HTTP 422 `{"error": "invalid coupon code"}`
  - Map `ErrCouponAlreadyUsed` → HTTP 409 `{"error": "coupon already used by this key"}`
  - Map `ErrCouponExhausted` → HTTP 410 `{"error": "coupon expired or exhausted"}`
- [x] Update claim response (optional): add `granted_months` to `mailboxView` response
- [x] Write unit/integration tests:
  - `polar_gateway_test.go`: CreatePaymentLink with DiscountID → payload includes `discount_id`
  - `handler_test.go`: claim with invalid coupon → 422
  - `handler_test.go`: claim with valid coupon → 200, payment link created with discount

### Phase 4: Config & Deploy

**Goal:** Wire config values and update all deployment workflows.

- [x] Add `POLAR_GIFT_DISCOUNT_ID` and `POLAR_GIFT_COUPON_CODE` to env loading in `config.go`
- [x] Create the Polar discount via API or dashboard:
  - Code: `OPENCLAWS`, percentage: 100, max_uses: 23
  - Production discount_id: `a216b04f-606f-4c33-a6aa-df0b9ae4518a`
- [x] Create sandbox Polar discount for testing (same params)
  - Sandbox discount_id: `c0cd2e40-c2b1-4b18-bae3-c8b280f969e5`
- [x] Update ALL deploy workflows with new env vars:
  - Production deploy
  - Smoke/sandbox deploy
  - Any other deploy-*.yml files
  - **Critical:** compare all deploy workflow env var lists for drift (learnings: `docs/solutions/integration-issues/missing-edproof-hmac-secret-in-smoke-deploy.md`)
- [x] Set GitHub secrets/variables for `POLAR_GIFT_DISCOUNT_ID` and `POLAR_GIFT_COUPON_CODE`

### Phase 5: E2E Verification

**Goal:** Verify the full flow works in sandbox.

- [x] Run smoke test with coupon code against sandbox instance => GH Actions run #23140408725 passed (2026-03-16)
- [x] Verify: claim with "OPENCLAWS" → $0 Polar checkout → webhook → mailbox active, ExpiresAt = +3 months => confirmed via smoke-sandbox auto-pay with discount code
- [x] Verify: claim without coupon → normal payment flow, ExpiresAt = +1 month => confirmed via prior smoke run #23060192524 (2026-03-13)
- [ ] Verify: Polar dashboard shows $0 order with discount applied => requires manual browser check
- [ ] Verify: after 23 uses, next claim with coupon returns 410 => requires burning real coupon slots, deferred

## Sources & References

### Origin

- **Brainstorm document:** [docs/brainstorms/2026-03-12-gift-coupons-brainstorm.md](docs/brainstorms/2026-03-12-gift-coupons-brainstorm.md)
  - Key decisions: Polar-native discounts, $0 checkout flow, 3-month duration from webhook/mailbox state

### Internal References

- Claim handler: `internal/adapters/httpapi/handler.go:658-698`
- ClaimMailbox service: `internal/core/service/mailbox_service.go:74-157`
- MarkMailboxPaid: `internal/core/service/mailbox_service.go:255-319`
- PolarGateway: `internal/adapters/payment/polar_gateway.go:52-83`
- PaymentLinkRequest: `internal/core/ports/ports.go:74-77`
- Mailbox domain: `internal/domain/mailbox.go:13-41`
- Config: `internal/platform/config/config.go`
- Deploy config lesson: `docs/solutions/integration-issues/missing-edproof-hmac-secret-in-smoke-deploy.md`

### External References

- Polar Discounts API: `POST /v1/discounts/` (create), checkout `discount_id` field
- Polar Checkout Sessions API: `POST /v1/checkouts/` with `discount_id` parameter
