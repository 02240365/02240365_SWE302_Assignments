// common.js - shared by every page of the storefront.
// Talks only to the gateway's REST endpoints (/api/...), never to a gRPC
// service directly. Provides: icons, small helpers, toasts, the header
// (customer ID and coupon boxes that are remembered between pages) and the
// highlighted navigation link.

// ---- Shared status icons ----
const icons = {
  loading: `<svg class="icon spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M21 12a9 9 0 1 1-9-9"/></svg>`,
  success: `<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>`,
  warning: `<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>`,
  offline: `<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="1" y1="1" x2="23" y2="23"/><path d="M16.72 11.06A10.94 10.94 0 0 1 19 12.55"/><path d="M5 12.55a10.94 10.94 0 0 1 5.17-2.39"/><path d="M10.71 5.05A16 16 0 0 1 22.58 9"/><path d="M1.42 9a15.91 15.91 0 0 1 4.7-2.88"/><path d="M8.53 16.11a6 6 0 0 1 6.95 0"/><line x1="12" y1="20" x2="12.01" y2="20"/></svg>`,
};

// ---- Product-category icons (simple, non-emoji, flat stroke style) ----
const productIcons = {
  laptop: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="4" width="16" height="10" rx="1"></rect><path d="M2 18h20l-1.5 2.5a1 1 0 0 1-.86.5H4.36a1 1 0 0 1-.86-.5L2 18Z"></path></svg>`,
  keyboard: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="5" width="20" height="14" rx="2"></rect><path d="M6 9h.01M10 9h.01M14 9h.01M18 9h.01M6 13h.01M18 13h.01M9 13h6"></path></svg>`,
  mouse: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round"><rect x="6" y="2" width="12" height="20" rx="6"></rect><line x1="12" y1="2" x2="12" y2="10"></line></svg>`,
  monitor: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="20" height="14" rx="2"></rect><line x1="8" y1="21" x2="16" y2="21"></line><line x1="12" y1="17" x2="12" y2="21"></line></svg>`,
  box: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round"><path d="m21 8-9-5-9 5 9 5 9-5Z"></path><path d="M3 8v8l9 5 9-5V8"></path><path d="M12 13v8"></path></svg>`,
};

// Known catalogue IDs -> icon key. Falls back to a generic box icon for
// any product ID not listed here, so the grid never breaks if the
// catalogue changes.
const idToIconKey = { P001: "laptop", P002: "keyboard", P003: "mouse", P004: "monitor" };
const tileColors = ["tile-blue", "tile-teal", "tile-amber", "tile-violet"];

function escapeHtml(text) {
  const div = document.createElement("div");
  div.textContent = text == null ? "" : String(text);
  return div.innerHTML;
}

function money(n) {
  return "Nu. " + Number(n || 0).toFixed(2);
}

// api: call the gateway and return {ok, status, data}. An HTTP error (400,
// 404, 409 ...) is returned, not thrown; only a network failure throws.
async function api(method, url, body) {
  const res = await fetch(url, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  let data = {};
  try {
    data = await res.json();
  } catch (e) {
    data = { message: "The server sent an unreadable answer." };
  }
  return { ok: res.ok, status: res.status, data };
}

// ---- Toasts: small auto-dismissing panels in the bottom-right corner ----
const toastContainer = document.getElementById("toast-container");

function showToast(kind, htmlOrText, isHTML = false) {
  const toast = document.createElement("div");
  toast.className = `toast toast-${kind}`;
  toast.setAttribute("data-cy", "toast");
  toast.setAttribute("data-kind", kind);
  const icon = kind === "success" ? icons.success : kind === "unavailable" ? icons.offline : icons.warning;
  toast.innerHTML = `${icon} ${isHTML ? htmlOrText : `<span>${escapeHtml(htmlOrText)}</span>`}`;
  toastContainer.appendChild(toast);
  requestAnimationFrame(() => toast.classList.add("show"));
  const lifetime = isHTML ? 9000 : 6000;
  setTimeout(() => {
    toast.classList.remove("show");
    setTimeout(() => toast.remove(), 250);
  }, lifetime);
}

function reportError(data) {
  showToast(data.errorCode === "unavailable" ? "unavailable" : "warning", `${data.step ? `[${data.step}] ` : ""}${data.message || "Something went wrong."}`);
}

function gatewayDown() {
  showToast("unavailable", "Could not reach the Gateway. Is ui-service running?");
}

// ---- Header: customer ID and coupon, remembered while moving between pages ----
const customerIdInput = document.getElementById("customer-id");
const couponInput = document.getElementById("coupon-code");

function safeGet(key) {
  try { return localStorage.getItem(key) || ""; } catch (e) { return ""; }
}
function safeSet(key, value) {
  try { localStorage.setItem(key, value); } catch (e) { /* remembering is optional */ }
}

customerIdInput.value = safeGet("ecom.customer");
couponInput.value = safeGet("ecom.coupon");
customerIdInput.addEventListener("input", () => safeSet("ecom.customer", customerIdInput.value));
couponInput.addEventListener("input", () => safeSet("ecom.coupon", couponInput.value));

function needCustomer() {
  const id = customerIdInput.value.trim();
  if (!id) {
    showToast("warning", "Please enter your Customer ID at the top first.");
    customerIdInput.focus();
    return null;
  }
  return id;
}

// Coupon check: asks the Coupon Service what the code would save on a
// sample order of Nu. 10,000 and shows the answer in a toast.
document.getElementById("coupon-check-btn").addEventListener("click", async () => {
  const customerId = needCustomer();
  if (!customerId) return;
  try {
    const { ok, data } = await api("POST", "/api/coupons/validate", { customerId, code: couponInput.value.trim(), subtotal: 10000 });
    if (ok) {
      showToast("success", `<span data-cy="coupon-ok">Coupon ${escapeHtml(data.code)} is valid: ${escapeHtml(data.description)}. On Nu. 10,000 you would save ${money(data.discountAmount)}.</span>`, true);
    } else {
      showToast("warning", `<span data-cy="coupon-error">${escapeHtml(data.message)}</span>`, true);
    }
  } catch (err) {
    gatewayDown();
  }
});

// Highlight the link of the page we are on (body has data-page="shop" etc.).
const here = document.body.getAttribute("data-page");
document.querySelectorAll(".nav a").forEach((a) => {
  if (a.getAttribute("data-page") === here) {
    a.classList.add("active");
    a.setAttribute("aria-current", "page");
  }
});
