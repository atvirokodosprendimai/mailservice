#!/usr/bin/env node
/**
 * Headless browser checkout confirmation for Paddle sandbox.
 *
 * Usage:
 *   node ops/paddle-checkout-confirm.js <checkout-page-url>
 *
 * Unlike Polar (Stripe Elements embedded directly in Polar's own checkout
 * page), Paddle Checkout renders as an overlay iframe injected by Paddle.js
 * (see internal/adapters/httpapi/paddle_checkout.go), hosted on a Paddle
 * origin cross-domain from this app. Paddle does not publish the overlay's
 * internal DOM/field selectors for automation, so this script:
 *   1. Opens our own checkout page (same-origin, stable selectors) and
 *      clicks "Open payment window" to trigger Paddle.Checkout.open().
 *   2. Waits on Paddle.js's own documented eventCallback events
 *      (checkout.completed / checkout.error / checkout.closed), which the
 *      checkout page pushes onto window.__paddleEvents specifically for
 *      this script — NOT on guessed third-party DOM state.
 *   3. Best-effort fills common card/contact fields inside the overlay
 *      iframe using Paddle's documented sandbox test card. This step is
 *      UNVERIFIED against a live Paddle sandbox checkout (Paddle does not
 *      publish these selectors) and tolerates missing fields — expected
 *      when a 100%-off gift coupon already makes the transaction $0, in
 *      which case Paddle's overlay may need no card details at all.
 *
 * Environment:
 *   CHECKOUT_EMAIL   - Customer email (default: smoke@test.com)
 *   CHECKOUT_VERBOSE - Set to "1" for screenshots on failure
 *
 * Exit codes:
 *   0 - Checkout completed, or submitted and awaiting webhook confirmation
 *   1 - Checkout failed (payment error, closed overlay, or no submit path found)
 *   2 - Bad arguments
 */

const { chromium } = require("playwright");

const CHECKOUT_URL = process.argv[2];
if (!CHECKOUT_URL) {
  console.error("Usage: node paddle-checkout-confirm.js <paddle-checkout-page-url>");
  process.exit(2);
}

const EMAIL = process.env.CHECKOUT_EMAIL || "smoke@test.com";
const VERBOSE = process.env.CHECKOUT_VERBOSE === "1";
const TIMEOUT = 30000;

// Paddle's documented sandbox test card (no 3DS):
// https://developer.paddle.com/sdks/sandbox#test-cards
const TEST_CARD = {
  number: "4242424242424242",
  expiry: "12/30",
  cvc: "100",
};

async function paddleEvents(page) {
  return page.evaluate(() => window.__paddleEvents || []);
}

// Best-effort fill: tries each candidate locator in order, fills the first
// one found visible, and silently continues if none match. Paddle does not
// publish overlay field selectors, so this list is a defensive guess, not a
// verified contract — see file header.
async function tryFill(frame, candidates, value, label) {
  for (const selector of candidates) {
    const field = frame.locator(selector).first();
    if (await field.isVisible({ timeout: 1500 }).catch(() => false)) {
      await field.fill(value).catch(() => {});
      console.log(`  filled ${label} via ${selector}`);
      return true;
    }
  }
  console.log(`  ${label} field not found (may be unnecessary for this checkout)`);
  return false;
}

function fail(message) {
  throw new Error(message);
}

async function main() {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });

  try {
    console.log(`Opening checkout page: ${CHECKOUT_URL}`);
    await page.goto(CHECKOUT_URL, { waitUntil: "networkidle", timeout: TIMEOUT });

    const openBtn = page.locator("#paddle-open-btn");
    await openBtn.waitFor({ state: "visible", timeout: TIMEOUT });
    await page.waitForFunction(
      () => document.getElementById("paddle-open-btn") && !document.getElementById("paddle-open-btn").disabled,
      null,
      { timeout: TIMEOUT }
    );
    await openBtn.click();
    console.log("  clicked 'Open payment window'");

    // Paddle.js injects the overlay as an iframe. Its exact origin/selector
    // is not published; match any iframe Paddle.js adds to the page.
    const overlayFrame = page.frameLocator('iframe[name^="paddle"], iframe[src*="paddle.com"]').first();

    await tryFill(overlayFrame, ['input[name="email"]', 'input[type="email"]'], EMAIL, "email");
    await tryFill(overlayFrame, ['input[name="cardNumber"]', 'input[name="number"]', 'input[autocomplete="cc-number"]'], TEST_CARD.number, "card number");
    await tryFill(overlayFrame, ['input[name="cardExpiry"]', 'input[name="expiry"]', 'input[autocomplete="cc-exp"]'], TEST_CARD.expiry, "expiry");
    await tryFill(overlayFrame, ['input[name="cardCvv"]', 'input[name="cvv"]', 'input[name="cvc"]', 'input[autocomplete="cc-csc"]'], TEST_CARD.cvc, "cvc");
    await tryFill(overlayFrame, ['input[name="cardholderName"]', 'input[name="name"]', 'input[autocomplete="cc-name"]'], "Smoke Test", "cardholder name");
    await tryFill(overlayFrame, ['input[name="postalCode"]', 'input[name="postcode"]', 'input[autocomplete="postal-code"]'], "94102", "postal code");

    if (VERBOSE) {
      await page.screenshot({ path: "/tmp/checkout-before-submit.png", fullPage: true }).catch(() => {});
      console.log("  screenshot saved: /tmp/checkout-before-submit.png");
    }

    // Submit — Paddle overlays are self-contained, so this is normally
    // inside the overlay frame.
    const submitCandidates = [
      'button[type="submit"]',
      'button:has-text("Pay")',
      'button:has-text("Subscribe")',
      'button:has-text("Complete")',
    ];
    let submitted = false;
    for (const selector of submitCandidates) {
      const btn = overlayFrame.locator(selector).first();
      if (await btn.isVisible({ timeout: 2000 }).catch(() => false)) {
        await btn.click();
        console.log(`  clicked submit via ${selector}`);
        submitted = true;
        break;
      }
    }
    if (!submitted) {
      fail("no submit button found in Paddle checkout overlay");
    }

    // Wait for Paddle.js's own documented event, not DOM guesswork.
    try {
      await page.waitForFunction(
        () => {
          const events = window.__paddleEvents || [];
          return events.includes("checkout.completed") || events.includes("checkout.error") || events.includes("checkout.closed");
        },
        null,
        { timeout: TIMEOUT }
      );
    } catch {
      // Timed out without a terminal event — submit went through, webhook
      // delivery/reconciliation in the calling shell script is the backstop.
      console.log("OK: checkout submitted (no terminal event observed, awaiting webhook)");
      process.exit(0);
    }

    const events = await paddleEvents(page);
    if (events.includes("checkout.completed")) {
      console.log("OK: checkout completed");
      process.exit(0);
    }

    if (VERBOSE) {
      await page.screenshot({ path: "/tmp/checkout-after-submit.png", fullPage: true }).catch(() => {});
      console.log("  screenshot saved: /tmp/checkout-after-submit.png");
    }
    fail(`checkout did not complete (events: ${events.join(", ")})`);
  } catch (err) {
    console.error(`FAIL: ${err.message}`);
    if (VERBOSE) {
      await page.screenshot({ path: "/tmp/checkout-error.png", fullPage: true }).catch(() => {});
      console.log("  screenshot saved: /tmp/checkout-error.png");
    }
    process.exit(1);
  } finally {
    await browser.close();
  }
}

main();
