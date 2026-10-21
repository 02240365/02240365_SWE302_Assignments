// reviews.js - read and write product reviews (reviews.html)
// ---- Reviews ----
const reviewProduct = document.getElementById("review-product");
const reviewList = document.getElementById("review-list");
const reviewSummary = document.getElementById("review-summary");
const reviewMessage = document.getElementById("review-message");

function fillReviewProducts(products) {
  reviewProduct.innerHTML = products.map((p) => `<option value="${escapeHtml(p.productId)}">${escapeHtml(p.name)} (${escapeHtml(p.productId)})</option>`).join("");
  loadReviews();
}

function stars(n) {
  return `${n}/5`;
}

async function loadReviews() {
  const id = reviewProduct.value;
  if (!id) return;
  try {
    const { ok, data } = await api("GET", `/api/reviews/${encodeURIComponent(id)}`);
    if (!ok) {
      reviewSummary.textContent = data.message;
      reviewList.innerHTML = "";
      return;
    }
    reviewSummary.innerHTML = data.reviewCount === 0
      ? `<span data-cy="review-average">No reviews yet</span>`
      : `<span data-cy="review-average">Average rating: ${data.averageRating.toFixed(1)} / 5</span> <span class="muted" data-cy="review-count">(${data.reviewCount} review${data.reviewCount === 1 ? "" : "s"})</span>`;
    reviewList.innerHTML = data.reviews.map((r) => `
      <li data-cy="review-item">
        <div><span data-cy="review-stars">${stars(r.rating)}</span> <span class="review-meta">${escapeHtml(r.customerName)}</span></div>
        <div data-cy="review-text">${escapeHtml(r.comment)}</div>
      </li>`).join("");
  } catch (err) {
    reviewSummary.textContent = "Could not reach the Gateway.";
  }
}

reviewProduct.addEventListener("change", loadReviews);
document.getElementById("review-load-btn").addEventListener("click", loadReviews);

document.getElementById("review-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const customerId = customerIdInput.value.trim();
  const btn = document.getElementById("review-submit");
  btn.disabled = true;
  try {
    const { ok, data } = await api("POST", "/api/reviews", {
      productId: reviewProduct.value,
      customerId,
      rating: parseInt(document.getElementById("review-rating").value, 10),
      comment: document.getElementById("review-comment").value,
    });
    if (ok) {
      reviewMessage.className = "result state-success";
      reviewMessage.innerHTML = `${icons.success} <span data-cy="review-ok">Thank you! Your review was added.</span>`;
      document.getElementById("review-comment").value = "";
      loadReviews();
    } else {
      reviewMessage.className = "result state-warning";
      reviewMessage.innerHTML = `${icons.warning} <span data-cy="review-error">${escapeHtml(data.message)}</span>`;
    }
  } catch (err) {
    reviewMessage.className = "result state-unavailable";
    reviewMessage.innerHTML = `${icons.offline} <span>Could not reach the Gateway.</span>`;
  } finally {
    btn.disabled = false;
  }
});


async function loadReviewProducts() {
  try {
    const { data } = await api("GET", "/api/products");
    const products = data.products || [];
    fillReviewProducts(products);
    const wanted = new URLSearchParams(location.search).get("id");
    if (wanted && products.some((p) => p.productId === wanted)) {
      reviewProduct.value = wanted;
      loadReviews();
    }
  } catch (err) {
    gatewayDown();
  }
}
loadReviewProducts();
