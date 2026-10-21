"""SWE302 Assignment 3 - system (end-to-end) tests with Playwright.

They drive a real Chromium browser against the running store, so every
layer is exercised: HTML/JS front end -> HTTP gateway -> ten gRPC services.

Start the system:  ./run-all.sh test
Run:               pytest tests/e2e -v
Screenshots of each step are saved to tests/e2e/screens/ as evidence.
"""
import os
import re
import urllib.request

import pytest
from playwright.sync_api import expect, sync_playwright

BASE = os.environ.get("GATEWAY_URL", "http://localhost:8080")
SHOTS = os.path.join(os.path.dirname(__file__), "screens")


def reset_data():
    """Put every service back to its seed data (test mode only)."""
    req = urllib.request.Request(BASE + "/api/test/reset", method="POST")
    assert urllib.request.urlopen(req, timeout=5).status == 200


@pytest.fixture
def page():
    os.makedirs(SHOTS, exist_ok=True)
    reset_data()
    with sync_playwright() as p:
        browser = p.chromium.launch()
        pg = browser.new_page(viewport={"width": 1280, "height": 860})
        yield pg
        browser.close()


def shot(page, name):
    page.screenshot(path=os.path.join(SHOTS, name + ".png"))


def test_TC21_full_shopping_workflow(page):
    """TC21 (FR1-FR8): browse -> add to cart -> delivery quote -> coupon ->
    checkout -> order history -> parcel tracking, as customer C002."""
    page.goto(BASE + "/")
    expect(page.locator("[data-cy=featured-P001]")).to_be_visible()
    shot(page, "01_home")

    # Shop: add 2 Mechanical Keyboards (P002, Nu. 4,500 each) to the cart.
    page.goto(BASE + "/shop.html")
    page.fill("[data-cy=customer-id]", "C002")
    card = page.locator("[data-cy=product-card-P002]")
    card.locator("[data-cy=qty-input]").fill("2")
    card.locator("[data-cy=add-to-cart-btn]").click()
    expect(page.locator("[data-cy=toast][data-kind=success]")).to_contain_text("2 item(s)")
    shot(page, "02_shop_added_to_cart")

    # Cart: 2 x 4,500 = 9,000. C002 lives in Phuentsholing (border zone).
    page.goto(BASE + "/cart.html")
    page.click("[data-cy=cart-refresh-btn]")
    expect(page.locator("[data-cy=cart-subtotal]")).to_contain_text("Nu. 9000.00")
    expect(page.locator("[data-cy=cart-shipping]")).to_contain_text(re.compile("free", re.I))
    page.fill("[data-cy=coupon-code]", "WELCOME10")
    shot(page, "03_cart_with_coupon")

    # Checkout: 9,000 - 5% quantity discount = 8,550; WELCOME10 -10% = 7,695;
    # shipping is free because 7,695 >= Nu. 5,000.
    page.click("[data-cy=cart-checkout-btn]")
    toast = page.locator("[data-cy=toast][data-kind=success]").last
    expect(toast).to_contain_text("Checkout complete: 1 order(s)", timeout=15000)
    expect(page.locator("[data-cy=checkout-total]")).to_contain_text("Nu. 7695.00")
    page.wait_for_timeout(700)  # let the toast finish fading in for the screenshot
    shot(page, "04_checkout_complete")

    # Order history and tracking.
    page.goto(BASE + "/orders.html")
    page.click("[data-cy=orders-load-btn]")
    body = page.locator("[data-cy=orders-body]")
    expect(body).to_contain_text("Mechanical Keyboard")
    tracking = re.search(r"TRK-\d{5}", body.inner_text()).group(0)
    page.fill("[data-cy=track-input]", tracking)
    page.click("[data-cy=track-btn]")
    expect(page.locator("[data-cy=track-result]")).to_contain_text("PROCESSING")
    shot(page, "05_orders_and_tracking")


def test_TC22_exploratory_unknown_customer_and_empty_checkout(page):
    """TC22 (exploratory/negative): an unknown customer gets a friendly
    message, and checking out an empty cart is refused."""
    page.goto(BASE + "/cart.html")
    page.fill("[data-cy=customer-id]", "C999")
    page.click("[data-cy=cart-refresh-btn]")
    expect(page.locator("[data-cy=toast]").last).to_contain_text("does not exist")
    shot(page, "06_unknown_customer")

    page.fill("[data-cy=customer-id]", "C003")
    page.click("[data-cy=cart-refresh-btn]")
    expect(page.locator("[data-cy=cart-empty]")).to_be_visible()

    status = urllib.request.Request(BASE + "/api/cart/checkout", method="POST",
                                    data=b'{"customerId":"C003"}',
                                    headers={"Content-Type": "application/json"})
    with pytest.raises(urllib.error.HTTPError) as err:
        urllib.request.urlopen(status, timeout=10)
    assert err.value.code == 409
