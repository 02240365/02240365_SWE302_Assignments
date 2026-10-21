// shop.js - catalogue, product lookup, ordering and add-to-cart (shop.html)

const lookupForm = document.getElementById("lookup-form");
const productIdInput = document.getElementById("product-id");
const lookupResult = document.getElementById("lookup-result");
const lookupBtn = document.getElementById("lookup-btn");

lookupForm.addEventListener("submit", async (event) => {
  event.preventDefault();

  const id = productIdInput.value.trim();
  lookupBtn.disabled = true;
  lookupResult.className = "result state-loading";
  lookupResult.innerHTML = `${icons.loading} <span>Looking up "${escapeHtml(id) || "..."}"...</span>`;

  try {
    const res = await fetch(`/api/products/${encodeURIComponent(id)}`);
    const data = await res.json();

    if (data.found) {
      lookupResult.className = "result state-success";
      lookupResult.innerHTML = `
        ${icons.success}
        <div>
          <div class="product-name">${escapeHtml(data.name)}</div>
          <div class="product-meta">ID: ${escapeHtml(data.productId)} &nbsp;&middot;&nbsp; Nu. ${data.price.toFixed(2)}</div>
        </div>`;
    } else if (data.errorCode === "unavailable") {
      lookupResult.className = "result state-unavailable";
      lookupResult.innerHTML = `${icons.offline} <span>${escapeHtml(data.message)}</span>`;
    } else {
      lookupResult.className = "result state-warning";
      lookupResult.innerHTML = `${icons.warning} <span>${escapeHtml(data.message)}</span>`;
    }
  } catch (err) {
    lookupResult.className = "result state-unavailable";
    lookupResult.innerHTML = `${icons.offline} <span>Could not reach the Gateway. Is ui-service running?</span>`;
  } finally {
    lookupBtn.disabled = false;
  }
});

// =======================================================================
// Section 2: Shop - browsable product grid with inline ordering

const grid = document.getElementById("product-grid");

function renderSkeletons(n) {
  return Array.from({ length: n })
    .map(
      () => `
      <div class="product-card skeleton">
        <div class="thumb"></div>
        <div class="card-body">
          <div class="line"></div>
          <div class="line short"></div>
        </div>
      </div>`
    )
    .join("");
}

function renderCard(product, index) {
  const iconKey = idToIconKey[product.productId] || "box";
  const tileClass = tileColors[index % tileColors.length];
  return `
    <div class="product-card" data-id="${escapeHtml(product.productId)}" data-cy="product-card-${escapeHtml(product.productId)}">
      <div class="thumb ${tileClass}">${productIcons[iconKey]}</div>
      <div class="card-body">
        <div class="p-name" data-cy="product-name">${escapeHtml(product.name)}</div>
        <div class="p-price" data-cy="product-price">Nu. ${product.price.toFixed(2)}</div>
        <div class="order-row">
          <input type="number" min="1" value="1" class="qty-input" data-cy="qty-input" aria-label="Quantity for ${escapeHtml(product.name)}" />
          <button type="button" class="small-btn add-to-cart-btn" data-cy="add-to-cart-btn">Add to cart</button>
          <button type="button" class="place-order-btn" data-cy="place-order-btn">
            <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M6 2 3 6v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V6l-3-4Z"></path>
              <path d="M3 6h18"></path>
              <path d="M16 10a4 4 0 0 1-8 0"></path>
            </svg>
            <span>Place Order</span>
          </button>
        </div>
      </div>
    </div>`;
}

async function loadCatalog() {
  grid.innerHTML = renderSkeletons(4);

  try {
    const res = await fetch("/api/products");
    const data = await res.json();
    const products = data.products || [];

    if (products.length === 0) {
      grid.innerHTML = `<p class="empty-state">${icons.warning} No products could be loaded right now.</p>`;
      return;
    }

    grid.innerHTML = products.map(renderCard).join("");
    grid.querySelectorAll(".place-order-btn").forEach((btn) => btn.addEventListener("click", onPlaceOrderClick));
    grid.querySelectorAll(".add-to-cart-btn").forEach((btn) => btn.addEventListener("click", onAddToCartClick));
  } catch (err) {
    grid.innerHTML = `<p class="empty-state">${icons.offline} Could not reach the Gateway. Is ui-service running?</p>`;
  }
}

async function onPlaceOrderClick(event) {
  const btn = event.currentTarget;
  const card = btn.closest(".product-card");
  const productId = card.dataset.id;
  const quantity = parseInt(card.querySelector(".qty-input").value, 10) || 0;
  const customerId = customerIdInput.value.trim();

  if (!customerId) {
    showToast("warning", "Please enter your Customer ID at the top before placing an order.");
    customerIdInput.focus();
    return;
  }
  if (quantity <= 0) {
    showToast("warning", "Quantity must be at least 1.");
    return;
  }

  const originalHTML = btn.innerHTML;
  btn.disabled = true;
  btn.innerHTML = `${icons.loading} <span>Placing...</span>`;

  try {
    const res = await fetch("/api/orders", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ customerId, productId, quantity, couponCode: couponInput.value.trim() }),
    });
    const data = await res.json();

    if (data.success) {
      showToast("success", buildOrderSummaryHTML(data), true);
    } else if (data.errorCode === "unavailable") {
      showToast("unavailable", `${data.step ? `[${data.step}] ` : ""}${data.message}`);
    } else {
      showToast("warning", `${data.step ? `[${data.step}] ` : ""}${data.message}`);
    }
  } catch (err) {
    showToast("unavailable", "Could not reach the Gateway. Is ui-service running?");
  } finally {
    btn.disabled = false;
    btn.innerHTML = originalHTML;
  }
}

function buildOrderSummaryHTML(data) {
  const notifItems = (data.notifications || []).map((n) => `<li>${escapeHtml(n)}</li>`).join("");
  const warnings = (data.warnings || []).map((w) => `<div class="order-detail">Note: ${escapeHtml(w)}</div>`).join("");
  const lines = [];
  lines.push(`<div class="order-detail" data-cy="order-subtotal">Subtotal: ${money(data.totalAmount)}</div>`);
  if (data.discountPercent > 0) {
    lines.push(`<div class="order-detail" data-cy="order-discount">Quantity discount: ${data.discountPercent}% (-${money(data.discountAmount)})</div>`);
  }
  if (data.couponCode) {
    lines.push(`<div class="order-detail" data-cy="order-coupon">Coupon ${escapeHtml(data.couponCode)}: -${money(data.couponDiscount)}</div>`);
  }
  lines.push(`<div class="order-detail" data-cy="order-shipping">Shipping (${escapeHtml(data.shippingZone || "")}): ${data.shippingFee > 0 ? money(data.shippingFee) : "Free"} &middot; arrives in ${data.etaDays || "?"} day(s)</div>`);
  lines.push(`<div class="order-detail" data-cy="order-total">Total charged: ${money(data.finalTotal)} &middot; Payment ID: ${escapeHtml(data.paymentId)}</div>`);
  if (data.trackingId) {
    lines.push(`<div class="order-detail" data-cy="order-tracking">Tracking ID: ${escapeHtml(data.trackingId)}</div>`);
  }
  return `
    <div>
      <div class="order-summary-title" data-cy="order-id">Order ${escapeHtml(data.orderId)} placed</div>
      <div class="order-detail">Customer: ${escapeHtml(data.customerName)}</div>
      <div class="order-detail">Product: ${escapeHtml(data.productName)} &middot; ${money(data.unitPrice)} each &middot; Quantity: ${data.quantity}</div>
      ${lines.join("")}
      <div class="order-detail" data-cy="order-remaining">Remaining stock: ${data.remainingStock}</div>
      ${warnings}
      ${notifItems ? `<ul class="notification-list">${notifItems}</ul>` : ""}
    </div>`;
}

// showToast renders a small, auto-dismissing panel in the bottom-right
// corner - success toasts stay a little longer since they hold more

// ---- Add to cart ----
async function onAddToCartClick(event) {
  const card = event.currentTarget.closest(".product-card");
  const customerId = needCustomer();
  if (!customerId) return;
  const quantity = parseInt(card.querySelector(".qty-input").value, 10) || 0;
  if (quantity <= 0) {
    showToast("warning", "Quantity must be at least 1.");
    return;
  }
  try {
    const { ok, data } = await api("POST", "/api/cart", { customerId, productId: card.dataset.id, quantity });
    if (!ok) return reportError(data);
    showToast("success", `<span>Added to cart. Your cart now has ${data.itemCount} item(s). <a href="/cart.html" data-cy="toast-view-cart">View cart</a></span>`, true);
  } catch (err) {
    gatewayDown();
  }
}

// Arriving from the home page search (shop.html?id=P001) runs the lookup.
(function () {
  const wanted = new URLSearchParams(location.search).get("id");
  if (wanted) {
    productIdInput.value = wanted;
    lookupForm.requestSubmit();
  }
})();

loadCatalog();
