// cart.js - shopping cart and checkout (cart.html)
// ---- Cart panel ----
const cartBody = document.getElementById("cart-body");

function renderCart(cart) {
  if (!cart.lines || cart.lines.length === 0) {
    cartBody.innerHTML = `<p class="muted" data-cy="cart-empty">Your cart is empty.</p>`;
    return;
  }
  const rows = cart.lines.map((l) => `
    <tr data-cy="cart-line-${escapeHtml(l.productId)}">
      <td>${escapeHtml(l.productName)}</td>
      <td data-cy="cart-qty">${l.quantity}</td>
      <td>${money(l.unitPrice)}</td>
      <td data-cy="cart-line-total">${money(l.lineTotal)}</td>
      <td><button type="button" class="small-btn cart-remove-btn" data-id="${escapeHtml(l.productId)}" data-cy="cart-remove-btn">Remove</button></td>
    </tr>`).join("");
  cartBody.innerHTML = `
    <table class="cart-table" data-cy="cart-table">
      <thead><tr><th>Product</th><th>Qty</th><th>Price</th><th>Total</th><th></th></tr></thead>
      <tbody>${rows}</tbody>
    </table>
    <div class="cart-actions">
      <span class="cart-total" data-cy="cart-subtotal">Subtotal: ${money(cart.subtotal)}</span>
      <span class="muted" data-cy="cart-count">${cart.itemCount} item(s)</span>
      <span class="muted" id="cart-shipping" data-cy="cart-shipping"></span>
      <button type="button" class="small-btn" id="cart-clear-btn" data-cy="cart-clear-btn">Empty cart</button>
      <button type="button" class="primary-btn" id="cart-checkout-btn" data-cy="cart-checkout-btn">Check out</button>
    </div>`;
  cartBody.querySelectorAll(".cart-remove-btn").forEach((b) => b.addEventListener("click", onCartRemove));
  document.getElementById("cart-clear-btn").addEventListener("click", onCartClear);
  document.getElementById("cart-checkout-btn").addEventListener("click", onCheckout);
  showShippingEstimate(cart);
}

async function showShippingEstimate(cart) {
  const customerId = customerIdInput.value.trim();
  const el = document.getElementById("cart-shipping");
  if (!customerId || !el) return;
  try {
    const { ok, data } = await api("GET", `/api/shipping/quote?customerId=${encodeURIComponent(customerId)}&subtotal=${cart.subtotal}&itemCount=${cart.itemCount}`);
    if (ok && el) el.textContent = `Shipping to ${data.address}: ${data.freeShipping ? "Free" : money(data.fee)} (${data.etaDays} day(s))`;
  } catch (e) { /* the estimate is optional */ }
}

async function onCartRemove(event) {
  const customerId = needCustomer();
  if (!customerId) return;
  const { ok, data } = await api("DELETE", `/api/cart/${encodeURIComponent(event.currentTarget.dataset.id)}?customerId=${encodeURIComponent(customerId)}`);
  if (!ok) return reportError(data);
  renderCart(data);
}

async function onCartClear() {
  const customerId = needCustomer();
  if (!customerId) return;
  const { ok, data } = await api("DELETE", `/api/cart?customerId=${encodeURIComponent(customerId)}`);
  if (!ok) return reportError(data);
  renderCart(data);
}

async function onCheckout(event) {
  const customerId = needCustomer();
  if (!customerId) return;
  const btn = event.currentTarget;
  btn.disabled = true;
  btn.textContent = "Placing orders...";
  try {
    const { ok, data } = await api("POST", "/api/cart/checkout", { customerId, couponCode: couponInput.value.trim() });
    if (ok && data.success) {
      const items = data.orders.map((o) => `<div class="order-detail">${escapeHtml(o.orderId)} &middot; ${escapeHtml(o.productName)} x ${o.quantity} &middot; ${money(o.finalTotal)}</div>`).join("");
      showToast("success", `<div><div class="order-summary-title" data-cy="checkout-title">Checkout complete: ${data.orders.length} order(s)</div>${items}<div class="order-detail" data-cy="checkout-total">Total charged: ${money(data.total)}</div></div>`, true);
    } else {
      reportError(data);
    }
    const cart = await api("GET", `/api/cart?customerId=${encodeURIComponent(customerId)}`);
    if (cart.ok) renderCart(cart.data);
  } catch (err) {
    gatewayDown();
  }
}


async function loadCart(quiet) {
  const customerId = quiet ? customerIdInput.value.trim() : needCustomer();
  if (!customerId) return;
  try {
    const { ok, data } = await api("GET", `/api/cart?customerId=${encodeURIComponent(customerId)}`);
    if (!ok) return reportError(data);
    renderCart(data);
  } catch (err) {
    gatewayDown();
  }
}
document.getElementById("cart-refresh-btn").addEventListener("click", () => loadCart(false));
customerIdInput.addEventListener("change", () => loadCart(true));
loadCart(true);
