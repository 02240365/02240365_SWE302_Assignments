// home.js - landing page: featured products and hero search (index.html)
const featured = document.getElementById("featured-grid");

async function loadFeatured() {
  try {
    const { data } = await api("GET", "/api/products");
    const products = data.products || [];
    if (products.length === 0) {
      featured.innerHTML = `<p class="empty-state">${icons.warning} No products could be loaded right now.</p>`;
      return;
    }
    featured.innerHTML = products.map((p, i) => `
      <a class="product-card featured-card" href="/shop.html?id=${encodeURIComponent(p.productId)}" data-cy="featured-${escapeHtml(p.productId)}">
        <div class="thumb ${tileColors[i % tileColors.length]}">${productIcons[idToIconKey[p.productId] || "box"]}</div>
        <div class="card-body">
          <div class="p-name">${escapeHtml(p.name)}</div>
          <div class="p-price">Nu. ${p.price.toFixed(2)}</div>
        </div>
      </a>`).join("");
  } catch (err) {
    featured.innerHTML = `<p class="empty-state">${icons.offline} Could not reach the Gateway. Is ui-service running?</p>`;
  }
}

document.getElementById("hero-search").addEventListener("submit", (event) => {
  event.preventDefault();
  const id = document.getElementById("hero-product-id").value.trim();
  location.href = id ? `/shop.html?id=${encodeURIComponent(id)}` : "/shop.html";
});

loadFeatured();
