// status.js - the ten microservices (status.html)
async function loadStatus() {
  const list = document.getElementById("status-list");
  try {
    const { data } = await api("GET", "/api/services");
    list.innerHTML = (data.services || []).map((s) => `
      <li data-cy="svc-${escapeHtml(s.name)}" data-up="${s.up}">
        <span>${escapeHtml(s.name)} <span class="muted">:${s.port}</span></span>
        <span class="${s.up ? "status-up" : "status-down"}">${s.up ? "Up" : "Down"}</span>
      </li>`).join("");
    document.getElementById("discount-mode").textContent = `Discounts: ${data.discount}`;
  } catch (err) {
    list.innerHTML = `<li>Could not reach the Gateway.</li>`;
  }
}
document.getElementById("status-refresh-btn").addEventListener("click", loadStatus);

loadStatus();
