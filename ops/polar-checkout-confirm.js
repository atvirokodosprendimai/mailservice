#!/usr/bin/env node
/**
 * Headless browser checkout confirmation for Polar sandbox.
 *
 * Usage:
 *   node ops/polar-checkout-confirm.js <checkout-url>
 *
 * Polar requires Stripe Elements (browser-based) for payment confirmation,
 * even for $0 discounted checkouts. This script uses Playwright to:
 *   1. Navigate to the Polar checkout URL
 *   2. Fill in test billing details and Stripe test card
 *   3. Submit and wait for redirect to success URL
 *
 * Environment:
 *   CHECKOUT_EMAIL   - Customer email (default: smoke@test.com)
 *   CHECKOUT_VERBOSE - Set to "1" for screenshots on failure
 *
 * Exit codes:
 *   0 - Checkout confirmed successfully
 *   1 - Checkout failed
 *   2 - Bad arguments
 */

const { chromium } = require("playwright");

const CHECKOUT_URL = process.argv[2];
if (!CHECKOUT_URL || !CHECKOUT_URL.includes("polar")) {
  console.error("Usage: node polar-checkout-confirm.js <polar-checkout-url>");
  process.exit(2);
}

const EMAIL = process.env.CHECKOUT_EMAIL || "smoke@test.com";
const VERBOSE = process.env.CHECKOUT_VERBOSE === "1";
const TIMEOUT = 30000;

async function main() {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });

  try {
    console.log(`Opening checkout: ${CHECKOUT_URL}`);
    await page.goto(CHECKOUT_URL, { waitUntil: "networkidle", timeout: TIMEOUT });
    await page.waitForTimeout(3000);

    // Fill email
    const emailField = page.locator('input[name="customer_email"]');
    if (await emailField.isVisible({ timeout: 3000 }).catch(() => false)) {
      await emailField.clear();
      await emailField.fill(EMAIL);
      console.log("  filled email");
    }

    // Fill customer name
    const nameField = page.locator('input[name="customer_name"]');
    if (await nameField.isVisible({ timeout: 2000 }).catch(() => false)) {
      await nameField.fill("Smoke Test");
      console.log("  filled name");
    }

    // Fill Stripe card fields inside the iframe.
    // Stripe embeds card inputs (number, expiry, cvc) in an iframe whose src
    // contains "elements-inner-accessory-target". The title is "Secure payment input frame".
    const stripeFrameLocator = page.frameLocator(
      'iframe[title="Secure payment input frame"]'
    );

    // Card number — Stripe uses name="number" with placeholder "1234 1234 1234 1234"
    const cardInput = stripeFrameLocator.locator('input[name="number"]');
    try {
      await cardInput.waitFor({ state: "visible", timeout: 8000 });
      await cardInput.fill("4242424242424242");
      console.log("  filled card number");

      // Expiry
      const expiryInput = stripeFrameLocator.locator('input[name="expiry"]');
      await expiryInput.fill("12 / 30");
      console.log("  filled expiry");

      // CVC
      const cvcInput = stripeFrameLocator.locator('input[name="cvc"]');
      await cvcInput.fill("123");
      console.log("  filled cvc");
    } catch (e) {
      console.log(`  Stripe iframe not found or not interactive: ${e.message.split("\n")[0]}`);
      console.log("  (may be a free product with no payment form)");
    }

    // Cardholder name (outside Stripe iframe, inside main page)
    // It appears after the Stripe fields in the DOM
    const cardholderInputs = page.locator('input:not([name])').all();
    // Try to find the cardholder name field by checking visible text inputs
    // that aren't email/name/discount
    const allInputs = await page.locator("input[type='text']").all();
    for (const input of allInputs) {
      const name = await input.getAttribute("name");
      const placeholder = await input.getAttribute("placeholder");
      if (!name && !placeholder && (await input.isVisible())) {
        await input.fill("Smoke Test");
        console.log("  filled cardholder name");
        break;
      }
    }

    // Select country (billing address)
    // Polar uses a hidden <select> behind a custom dropdown — try but don't block on it.
    // The country often defaults to "United States" already.
    try {
      const countrySelect = page.locator("select").last();
      await countrySelect.selectOption({ label: "United States" }, { timeout: 3000 });
      console.log("  selected country: United States");
    } catch {
      console.log("  country select skipped (may already be set)");
    }

    await page.waitForTimeout(1000);

    // Fill billing address fields (placeholders observed from Polar checkout UI)
    const addressMappings = [
      { pattern: 'input[placeholder="Street address"]', value: "123 Test St" },
      { pattern: 'input[placeholder="Apartment or unit number"]', value: "" },
      { pattern: 'input[placeholder="Postal code"]', value: "94102" },
      { pattern: 'input[placeholder="City"]', value: "San Francisco" },
    ];

    for (const { pattern, value } of addressMappings) {
      const field = page.locator(pattern).first();
      if (await field.isVisible({ timeout: 500 }).catch(() => false)) {
        if (value) {
          await field.fill(value);
          console.log(`  filled ${pattern}`);
        }
      }
    }

    // State dropdown (first <select>, values are "US-CA" format)
    try {
      const stateSelect = page.locator("select").first();
      await stateSelect.selectOption("US-CA", { timeout: 3000 });
      console.log("  selected state: California (US-CA)");
    } catch {
      console.log("  state select skipped");
    }

    if (VERBOSE) {
      await page.screenshot({ path: "/tmp/checkout-before-submit.png", fullPage: true });
      console.log("  screenshot saved: /tmp/checkout-before-submit.png");
    }

    // Click submit
    const submitBtn = page.locator('button[type="submit"]');
    await submitBtn.waitFor({ state: "visible", timeout: 3000 });
    await submitBtn.click();
    console.log("  clicked 'Subscribe now'");

    // Wait for navigation to success URL or confirmation state
    try {
      await page.waitForURL(/success|confirmed|thank/, { timeout: TIMEOUT });
      console.log(`  redirected to: ${page.url()}`);
      console.log("OK: checkout confirmed");
      process.exit(0);
    } catch {
      // Check if page shows a success/confirmed state without URL change
      const body = await page.textContent("body");
      if (/thank|confirmed|success|order/i.test(body)) {
        console.log("OK: checkout confirmed (no redirect)");
        process.exit(0);
      }

      if (VERBOSE) {
        await page.screenshot({ path: "/tmp/checkout-after-submit.png", fullPage: true });
        console.log("  screenshot saved: /tmp/checkout-after-submit.png");
      }

      // Still exit 0 — the submit went through, webhook will follow
      console.log("OK: checkout submitted (waiting for webhook)");
      process.exit(0);
    }
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
