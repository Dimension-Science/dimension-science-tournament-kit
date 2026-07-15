(function () {
  "use strict";
  const list = document.querySelector("[data-discord-applications-list]");
  const message = document.querySelector("[data-discord-applications-message]");
  if (!list || !message) return;

  function text(tag, value, className) {
    const node = document.createElement(tag);
    if (className) node.className = className;
    node.textContent = value;
    return node;
  }

  async function request(url, options) {
    const response = await fetch(url, Object.assign({ credentials: "same-origin", headers: { "Content-Type": "application/json" } }, options || {}));
    const payload = await response.json().catch(function () { return {}; });
    if (!response.ok) throw new Error(payload.error || "Ошибка запроса");
    return payload;
  }

  function statusLabel(status) {
    return { pending: "На рассмотрении", approved: "Одобрена", rejected: "Отклонена", needs_info: "Нужно уточнение" }[status] || status;
  }

  function render(applications) {
    list.replaceChildren();
    const pending = applications.filter(function (app) { return app.status === "pending" || app.status === "needs_info"; }).length;
    message.textContent = pending ? "Ожидают решения: " + pending : "Новых Discord-заявок нет.";
    applications.forEach(function (app) {
      const card = document.createElement("article");
      card.className = "admin-application-row is-" + app.status;
      const header = document.createElement("div");
      header.className = "admin-application-header";
      header.append(text("strong", "DSC-" + String(app.applicationNumber).padStart(5, "0") + " · " + app.gameNick), text("span", statusLabel(app.status), "admin-application-status"));
      const meta = document.createElement("div");
      meta.className = "admin-application-meta";
      meta.append(text("span", "Discord: " + app.discordUsername), text("span", "TZ: " + app.timezone), text("span", "ID: " + app.discordUserId));
      card.append(header, meta, text("p", "Опыт: " + app.experience), text("p", "Мотивация: " + app.motivation));
      if (app.links) card.append(text("p", "Ссылки: " + app.links));
      if (app.reviewReason) card.append(text("p", "Причина: " + app.reviewReason));
      if (app.status === "pending" || app.status === "needs_info") {
        const actions = document.createElement("div");
        actions.className = "admin-application-actions";
        const approve = text("button", "Одобрить", "button-secondary");
        approve.type = "button";
        approve.onclick = function () { review(app.id, "approve"); };
        const reject = text("button", "Отклонить", "button-danger");
        reject.type = "button";
        reject.onclick = function () {
          const reason = window.prompt("Причина отклонения:");
          if (reason && reason.trim()) review(app.id, "reject", reason.trim());
        };
        actions.append(approve, reject);
        card.append(actions);
      }
      list.append(card);
    });
  }

  async function load() {
    try { render(await request("/api/admin/discord-applications")); }
    catch (error) { message.textContent = error.message; }
  }

  async function review(id, action, reason) {
    message.textContent = action === "approve" ? "Выдаю роль и одобряю заявку…" : "Отклоняю заявку…";
    try {
      await request("/api/admin/discord-applications/" + encodeURIComponent(id) + "/" + action, { method: "POST", body: JSON.stringify({ reason: reason || "" }) });
      await load();
    } catch (error) { message.textContent = error.message; }
  }

  load();
}());
