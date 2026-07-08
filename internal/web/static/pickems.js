document.addEventListener("DOMContentLoaded", function () {
  const board = document.querySelector(".pickem-bracket-board");
  if (!board) return;

  const predictions = { ...(window.TOURNAMENT_PREDICTIONS || {}) };
  const isLoggedIn = Boolean(window.IS_LOGGED_IN);
  const isSubmitted = Boolean(window.TOURNAMENT_PICKEMS_SUBMITTED);
  const submitButton = document.querySelector("[data-pickems-submit]");
  const progress = document.querySelector("[data-pickems-progress]");
  const matches = Array.from(board.querySelectorAll(".bracket-match"));

  function matchFor(round, position) {
    return board.querySelector(`.bracket-match[data-round="${round}"][data-position="${position}"]`);
  }

  function selectionFor(match) {
    const winnerID = predictions[match.dataset.matchId];
    return Array.from(match.querySelectorAll(".pickem-option")).find(function (option) {
      return option.dataset.playerId && option.dataset.playerId === winnerID;
    });
  }

  function nextSlot(round, position) {
    if (round === "quarterfinal") {
      return { round: "semifinal", position: Math.ceil(position / 2), slot: position % 2 === 1 ? 1 : 2 };
    }
    if (round === "semifinal") {
      return { round: "final", position: 1, slot: position };
    }
    return null;
  }

  function clearFrom(match) {
    if (!match) return;
    delete predictions[match.dataset.matchId];
    match.querySelectorAll(".pickem-option").forEach(function (option) {
      option.classList.remove("is-selected");
    });

    const target = nextSlot(match.dataset.round, Number(match.dataset.position));
    if (!target) return;
    const targetMatch = matchFor(target.round, target.position);
    const targetOption = targetMatch && targetMatch.querySelector(`.pickem-option[data-player-slot="${target.slot}"]`);
    if (targetOption) setSlot(targetMatch, targetOption, null);
  }

  function setSlot(targetMatch, option, source) {
    const oldPlayerID = option.dataset.playerId || "";
    const newPlayerID = source ? source.dataset.playerId : "";
    if (oldPlayerID && oldPlayerID !== newPlayerID && predictions[targetMatch.dataset.matchId] === oldPlayerID) {
      clearFrom(targetMatch);
    }

    option.dataset.playerId = newPlayerID;
    option.dataset.playerName = source ? source.dataset.playerName : "";
    option.dataset.playerSeed = source ? source.dataset.playerSeed : "";
    option.querySelector(".pickem-option-name").textContent = source ? source.dataset.playerName : "Ожидается выбор";
    option.querySelector(".pickem-option-seed").textContent = source ? source.dataset.playerSeed : "";
    option.disabled = isSubmitted || !isLoggedIn || !source || targetMatch.dataset.status !== "scheduled";
  }

  function propagate(match, option) {
    const target = nextSlot(match.dataset.round, Number(match.dataset.position));
    if (!target) return;
    const targetMatch = matchFor(target.round, target.position);
    if (!targetMatch) return;
    const targetOption = targetMatch.querySelector(`.pickem-option[data-player-slot="${target.slot}"]`);
    if (!targetOption) return;
    setSlot(targetMatch, targetOption, option);

    const selected = selectionFor(targetMatch);
    if (selected) {
      selected.classList.add("is-selected");
      propagate(targetMatch, selected);
    }
  }

  function select(match, option) {
    match.querySelectorAll(".pickem-option").forEach(function (candidate) {
      candidate.classList.toggle("is-selected", candidate === option);
    });
    predictions[match.dataset.matchId] = option.dataset.playerId;
    propagate(match, option);
    updateProgress();
  }

  function updateProgress() {
    const selectedCount = matches.filter(function (match) {
      return Boolean(selectionFor(match));
    }).length;
    if (progress) progress.textContent = `Выбрано ${selectedCount} из ${matches.length} матчей`;
    if (submitButton) submitButton.disabled = selectedCount !== matches.length || isSubmitted;
  }

  ["quarterfinal", "semifinal", "final"].forEach(function (round) {
    matches.filter(function (match) { return match.dataset.round === round; }).forEach(function (match) {
      const selected = selectionFor(match);
      if (selected) {
        selected.classList.add("is-selected");
        propagate(match, selected);
      }
    });
  });

  matches.forEach(function (match) {
    match.querySelectorAll(".pickem-option").forEach(function (option) {
      if (isSubmitted) option.disabled = true;
      option.addEventListener("click", function () {
        if (isSubmitted || option.disabled || !option.dataset.playerId) return;
        select(match, option);
      });
    });
  });

  if (submitButton) {
    submitButton.addEventListener("click", async function () {
      if (submitButton.disabled) return;
      const confirmed = window.confirm("Подтвердить прогноз? После подтверждения изменить или начать сетку заново будет нельзя.");
      if (!confirmed) return;

      submitButton.disabled = true;
      submitButton.textContent = "Сохраняем...";
      try {
        const response = await fetch("/api/predictions", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            predictions: matches.map(function (match) {
              return { matchId: match.dataset.matchId, predictedWinnerId: predictions[match.dataset.matchId] };
            }),
          }),
        });
        if (response.status === 401) {
          window.location.href = "/api/auth/twitch/start";
          return;
        }
        if (!response.ok) {
          const payload = await response.json().catch(function () { return {}; });
          throw new Error(payload.error || "Не удалось подтвердить прогноз");
        }
        window.location.reload();
      } catch (error) {
        window.alert(error.message);
        submitButton.textContent = "Подтвердить прогноз";
        updateProgress();
      }
    });
  }

  updateProgress();
});
