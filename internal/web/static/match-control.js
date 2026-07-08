document.addEventListener("DOMContentLoaded", function () {
  const title = document.querySelector("[data-control-title]");
  const message = document.querySelector("[data-control-message]");
  const daysNode = document.querySelector("[data-match-days]");
  const listNode = document.querySelector("[data-match-list]");
  let tournament = null;
  let matches = [];
  let selectedDay = "";

  function setText(node, value) {
    if (node) {
      node.textContent = value;
    }
  }

  async function requestJSON(url, options) {
    const response = await fetch(url, Object.assign({
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
    }, options || {}));

    if (!response.ok) {
      let error = "Ошибка запроса";
      try {
        const payload = await response.json();
        error = payload.error || payload.message || error;
      } catch (_) {}
      throw new Error(error);
    }

    if (response.status === 204) {
      return null;
    }
    return response.json();
  }

  function participantName(participant) {
    if (!participant) {
      return "ожидает участника";
    }
    return participant.twitchDisplayName || participant.twitchLogin || participant.minecraftNick || "участник";
  }

  function roundLabel(round) {
    switch (round) {
      case "quarterfinal": return "1/4";
      case "semifinal": return "1/2";
      case "final": return "Финал";
      case "third_place": return "За 3 место";
      default: return round || "Матч";
    }
  }

  function dateKey(value) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
      return "";
    }
    return date.toISOString().slice(0, 10);
  }

  function dateLabel(value) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
      return "дата не назначена";
    }
    return date.toLocaleDateString("ru-RU", { day: "2-digit", month: "long", timeZone: "Europe/Moscow" });
  }

  function timeLabel(value) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
      return "-";
    }
    return date.toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit", timeZone: "Europe/Moscow" });
  }

  function uniqueDays() {
    const days = [];
    const seen = new Set();
    matches.forEach(function (match) {
      const key = dateKey(match.startsAt);
      if (key && !seen.has(key)) {
        seen.add(key);
        days.push({ key: key, label: dateLabel(match.startsAt) });
      }
    });
    return days;
  }

  function renderDays() {
    daysNode.innerHTML = "";
    const days = uniqueDays();
    if (!days.length) {
      const empty = document.createElement("div");
      empty.className = "empty-state empty-state-inline";
      empty.innerHTML = "<strong>Матчей пока нет</strong><span>Сетка появится после квалификации.</span>";
      daysNode.appendChild(empty);
      return;
    }
    if (!selectedDay || !days.some(function (day) { return day.key === selectedDay; })) {
      selectedDay = days[0].key;
    }
    days.forEach(function (day) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "match-day-button";
      button.classList.toggle("is-active", day.key === selectedDay);
      const count = matches.filter(function (match) { return dateKey(match.startsAt) === day.key; }).length;
      button.innerHTML = "<strong></strong><span></span>";
      button.querySelector("strong").textContent = day.label;
      button.querySelector("span").textContent = count + " матч" + (count === 1 ? "" : "а");
      button.addEventListener("click", function () {
        selectedDay = day.key;
        render();
      });
      daysNode.appendChild(button);
    });
  }

  function statusBadge(connected) {
    return connected ? "<span class=\"match-ready-badge is-ready\">подключен</span>" : "<span class=\"match-ready-badge\">ожидает</span>";
  }

  function renderMatch(match) {
    const row = document.createElement("article");
    row.className = "match-control-card";
    const player1Connected = Boolean(match.player1ConnectedAt);
    const player2Connected = Boolean(match.player2ConnectedAt);
    const started = Boolean(match.officialStartedAt);
    const canStart = player1Connected && player2Connected && !started;
    row.innerHTML = [
      "<div class=\"match-control-card-head\">",
      "  <div><p class=\"eyebrow\"></p><h2></h2><small></small></div>",
      "  <strong class=\"match-seed\"></strong>",
      "</div>",
      "<div class=\"match-ready-grid\">",
      "  <div class=\"match-ready-player\"><strong></strong><span></span></div>",
      "  <div class=\"match-ready-player\"><strong></strong><span></span></div>",
      "</div>",
      "<div class=\"match-control-actions\" style=\"display: flex; gap: 8px; align-items: center;\">",
      "  <span class=\"match-start-state\" style=\"margin-right: auto;\"></span>",
      "  <button class=\"button\" type=\"button\" data-action=\"start\"></button>",
      "  <button class=\"button button-danger\" type=\"button\" data-action=\"reset\" style=\"background: #e03131; border-color: #c92a2a; color: #fff;\">Сбросить</button>",
      "</div>",
    ].join("");
    row.querySelector(".eyebrow").textContent = roundLabel(match.round) + " #" + match.position;
    row.querySelector("h2").textContent = participantName(match.player1) + " vs " + participantName(match.player2);
    row.querySelector("small").textContent = timeLabel(match.startsAt) + " - " + timeLabel(match.endsAt);
    row.querySelector(".match-seed").textContent = match.worldSeed ? "Seed: " + match.worldSeed : "Seed не назначен";
    const players = row.querySelectorAll(".match-ready-player");
    players[0].querySelector("strong").textContent = participantName(match.player1);
    players[0].querySelector("span").outerHTML = statusBadge(player1Connected);
    players[1].querySelector("strong").textContent = participantName(match.player2);
    players[1].querySelector("span").outerHTML = statusBadge(player2Connected);
    const state = row.querySelector(".match-start-state");
    state.textContent = started ? "Матч запущен" : (canStart ? "Оба подключены" : "Ждем игроков в мирах");
    const startButton = row.querySelector('button[data-action="start"]');
    const resetButton = row.querySelector('button[data-action="reset"]');
    startButton.disabled = !canStart;
    startButton.textContent = started ? "Запущено" : "Старт";
    startButton.addEventListener("click", function () {
      startMatch(match.id);
    });
    resetButton.addEventListener("click", function () {
      if (confirm("Вы уверены, что хотите сбросить этот матч? Это вернет его в режим ожидания, удалит результаты и текущие сессии игроков, но сохранит сид.")) {
        resetMatch(match.id);
      }
    });
    return row;
  }

  function renderList() {
    listNode.innerHTML = "";
    const dayMatches = matches.filter(function (match) {
      return dateKey(match.startsAt) === selectedDay;
    });
    if (!dayMatches.length) {
      const empty = document.createElement("div");
      empty.className = "empty-state empty-state-inline";
      empty.innerHTML = "<strong>На этот день матчей нет</strong><span>Выбери другой день или перенеси матч в кабинете.</span>";
      listNode.appendChild(empty);
      return;
    }
    dayMatches.forEach(function (match) {
      listNode.appendChild(renderMatch(match));
    });
  }

  function render() {
    renderDays();
    renderList();
  }

  async function load() {
    const current = await requestJSON("/api/tournament/current");
    tournament = current ? current.tournament : null;
    if (!tournament) {
      matches = [];
      setText(title, "Турнир не запущен");
      setText(message, "Сначала запусти турнир в кабинете администратора.");
      render();
      return;
    }
    const payload = await requestJSON("/api/admin/tournament/" + encodeURIComponent(tournament.id) + "/matches/control");
    matches = payload && Array.isArray(payload.matches) ? payload.matches : [];
    setText(title, tournament.phase === "qualification" ? "Квалификация идет" : "Матчи готовы к старту");
    setText(message, matches.length ? "Игроки должны войти в свои миры через мод. После двух подключений появится кнопка старта." : "Сетка откроется после квалификации.");
    render();
  }

  async function startMatch(matchID) {
    if (!tournament || !matchID) {
      return;
    }
    setText(message, "Запускаю матч...");
    try {
      await requestJSON("/api/admin/tournament/" + encodeURIComponent(tournament.id) + "/matches/" + encodeURIComponent(matchID) + "/start", {
        method: "POST",
      });
      setText(message, "Матч запущен. Мод начнет таймер у обоих игроков при следующей проверке.");
      await load();
    } catch (error) {
      setText(message, error.message);
    }
  }

  async function resetMatch(matchID) {
    if (!tournament || !matchID) {
      return;
    }
    setText(message, "Сбрасываю матч...");
    try {
      await requestJSON("/api/admin/tournament/" + encodeURIComponent(tournament.id) + "/matches/" + encodeURIComponent(matchID) + "/reset", {
        method: "POST",
      });
      setText(message, "Матч сброшен. Игроки могут заново войти в мир, чтобы начать забег.");
      await load();
    } catch (error) {
      setText(message, error.message);
    }
  }

  load().catch(function (error) {
    setText(title, "Не удалось загрузить");
    setText(message, error.message);
  });
  window.setInterval(function () {
    load().catch(function () {});
  }, 3000);
});
