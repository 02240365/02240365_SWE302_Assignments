// orders.js - order history and parcel tracking (orders.html)
// ---- Order history and tracking ----
const ordersBody = document.getElementById("orders-body");

async function loadOrders(quiet) {
  const customerId = quiet ? customerIdInput.value.trim() : needCustomer();
  if (!customerId) return;
  try {
    const { ok, data } = await api("GET", `/api/orders?customerId=${encodeURIComponent(customerId)}`);
    if (!ok) return reportError(data);
    if (data.orders.length === 0) {
      ordersBody.innerHTML = `<p class="muted" data-cy="orders-empty">You have not placed any orders yet.</p>`;
      return;
    }
    ordersBody.innerHTML = `
      <table class="orders-table" data-cy="orders-table">
        <thead><tr><th>Order</th><th>Product</th><th>Qty</th><th>Total</th><th>Tracking</th></tr></thead>
        <tbody>${data.orders.map((o) => `
          <tr data-cy="order-row">
            <td data-cy="order-row-id">${escapeHtml(o.orderId)}</td>
            <td>${escapeHtml(o.productName)}</td>
            <td>${o.quantity}</td>
            <td data-cy="order-row-total">${money(o.total)}</td>
            <td data-cy="order-row-tracking">${escapeHtml(o.trackingId)}</td>
          </tr>`).join("")}</tbody>
      </table>`;
  } catch (err) {
    gatewayDown();
  }
}
document.getElementById("orders-load-btn").addEventListener("click", () => loadOrders(false));
customerIdInput.addEventListener("change", () => loadOrders(true));

document.getElementById("track-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const out = document.getElementById("track-result");
  const id = document.getElementById("track-input").value.trim();
  try {
    const { ok, data } = await api("GET", `/api/shipping/track/${encodeURIComponent(id)}`);
    if (ok) {
      out.className = "result state-success";
      out.innerHTML = `${icons.success} <div><div class="product-name" data-cy="track-status">${escapeHtml(data.trackingId)}: ${escapeHtml(data.status)}</div>
        <div class="product-meta">Order ${escapeHtml(data.orderId)} &middot; to ${escapeHtml(data.address)} &middot; about ${data.etaDays} day(s)</div>
        ${data.events.map((e) => `<div class="product-meta" data-cy="track-event">${escapeHtml(e.time)} - ${escapeHtml(e.message)}</div>`).join("")}</div>`;
    } else {
      out.className = "result state-warning";
      out.innerHTML = `${icons.warning} <span data-cy="track-error">${escapeHtml(data.message)}</span>`;
    }
  } catch (err) {
    out.className = "result state-unavailable";
    out.innerHTML = `${icons.offline} <span>Could not reach the Gateway.</span>`;
  }
});


// A tracking ID in the address (orders.html?track=TRK-00001) is looked up straight away.
(function () {
  const t = new URLSearchParams(location.search).get("track");
  if (t) {
    document.getElementById("track-input").value = t;
    document.getElementById("track-form").requestSubmit();
  }
})();
loadOrders(true);
