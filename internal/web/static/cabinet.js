document.addEventListener("DOMContentLoaded", function () {
  const avatarForm = document.querySelector("[data-avatar-form]");
  if (avatarForm) {
    const avatarInput = avatarForm.querySelector("[data-avatar-input]");
    const avatarImage = document.querySelector("[data-avatar-image]");
    const avatarTrigger = avatarForm.querySelector("[data-avatar-trigger]");

    avatarInput.addEventListener("change", function () {
      if (!avatarInput.files || avatarInput.files.length === 0) {
        return;
      }

      if (avatarImage) {
        avatarImage.src = URL.createObjectURL(avatarInput.files[0]);
      }
      if (avatarTrigger) {
        avatarTrigger.textContent = "Р—Р°РіСЂСѓР·РєР°...";
      }
      avatarForm.submit();
    });
  }

  const nickForm = document.querySelector("[data-nick-form]");
  if (nickForm) {
    const nickInput = nickForm.querySelector("[data-nick-input]");
    const nickToggle = nickForm.querySelector("[data-nick-toggle]");

    nickToggle.addEventListener("click", function () {
      if (!nickForm.classList.contains("is-editing")) {
        nickForm.classList.add("is-editing");
        nickToggle.setAttribute("aria-label", "РЎРѕС…СЂР°РЅРёС‚СЊ РЅРёРє");
        nickInput.focus();
        nickInput.select();
        return;
      }

      nickForm.requestSubmit();
    });

    nickInput.addEventListener("keydown", function (event) {
      if (event.key === "Escape") {
        nickForm.classList.remove("is-editing");
        nickInput.value = nickInput.defaultValue;
        nickToggle.setAttribute("aria-label", "Р РµРґР°РєС‚РёСЂРѕРІР°С‚СЊ РЅРёРє");
        nickToggle.focus();
      }
    });
  }

  const tokenInput = document.querySelector("[data-token-input]");
  const tokenCopy = document.querySelector("[data-token-copy]");
  const newsPromo = document.querySelector("[data-news-promo]");
  const newsPromoClose = document.querySelector("[data-news-promo-close]");
  if (newsPromo) {
    const promoKey = "tournament-news-promo-v2";
    let promoClosed = false;
    try {
      promoClosed = localStorage.getItem(promoKey) === "closed";
    } catch (_) {}
    if (!promoClosed) {
      newsPromo.hidden = false;
    }
    if (newsPromoClose) {
      newsPromoClose.addEventListener("click", function () {
        try {
          localStorage.setItem(promoKey, "closed");
        } catch (_) {}
        newsPromo.hidden = true;
      });
    }
  }

  if (tokenInput) {
    tokenInput.addEventListener("click", function () {
      if (tokenInput.dataset.hasFullToken !== "true") {
        return;
      }
      tokenInput.classList.toggle("is-visible");
      if (tokenInput.classList.contains("is-visible")) {
        tokenInput.focus();
        tokenInput.select();
      }
    });

    tokenInput.addEventListener("keydown", function (event) {
      if (event.key === "Escape") {
        tokenInput.classList.remove("is-visible");
        tokenInput.blur();
      }
    });
  }

  if (tokenInput && tokenCopy) {
    let tokenCopyTimer = 0;
    const tokenCopyDefaultHTML = tokenCopy.innerHTML;
    const tokenCopyDefaultLabel = tokenCopy.getAttribute("aria-label") || "РЎРєРѕРїРёСЂРѕРІР°С‚СЊ С‚РѕРєРµРЅ";
    const tokenCopyStatus = document.querySelector("[data-token-copy-status]");

    tokenCopy.addEventListener("click", async function () {
      if (!tokenInput.value || tokenInput.dataset.hasFullToken !== "true") {
        return;
      }
      try {
        await navigator.clipboard.writeText(tokenInput.value);
      } catch (_) {
        tokenInput.select();
        document.execCommand("copy");
      }
      window.clearTimeout(tokenCopyTimer);
      tokenCopy.classList.add("is-copied");
      tokenCopy.setAttribute("aria-label", "РЎРєРѕРїРёСЂРѕРІР°РЅРѕ");
      tokenCopy.innerHTML = '<span class="token-copy-feedback" aria-hidden="true">вњ“</span>';
      if (tokenCopyStatus) {
        tokenCopyStatus.hidden = false;
      }
      tokenCopyTimer = window.setTimeout(function () {
        tokenCopy.classList.remove("is-copied");
        tokenCopy.setAttribute("aria-label", tokenCopyDefaultLabel);
        tokenCopy.innerHTML = tokenCopyDefaultHTML;
        if (tokenCopyStatus) {
          tokenCopyStatus.hidden = true;
        }
      }, 1800);
    });
  }

  const nowPlayingForm = document.querySelector("[data-now-playing-form]");
  if (nowPlayingForm) {
    const nowPlayingCheckbox = nowPlayingForm.querySelector("[data-now-playing-checkbox]");

    if (nowPlayingCheckbox) {
      nowPlayingCheckbox.addEventListener("change", function () {
        nowPlayingForm.requestSubmit();
      });
    }
  }

  const achievementsModal = document.querySelector("[data-achievements-modal]");
  const achievementsOpen = document.querySelector("[data-achievements-open]");
  const achievementsCloseButtons = document.querySelectorAll("[data-achievements-close]");

  function setAchievementsOpen(open) {
    if (!achievementsModal) {
      return;
    }
    achievementsModal.hidden = !open;
    document.body.classList.toggle("achievements-open", open);
  }

  if (achievementsModal && achievementsOpen) {
    achievementsOpen.addEventListener("click", function () {
      setAchievementsOpen(true);
    });

    achievementsCloseButtons.forEach(function (button) {
      button.addEventListener("click", function () {
        setAchievementsOpen(false);
      });
    });

    document.addEventListener("keydown", function (event) {
      if (event.key === "Escape" && !achievementsModal.hidden) {
        setAchievementsOpen(false);
        achievementsOpen.focus();
      }
    });
  }

  const eventBadgesModal = document.querySelector("[data-event-badges-modal]");
  const eventBadgesOpen = document.querySelector("[data-event-badges-open]");
  const eventBadgesCloseButtons = document.querySelectorAll("[data-event-badges-close]");

  function setEventBadgesOpen(open) {
    if (!eventBadgesModal) {
      return;
    }
    eventBadgesModal.hidden = !open;
    document.body.classList.toggle("event-badges-open", open);
  }

  if (eventBadgesModal && eventBadgesOpen) {
    eventBadgesOpen.addEventListener("click", function () {
      setEventBadgesOpen(true);
    });

    eventBadgesCloseButtons.forEach(function (button) {
      button.addEventListener("click", function () {
        setEventBadgesOpen(false);
      });
    });

    document.addEventListener("keydown", function (event) {
      if (event.key === "Escape" && !eventBadgesModal.hidden) {
        setEventBadgesOpen(false);
        eventBadgesOpen.focus();
      }
    });
  }

  const adminPanel = document.querySelector("[data-admin-panel]");
  if (!adminPanel) {
    return;
  }

  const adminIsDedicatedPage = adminPanel.dataset.adminPage === "true";
  const adminToggle = document.querySelector("[data-admin-toggle]");
  const adminApplicationsOpen = document.querySelector("[data-admin-applications-open]");
  const adminClose = document.querySelector("[data-admin-close]");
  const applicationsToggle = document.querySelector("[data-applications-toggle]");
  const discordSyncRoles = document.querySelector("[data-discord-sync-roles]");
  const newsSync = document.querySelector("[data-news-sync]");
  const applicationsPanel = document.querySelector("[data-applications-panel]");
  const whitelistForm = document.querySelector("[data-whitelist-form]");
  const whitelistMessage = document.querySelector("[data-whitelist-message]");
  const participantsList = document.querySelector("[data-participants-list]");
  const applicationsMessage = document.querySelector("[data-applications-message]");
  const applicationsList = document.querySelector("[data-applications-list]");
  const tournamentStatus = document.querySelector("[data-tournament-status]");
  const tournamentMessage = document.querySelector("[data-tournament-message]");
  const tournamentStart = document.querySelector("[data-tournament-start]");
  const tournamentScheduleForm = document.querySelector("[data-tournament-schedule-form]");
  const tournamentCalendar = document.querySelector("[data-tournament-calendar]");
  const tournamentCalendarGrid = document.querySelector("[data-calendar-grid]");
  const tournamentCalendarSummary = document.querySelector("[data-calendar-summary]");
  const tournamentCalendarSelected = document.querySelector("[data-calendar-selected]");
  const tournamentMatchSeedField = document.querySelector("[data-match-seed-field]");
  const tournamentMatchSeedInput = document.querySelector("[data-match-seed-input]");
  const tournamentMatchList = document.querySelector("[data-calendar-match-list]");
  const tournamentMatchEditorPanel = document.querySelector("[data-match-editor-panel]");
  const tournamentMatchDateInput = document.querySelector("[data-match-date-input]");
  const tournamentMatchStartTimeInput = document.querySelector("[data-match-start-time-input]");
  const tournamentMatchEndTimeInput = document.querySelector("[data-match-end-time-input]");
  const tournamentStop = document.querySelector("[data-tournament-stop]");
  const leaderboardClear = document.querySelector("[data-leaderboard-clear]");
  const testModeStatus = document.querySelector("[data-test-mode-status]");
  const testModeEnable = document.querySelector("[data-test-mode-enable]");
  const testModeDisable = document.querySelector("[data-test-mode-disable]");
  const testRunsClear = document.querySelector("[data-test-runs-clear]");
  const applicationsCloseToggle = document.querySelector("[data-applications-close-toggle]");
  let currentTournament = null;
  let currentMatches = [];
  let selectedMatchID = "";
  const dayMS = 24 * 60 * 60 * 1000;

  function setPanelOpen(open) {
    adminPanel.hidden = !open;
    if (open) {
      loadAdminData();
    }
  }

  function setApplicationsOpen(open) {
    if (!applicationsPanel || !applicationsToggle) {
      return;
    }
    applicationsPanel.hidden = !open;
    applicationsToggle.setAttribute("aria-expanded", open ? "true" : "false");
    if (open) {
      applicationsToggle.textContent = "РЎРєСЂС‹С‚СЊ Р·Р°СЏРІРєРё";
      loadApplications().catch(function (error) {
        setText(applicationsMessage, error.message);
      });
    } else {
      applicationsToggle.textContent = "Р—Р°СЏРІРєРё РЅР° СѓС‡Р°СЃС‚РёРµ";
    }
  }

  function setText(node, text) {
    if (node) {
      node.textContent = text;
    }
  }

  function parseFormDate(fieldName) {
    if (!tournamentScheduleForm || !tournamentScheduleForm.elements[fieldName].value) {
      return null;
    }
    const date = new Date(fromDatetimeLocal(tournamentScheduleForm.elements[fieldName].value));
    if (Number.isNaN(date.getTime())) {
      return null;
    }
    return date;
  }

  function tournamentDay(startsAt, value) {
    if (!startsAt || !value) {
      return 0;
    }
    return Math.max(1, Math.ceil((value.getTime() - startsAt.getTime()) / dayMS));
  }

  function dateLabel(date) {
    return date.toLocaleDateString("ru-RU", { day: "2-digit", month: "2-digit", timeZone: "Europe/Moscow" });
  }

  function calendarStartDay(startsAt, value) {
    if (!startsAt || !value) {
      return 0;
    }
    return Math.max(1, Math.floor((value.getTime() - startsAt.getTime()) / dayMS) + 1);
  }

  function roundLabel(round) {
    switch (round) {
      case "quarterfinal": return "1/4";
      case "semifinal": return "1/2";
      case "final": return "Р¤РёРЅР°Р»";
      case "third_place": return "Р—Р° 3 РјРµСЃС‚Рѕ";
      default: return round || "РњР°С‚С‡";
    }
  }

  function participantName(participant) {
    if (!participant) {
      return "РѕР¶РёРґР°РµС‚";
    }
    return participantValue(participant, "twitchDisplayName")
      || participantValue(participant, "twitchLogin")
      || participantValue(participant, "minecraftNick")
      || "РѕР¶РёРґР°РµС‚";
  }

  function matchTitle(match) {
    return roundLabel(match.round) + " #" + match.position + ": " + participantName(match.player1) + " vs " + participantName(match.player2);
  }

  function selectedMatch() {
    return currentMatches.find(function (match) {
      return match.id === selectedMatchID;
    }) || null;
  }

  function matchScheduleLabel(match) {
    if (!match || !match.startsAt) {
      return "РґР°С‚Р° РЅРµ РЅР°Р·РЅР°С‡РµРЅР°";
    }
    const date = new Date(match.startsAt);
    return date.toLocaleDateString("ru-RU", { day: "2-digit", month: "2-digit", year: "2-digit", timeZone: "Europe/Moscow" });
  }

  function toMSKParts(isoString) {
    const date = new Date(isoString);
    if (Number.isNaN(date.getTime())) {
      return { date: "", time: "" };
    }
    const mskDate = new Date(date.getTime() + 3 * 3600 * 1000);
    const iso = mskDate.toISOString();
    return {
      date: iso.slice(0, 10),
      time: iso.slice(11, 16)
    };
  }

  function renderSelectedMatch() {
    const match = selectedMatch();
    if (tournamentCalendarSelected) {
      tournamentCalendarSelected.textContent = match
        ? "Р’С‹Р±СЂР°РЅ: " + matchTitle(match) + " / " + matchScheduleLabel(match)
        : "РњР°С‚С‡ РЅРµ РІС‹Р±СЂР°РЅ";
    }
    if (tournamentMatchEditorPanel) {
      tournamentMatchEditorPanel.hidden = !match;
    }
    if (match) {
      const starts = toMSKParts(match.startsAt);
      const ends = toMSKParts(match.endsAt);
      if (tournamentMatchDateInput) {
        tournamentMatchDateInput.value = starts.date;
      }
      if (tournamentMatchStartTimeInput) {
        tournamentMatchStartTimeInput.value = starts.time;
      }
      if (tournamentMatchEndTimeInput) {
        tournamentMatchEndTimeInput.value = ends.time;
      }
      if (tournamentMatchSeedInput) {
        tournamentMatchSeedInput.value = match.worldSeed || "";
      }
    }
  }

  function renderTournamentMatchList() {
    if (!tournamentMatchList) {
      return;
    }
    tournamentMatchList.innerHTML = "";
    if (!currentMatches || currentMatches.length === 0) {
      const empty = document.createElement("p");
      empty.className = "muted";
      empty.textContent = "РњР°С‚С‡Рё РїРѕСЏРІСЏС‚СЃСЏ Р·РґРµСЃСЊ РїРѕСЃР»Рµ РѕРєРѕРЅС‡Р°РЅРёСЏ РєРІР°Р»РёС„РёРєР°С†РёРё.";
      tournamentMatchList.appendChild(empty);
      renderSelectedMatch();
      return;
    }

    currentMatches.forEach(function (match) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "admin-match-chip";
      button.classList.toggle("is-selected", match.id === selectedMatchID);
      button.innerHTML = [
        "<strong></strong>",
        "<span></span>",
        "<small></small>",
      ].join("");
      button.querySelector("strong").textContent = roundLabel(match.round) + " #" + match.position;
      button.querySelector("span").textContent = participantName(match.player1) + " vs " + participantName(match.player2);
      button.querySelector("small").textContent = matchScheduleLabel(match) + (match.worldSeed ? " В· seed " + match.worldSeed : "");
      button.addEventListener("click", function () {
        selectedMatchID = match.id;
        renderTournamentCalendar();
        renderTournamentMatchList();
      });
      tournamentMatchList.appendChild(button);
    });
    renderSelectedMatch();
  }

  function setSelectedMatchByDay(dayNumber) {
    const startsAt = parseFormDate("startsAt");
    const qualificationEndsAt = parseFormDate("qualificationEndsAt");
    const endsAt = parseFormDate("endsAt");
    const match = selectedMatch();
    if (!startsAt || !qualificationEndsAt || !endsAt || !match) {
      setText(tournamentMessage, "РЎРЅР°С‡Р°Р»Р° РІС‹Р±РµСЂРёС‚Рµ РјР°С‚С‡ РїРѕРґ РєР°Р»РµРЅРґР°СЂРµРј.");
      return;
    }
    const nextStart = new Date(startsAt.getTime() + (dayNumber - 1) * dayMS);
    if (nextStart < qualificationEndsAt) {
      setText(tournamentMessage, "РњР°С‚С‡ РЅРµР»СЊР·СЏ РїРѕСЃС‚Р°РІРёС‚СЊ РґРѕ РєРѕРЅС†Р° РєРІР°Р»РёС„РёРєР°С†РёРё.");
      return;
    }
    const nextEnd = new Date(Math.min(nextStart.getTime() + dayMS, endsAt.getTime()));
    match.startsAt = nextStart.toISOString();
    match.endsAt = nextEnd.toISOString();
    renderTournamentCalendar();
    renderTournamentMatchList();
    setText(tournamentMessage, "Р”РµРЅСЊ РјР°С‚С‡Р° РёР·РјРµРЅРµРЅ. РќР°Р¶РјРёС‚Рµ В«РЎРѕС…СЂР°РЅРёС‚СЊ РєР°Р»РµРЅРґР°СЂСЊВ».");
  }

  function normalizeSeedInput(value) {
    return (value || "").trim().replace(/[^\d-]/g, "").replace(/(?!^)-/g, "");
  }

  function renderTournamentCalendar() {
    if (!tournamentCalendarGrid || !tournamentScheduleForm || !currentTournament || currentTournament.state === "finished") {
      return;
    }

    const startsAt = parseFormDate("startsAt");
    const qualificationEndsAt = parseFormDate("qualificationEndsAt");
    const playoffEndsAt = parseFormDate("playoffEndsAt");
    const endsAt = parseFormDate("endsAt");
    if (!startsAt || !qualificationEndsAt || !playoffEndsAt || !endsAt) {
      return;
    }

    const qualificationDay = tournamentDay(startsAt, qualificationEndsAt);
    const playoffDay = tournamentDay(startsAt, playoffEndsAt);
    const endDay = tournamentDay(startsAt, endsAt);
    const quarterEndDay = qualificationDay + Math.max(1, Math.ceil((playoffDay - qualificationDay) / 2));
    const totalDays = Math.min(Math.max(endDay, 1), 90);
    const matchesByDay = new Map();
    currentMatches.forEach(function (match) {
      const matchDay = calendarStartDay(startsAt, new Date(match.startsAt));
      if (!matchesByDay.has(matchDay)) {
        matchesByDay.set(matchDay, []);
      }
      matchesByDay.get(matchDay).push(match);
    });
    const activeMatch = selectedMatch();
    const activeMatchDay = activeMatch ? calendarStartDay(startsAt, new Date(activeMatch.startsAt)) : 0;

    tournamentCalendarGrid.innerHTML = "";
    for (let day = 1; day <= totalDays; day += 1) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "admin-calendar-day";
      button.dataset.day = String(day);

      if (day <= qualificationDay) {
        button.classList.add("is-qualification");
      } else if (day <= quarterEndDay) {
        button.classList.add("is-quarter");
      } else if (day <= playoffDay) {
        button.classList.add("is-semi");
      } else {
        button.classList.add("is-final");
      }

      if (day === qualificationDay) {
        button.classList.add("is-boundary");
        button.dataset.boundary = "РљРІР°Р»С‹";
      }
      if (day === playoffDay) {
        button.classList.add("is-boundary");
        button.dataset.boundary = "РџР»РµР№-РѕС„С„";
      }
      if (day === endDay) {
        button.classList.add("is-boundary");
        button.dataset.boundary = "Р¤РёРЅР°Р»";
      }
      if (matchesByDay.has(day)) {
        button.classList.add("has-match");
      }
      if (day === activeMatchDay) {
        button.classList.add("is-selected-match");
      }

      const displayDate = new Date(startsAt.getTime() + (day - 1) * dayMS);
      const dayMatches = matchesByDay.get(day) || [];
      button.innerHTML = [
        "<span>Р”РµРЅСЊ " + day + "</span>",
        "<small>" + dateLabel(displayDate) + "</small>",
        dayMatches.length ? "<em>" + dayMatches.length + " РјР°С‚С‡" + (dayMatches.length > 1 ? "Р°" : "") + "</em>" : "",
      ].join("");
      button.addEventListener("click", function () {
        setSelectedMatchByDay(day);
      });
      tournamentCalendarGrid.appendChild(button);
    }

    if (tournamentCalendarSummary) {
      tournamentCalendarSummary.textContent = currentMatches.length
        ? "РљР»РёРєРЅРёС‚Рµ РјР°С‚С‡ РЅРёР¶Рµ, РїРѕС‚РѕРј РґРµРЅСЊ РІ РєР°Р»РµРЅРґР°СЂРµ. РќРµСЃРєРѕР»СЊРєРѕ РјР°С‚С‡РµР№ РјРѕР¶РЅРѕ РїРѕСЃС‚Р°РІРёС‚СЊ РЅР° РѕРґРёРЅ РґРµРЅСЊ."
        : "РњР°С‚С‡Рё РїРѕСЏРІСЏС‚СЃСЏ РїРѕСЃР»Рµ РѕРєРѕРЅС‡Р°РЅРёСЏ РєРІР°Р»РёС„РёРєР°С†РёРё.";
    }
    renderSelectedMatch();
  }

  function setTournamentControls() {
    const hasTournament = Boolean(currentTournament);
    const canEditSchedule = hasTournament && currentTournament.state !== "finished";
    const canStart = !currentTournament || currentTournament.state === "finished";
    if (tournamentScheduleForm) {
      tournamentScheduleForm.hidden = !canEditSchedule;
    }
    if (tournamentCalendar) {
      tournamentCalendar.hidden = !canEditSchedule;
    }
    if (tournamentStart) {
      tournamentStart.disabled = !canStart;
    }
    if (tournamentStop) {
      tournamentStop.disabled = !hasTournament || currentTournament.state === "finished";
    }
  }

  function participantValue(participant, key) {
    return participant[key] || participant[key.charAt(0).toUpperCase() + key.slice(1)] || "";
  }

  function renderParticipants(participants) {
    if (!participantsList) {
      return;
    }
    participantsList.innerHTML = "";

    if (!participants || participants.length === 0) {
      const empty = document.createElement("div");
      empty.className = "admin-empty-state";
      empty.innerHTML = [
        "<strong>Whitelist РїСѓСЃС‚</strong>",
        "<span>Р”РѕР±Р°РІСЊС‚Рµ Twitch login СѓС‡Р°СЃС‚РЅРёРєР° РІС‹С€Рµ. РџРѕСЃР»Рµ РІС…РѕРґР° С‡РµСЂРµР· РєР°Р±РёРЅРµС‚ РѕРЅ РїРѕСЏРІРёС‚СЃСЏ Р·РґРµСЃСЊ СЃРѕ СЃС‚Р°С‚СѓСЃРѕРј СЃС‚СЂРёРјР°.</span>",
      ].join("");
      participantsList.appendChild(empty);
      return;
    }

    participants.forEach(function (participant) {
      const login = participantValue(participant, "twitchLogin");
      const status = participantValue(participant, "status");
      const userID = participantValue(participant, "twitchUserID");
      const streamOnline = Boolean(participantValue(participant, "streamOnline"));
      const isPending = userID && String(userID).startsWith("pending:");
      const row = document.createElement("div");
      row.className = "admin-list-row";
      row.innerHTML = [
        "<div class=\"admin-player-main\"><span class=\"admin-player-login\"></span><span class=\"admin-player-status\"></span></div>",
        "<span class=\"admin-stream-state\"><i aria-hidden=\"true\"></i><small></small></span>",
        "<div class=\"admin-list-actions\"></div>",
      ].join("");
      row.querySelector(".admin-player-login").textContent = login;
      row.querySelector(".admin-player-status").textContent = "status: " + status;
      row.querySelector(".admin-stream-state").classList.add(streamOnline ? "is-live" : "is-offline");
      row.querySelector("small").textContent = isPending
        ? status + " / РѕР¶РёРґР°РµС‚ РІС…РѕРґ"
        : (status === "blocked" ? "СѓРґР°Р»РµРЅ РёР· whitelist" : (streamOnline ? "СЃС‚СЂРёРјРёС‚" : "РЅРµ СЃС‚СЂРёРјРёС‚"));
      const actions = row.querySelector(".admin-list-actions");
      const button = document.createElement("button");
      button.type = "button";
      button.className = status === "blocked" ? "button-secondary" : "button-danger";
      button.textContent = status === "blocked" ? "Р’РµСЂРЅСѓС‚СЊ" : "РЈРґР°Р»РёС‚СЊ РёР· whitelist";
      button.addEventListener("click", function () {
        updateParticipantStatus(userID, status === "blocked" ? "invited" : "blocked");
      });
      actions.appendChild(button);
      participantsList.appendChild(row);
    });
  }

  function applicationValue(application, key) {
    return application[key] || application[key.charAt(0).toUpperCase() + key.slice(1)] || "";
  }

  function applicationStatusLabel(status) {
    switch (status) {
      case "pending": return "РЅР° РїСЂРѕРІРµСЂРєРµ";
      case "approved": return "РѕРґРѕР±СЂРµРЅР°";
      case "rejected": return "РѕС‚РєР»РѕРЅРµРЅР°";
      default: return status || "РЅРµРёР·РІРµСЃС‚РЅРѕ";
    }
  }

  function renderApplications(applications) {
    if (!applicationsList) {
      return;
    }
    applicationsList.innerHTML = "";
    const pendingCount = (applications || []).filter(function (application) {
      return applicationValue(application, "status") === "pending";
    }).length;
    if (applicationsToggle && applicationsPanel && applicationsPanel.hidden) {
      applicationsToggle.textContent = pendingCount
        ? "Р—Р°СЏРІРєРё РЅР° СѓС‡Р°СЃС‚РёРµ (" + pendingCount + ")"
        : "Р—Р°СЏРІРєРё РЅР° СѓС‡Р°СЃС‚РёРµ";
    }
    setText(applicationsMessage, pendingCount
      ? "РќРѕРІС‹С… Р·Р°СЏРІРѕРє: " + pendingCount
      : "РќРѕРІС‹С… Р·Р°СЏРІРѕРє РЅРµС‚.");

    if (!applications || applications.length === 0) {
      const empty = document.createElement("div");
      empty.className = "admin-empty-state";
      empty.innerHTML = [
        "<strong>Р—Р°СЏРІРѕРє РїРѕРєР° РЅРµС‚</strong>",
        "<span>РљРѕРіРґР° СЃС‚СЂРёРјРµСЂ РїСЂРѕР№РґРµС‚ СЃС‚СЂР°РЅРёС†Сѓ /apply, Р·Р°СЏРІРєР° РїРѕСЏРІРёС‚СЃСЏ Р·РґРµСЃСЊ РІРјРµСЃС‚Рµ СЃ ref.</span>",
      ].join("");
      applicationsList.appendChild(empty);
      return;
    }

    applications.forEach(function (application) {
      const id = applicationValue(application, "id");
      const number = applicationValue(application, "applicationNumber");
      const status = applicationValue(application, "status");
      const row = document.createElement("article");
      row.className = "admin-application-row";
      row.classList.add("is-" + status);

      const header = document.createElement("div");
      header.className = "admin-application-header";
      const title = document.createElement("strong");
      const displayName = applicationValue(application, "twitchDisplayName")
        || applicationValue(application, "twitchLogin")
        || "unknown";
      title.textContent = (number ? "#" + number + " " : "") + displayName;
      const badge = document.createElement("span");
      badge.className = "admin-application-status";
      badge.textContent = applicationStatusLabel(status);
      header.append(title, badge);

      const meta = document.createElement("div");
      meta.className = "admin-application-meta";
      const channel = document.createElement("a");
      channel.href = applicationValue(application, "twitchChannelUrl") || ("https://www.twitch.tv/" + applicationValue(application, "twitchLogin"));
      channel.target = "_blank";
      channel.rel = "noopener noreferrer";
      channel.textContent = "Twitch";
      const discord = document.createElement("span");
      discord.textContent = "Discord: " + (applicationValue(application, "discordUsername") || "-");
      const timezone = document.createElement("span");
      timezone.textContent = "TZ: " + (applicationValue(application, "timezone") || "-");
      const referral = document.createElement("span");
      referral.textContent = "ref: " + (applicationValue(application, "referral") || "-");
      meta.append(channel, discord, timezone, referral);

      const footer = document.createElement("div");
      footer.className = "admin-application-footer";
      const createdAt = applicationValue(application, "createdAt");
      const date = document.createElement("small");
      date.textContent = createdAt ? new Date(createdAt).toLocaleString("ru-RU", { timeZone: "Europe/Moscow" }) : "";
      footer.appendChild(date);

      if (status === "pending") {
        const actions = document.createElement("div");
        actions.className = "admin-application-actions";
        const approve = document.createElement("button");
        approve.type = "button";
        approve.className = "button-secondary";
        approve.textContent = "РћРґРѕР±СЂРёС‚СЊ";
        approve.addEventListener("click", function () {
          approveApplication(id);
        });
        const reject = document.createElement("button");
        reject.type = "button";
        reject.className = "button-danger";
        reject.textContent = "РћС‚РєР»РѕРЅРёС‚СЊ";
        reject.addEventListener("click", function () {
          rejectApplication(id);
        });
        actions.append(approve, reject);
        footer.appendChild(actions);
      }

      row.append(header, meta, footer);
      applicationsList.appendChild(row);
    });
  }

  async function requestJSON(url, options) {
    const response = await fetch(url, Object.assign({
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
    }, options || {}));

    if (!response.ok) {
      let message = "РћС€РёР±РєР° Р·Р°РїСЂРѕСЃР°";
      try {
        const payload = await response.json();
        message = payload.error || payload.message || message;
      } catch (_) {}
      throw new Error(message);
    }

    if (response.status === 204) {
      return null;
    }
    return response.json();
  }

  async function loadParticipants() {
    const participants = await requestJSON("/api/admin/participants");
    renderParticipants(participants);
    renderRunsTabParticipants(participants);
  }

  async function loadApplications() {
    const applications = await requestJSON("/api/admin/applications");
    renderApplications(applications);
  }

  async function syncDiscordRoles() {
    if (!discordSyncRoles) {
      return;
    }
    const originalText = discordSyncRoles.textContent;
    discordSyncRoles.disabled = true;
    discordSyncRoles.textContent = "РЎРёРЅС…СЂРѕРЅРёР·РёСЂСѓСЋ...";
    setText(whitelistMessage, "РџСЂРѕРІРµСЂСЏСЋ Discord Рё РІС‹РґР°СЋ СЂРѕР»СЊ Tournament Runner...");
    try {
      const payload = await requestJSON("/api/admin/discord/sync-roles", { method: "POST" });
      setText(whitelistMessage, payload && payload.message ? payload.message : "Р РѕР»Рё СЃРёРЅС…СЂРѕРЅРёР·РёСЂРѕРІР°РЅС‹.");
      await loadParticipants();
    } catch (error) {
      setText(whitelistMessage, error.message);
    } finally {
      discordSyncRoles.disabled = false;
      discordSyncRoles.textContent = originalText;
    }
  }

  async function syncNews() {
    if (!newsSync) {
      return;
    }
    const originalText = newsSync.textContent;
    newsSync.disabled = true;
    newsSync.textContent = "РЎРёРЅС…СЂРѕРЅРёР·РёСЂСѓСЋ...";
    setText(whitelistMessage, "Р—Р°РіСЂСѓР¶Р°СЋ СЃС‚Р°СЂС‹Рµ РЅРѕРІРѕСЃС‚Рё РёР· Telegram...");
    try {
      const payload = await requestJSON("/api/admin/news/sync", { method: "POST" });
      const imported = payload && Number.isFinite(payload.imported) ? payload.imported : 0;
      const seen = payload && Number.isFinite(payload.seen) ? payload.seen : 0;
      setText(whitelistMessage, "РќРѕРІРѕСЃС‚Рё СЃРёРЅС…СЂРѕРЅРёР·РёСЂРѕРІР°РЅС‹: " + imported + " РёР· " + seen + ".");
    } catch (error) {
      setText(whitelistMessage, error.message || "РќРµ СѓРґР°Р»РѕСЃСЊ СЃРёРЅС…СЂРѕРЅРёР·РёСЂРѕРІР°С‚СЊ РЅРѕРІРѕСЃС‚Рё.");
    } finally {
      newsSync.disabled = false;
      newsSync.textContent = originalText;
    }
  }

  function renderTestMode(status) {
    if (!testModeStatus) {
      return;
    }
    const enabled = Boolean(status && status.enabled);
    if (enabled) {
      const until = status.endsAt
        ? new Date(status.endsAt).toLocaleString("ru-RU", { timeZone: "Europe/Moscow" })
        : "СЃРєРѕСЂРѕ";
      testModeStatus.textContent = "Р’РєР»СЋС‡РµРЅ РґРѕ " + until + ". Р’СЃРµ Р·Р°Р±РµРіРё РёР· РјРѕРґР° СЃРѕС…СЂР°РЅСЏСЋС‚СЃСЏ РєР°Рє С‚РµСЃС‚РѕРІС‹Рµ.";
    } else {
      testModeStatus.textContent = "Р’С‹РєР»СЋС‡РµРЅ. РћР±С‹С‡РЅС‹Рµ Р·Р°Р±РµРіРё РёРґСѓС‚ С‚РѕР»СЊРєРѕ РїРѕ РїСЂР°РІРёР»Р°Рј Р°РєС‚РёРІРЅРѕРіРѕ С‚СѓСЂРЅРёСЂР°.";
    }
    if (testModeEnable) {
      testModeEnable.disabled = enabled;
    }
    if (testModeDisable) {
      testModeDisable.disabled = !enabled;
    }
  }

  async function loadTestMode() {
    const status = await requestJSON("/api/admin/test-mode");
    renderTestMode(status);
  }

  async function enableTestMode() {
    if (!testModeEnable) {
      return;
    }
    const originalText = testModeEnable.textContent;
    testModeEnable.disabled = true;
    testModeEnable.textContent = "Р’РєР»СЋС‡Р°СЋ...";
    setText(tournamentMessage, "Р’РєР»СЋС‡Р°СЋ С‚РµСЃС‚РѕРІС‹Р№ СЂРµР¶РёРј РЅР° 5 РґРЅРµР№...");
    try {
      const status = await requestJSON("/api/admin/test-mode", {
        method: "POST",
        body: JSON.stringify({ days: 5 }),
      });
      renderTestMode(status);
      setText(tournamentMessage, "РўРµСЃС‚РѕРІС‹Р№ СЂРµР¶РёРј РІРєР»СЋС‡РµРЅ. Р’СЃРµ Р·Р°Р±РµРіРё РёР· РјРѕРґР° Р±СѓРґСѓС‚ С‚РµСЃС‚РѕРІС‹РјРё.");
    } catch (error) {
      setText(tournamentMessage, error.message);
      renderTestMode({ enabled: false });
    } finally {
      testModeEnable.textContent = originalText;
    }
  }

  async function disableTestMode() {
    if (!testModeDisable) {
      return;
    }
    const originalText = testModeDisable.textContent;
    testModeDisable.disabled = true;
    testModeDisable.textContent = "Р’С‹РєР»СЋС‡Р°СЋ...";
    setText(tournamentMessage, "Р’С‹РєР»СЋС‡Р°СЋ С‚РµСЃС‚РѕРІС‹Р№ СЂРµР¶РёРј...");
    try {
      await requestJSON("/api/admin/test-mode", { method: "DELETE" });
      renderTestMode({ enabled: false });
      setText(tournamentMessage, "РўРµСЃС‚РѕРІС‹Р№ СЂРµР¶РёРј РІС‹РєР»СЋС‡РµРЅ.");
    } catch (error) {
      setText(tournamentMessage, error.message);
      await loadTestMode().catch(function () {});
    } finally {
      testModeDisable.textContent = originalText;
    }
  }

  let applicationsClosed = false;

  async function loadApplicationsStatus() {
    if (!applicationsCloseToggle) {
      return;
    }
    const status = await requestJSON("/api/admin/applications-status");
    applicationsClosed = Boolean(status && status.closed);
    applicationsCloseToggle.textContent = applicationsClosed ? "РћС‚РєСЂС‹С‚СЊ Р·Р°СЏРІРєРё" : "Р—Р°РєСЂС‹С‚СЊ Р·Р°СЏРІРєРё";
    applicationsCloseToggle.classList.toggle("button-danger", !applicationsClosed);
  }

  async function toggleApplicationsStatus() {
    if (!applicationsCloseToggle) {
      return;
    }
    const originalText = applicationsCloseToggle.textContent;
    applicationsCloseToggle.disabled = true;
    applicationsCloseToggle.textContent = applicationsClosed ? "РћС‚РєСЂС‹РІР°СЋ..." : "Р—Р°РєСЂС‹РІР°СЋ...";
    try {
      const nextState = !applicationsClosed;
      const status = await requestJSON("/api/admin/applications-status", {
        method: "POST",
        body: JSON.stringify({ closed: nextState }),
      });
      applicationsClosed = Boolean(status && status.closed);
      applicationsCloseToggle.textContent = applicationsClosed ? "РћС‚РєСЂС‹С‚СЊ Р·Р°СЏРІРєРё" : "Р—Р°РєСЂС‹С‚СЊ Р·Р°СЏРІРєРё";
      applicationsCloseToggle.classList.toggle("button-danger", !applicationsClosed);
    } catch (error) {
      window.alert("РќРµ СѓРґР°Р»РѕСЃСЊ РёР·РјРµРЅРёС‚СЊ СЃС‚Р°С‚СѓСЃ Р·Р°СЏРІРѕРє: " + error.message);
      applicationsCloseToggle.textContent = originalText;
    } finally {
      applicationsCloseToggle.disabled = false;
    }
  }

  async function clearTestRuns() {
    const confirmed = window.confirm("РћС‡РёСЃС‚РёС‚СЊ С‚РѕР»СЊРєРѕ С‚РµСЃС‚РѕРІС‹Рµ Р·Р°Р±РµРіРё? Р РµР°Р»СЊРЅР°СЏ С‚Р°Р±Р»РёС†Р° РЅРµ Р±СѓРґРµС‚ Р·Р°С‚СЂРѕРЅСѓС‚Р°.");
    if (!confirmed) {
      return;
    }
    setText(tournamentMessage, "РћС‡РёС‰Р°СЋ С‚РµСЃС‚РѕРІС‹Рµ Р·Р°Р±РµРіРё...");
    try {
      await requestJSON("/api/admin/test-runs", { method: "DELETE" });
      setText(tournamentMessage, "РўРµСЃС‚РѕРІС‹Рµ Р·Р°Р±РµРіРё РѕС‡РёС‰РµРЅС‹.");
      await loadParticipants();
      await loadTournament();
      await loadTestMode();
    } catch (error) {
      setText(tournamentMessage, error.message);
    }
  }

  async function approveApplication(id) {
    if (!id) {
      return;
    }
    setText(applicationsMessage, "РћРґРѕР±СЂСЏСЋ Р·Р°СЏРІРєСѓ...");
    try {
      await requestJSON("/api/admin/applications/" + encodeURIComponent(id) + "/approve", { method: "POST" });
      setText(applicationsMessage, "Р—Р°СЏРІРєР° РѕРґРѕР±СЂРµРЅР°, СѓС‡Р°СЃС‚РЅРёРє РґРѕР±Р°РІР»РµРЅ РІ whitelist.");
      await loadApplications();
      await loadParticipants();
    } catch (error) {
      setText(applicationsMessage, error.message);
    }
  }

  async function rejectApplication(id) {
    if (!id) {
      return;
    }
    setText(applicationsMessage, "РћС‚РєР»РѕРЅСЏСЋ Р·Р°СЏРІРєСѓ...");
    try {
      await requestJSON("/api/admin/applications/" + encodeURIComponent(id) + "/reject", { method: "POST" });
      setText(applicationsMessage, "Р—Р°СЏРІРєР° РѕС‚РєР»РѕРЅРµРЅР°.");
      await loadApplications();
    } catch (error) {
      setText(applicationsMessage, error.message);
    }
  }

  async function updateParticipantStatus(twitchUserID, status) {
    if (!twitchUserID || !status) {
      return;
    }
    const removing = status === "blocked";
    setText(whitelistMessage, removing ? "РЈРґР°Р»СЏСЋ РёРіСЂРѕРєР° РёР· whitelist..." : "Р’РѕР·РІСЂР°С‰Р°СЋ РёРіСЂРѕРєР° РІ whitelist...");
    try {
      await requestJSON("/api/admin/participants/" + encodeURIComponent(twitchUserID) + "/status", {
        method: "PATCH",
        body: JSON.stringify({ status: status }),
      });
      setText(whitelistMessage, removing ? "РРіСЂРѕРє СѓРґР°Р»РµРЅ РёР· whitelist." : "РРіСЂРѕРє СЃРЅРѕРІР° РІ whitelist.");
      await loadParticipants();
    } catch (error) {
      setText(whitelistMessage, error.message);
    }
  }

  async function loadTournamentMatches() {
    currentMatches = [];
    selectedMatchID = "";
    if (!currentTournament) {
      renderTournamentCalendar();
      renderTournamentMatchList();
      return;
    }
    const payload = await requestJSON("/api/admin/tournament/" + encodeURIComponent(currentTournament.id) + "/matches/control");
    if (payload && payload.tournament && payload.tournament.id === currentTournament.id && Array.isArray(payload.matches)) {
      currentMatches = payload.matches;
      selectedMatchID = currentMatches.length ? currentMatches[0].id : "";
    }
    renderTournamentCalendar();
    renderTournamentMatchList();
  }

  async function loadTournament() {
    const payload = await requestJSON("/api/tournament/current");
    if (!payload || !payload.tournament) {
      currentTournament = null;
      currentMatches = [];
      selectedMatchID = "";
      setTournamentControls();
      renderTournamentCalendar();
      renderTournamentMatchList();
      setText(tournamentStatus, "РўСѓСЂРЅРёСЂ РЅРµ Р·Р°РїСѓС‰РµРЅ. РќР°Р¶РјРёС‚Рµ В«РќР°С‡Р°С‚СЊ С‚СѓСЂРЅРёСЂВ»: РґР°С‚С‹ Р°РІС‚РѕРјР°С‚РёС‡РµСЃРєРё СЂР°СЃСЃС‡РёС‚Р°СЋС‚СЃСЏ РЅР° 30 РґРЅРµР№.");
      return;
    }

    currentTournament = payload.tournament;
    fillScheduleForm(currentTournament);
    setTournamentControls();
    if (currentTournament.state === "finished") {
      currentMatches = [];
      selectedMatchID = "";
      renderTournamentCalendar();
      renderTournamentMatchList();
      setText(tournamentStatus, "РџСЂРѕС€Р»С‹Р№ С‚СѓСЂРЅРёСЂ Р·Р°РІРµСЂС€РµРЅ. РќР°Р¶РјРёС‚Рµ В«РќР°С‡Р°С‚СЊ С‚СѓСЂРЅРёСЂВ»: РЅРѕРІС‹Рµ РґР°С‚С‹ Р°РІС‚РѕРјР°С‚РёС‡РµСЃРєРё СЂР°СЃСЃС‡РёС‚Р°СЋС‚СЃСЏ РЅР° 30 РґРЅРµР№.");
      return;
    }
    await loadTournamentMatches();
    const phase = currentTournament.phase || (payload.isRunning ? "running" : currentTournament.state);
    setText(tournamentStatus, "Р­С‚Р°Рї: " + phaseLabel(phase) + ". РўРѕРї-" + (currentTournament.playoffSlots || 8) + " РїСЂРѕС…РѕРґРёС‚ РІ РїР»РµР№-РѕС„С„.");
  }

  function phaseLabel(phase) {
    switch (phase) {
      case "qualification": return "РєРІР°Р»РёС„РёРєР°С†РёСЏ";
      case "playoff": return "РїР»РµР№-РѕС„С„";
      case "final": return "С„РёРЅР°Р»";
      case "scheduled": return "Р·Р°РїР»Р°РЅРёСЂРѕРІР°РЅ";
      case "finished": return "Р·Р°РІРµСЂС€РµРЅ";
      case "running": return "РёРґРµС‚";
      default: return phase || "РЅРµРёР·РІРµСЃС‚РЅРѕ";
    }
  }

  function toDatetimeLocal(value) {
    if (!value) {
      return "";
    }
    const date = new Date(value);
    const mskTime = new Date(date.getTime() + 3 * 60 * 60 * 1000);
    return mskTime.toISOString().slice(0, 16);
  }

  function fromDatetimeLocal(value) {
    if (!value) {
      return "";
    }
    const date = new Date(value + "Z");
    const utcTime = new Date(date.getTime() - 3 * 60 * 60 * 1000);
    return utcTime.toISOString();
  }

  function fillScheduleForm(tournament) {
    if (!tournamentScheduleForm || !tournament) {
      return;
    }
    tournamentScheduleForm.elements.startsAt.value = toDatetimeLocal(tournament.startsAt);
    tournamentScheduleForm.elements.qualificationEndsAt.value = toDatetimeLocal(tournament.qualificationEndsAt);
    tournamentScheduleForm.elements.playoffEndsAt.value = toDatetimeLocal(tournament.playoffEndsAt);
    tournamentScheduleForm.elements.endsAt.value = toDatetimeLocal(tournament.endsAt);
    tournamentScheduleForm.elements.playoffSlots.value = tournament.playoffSlots || 8;
    renderTournamentCalendar();
  }

  function loadAdminData() {
    loadApplications().catch(function (error) {
      setText(applicationsMessage, error.message);
    });
    loadParticipants().catch(function (error) {
      setText(whitelistMessage, error.message);
    });
    loadTournament().catch(function (error) {
      setText(tournamentStatus, error.message);
    });
    loadTestMode().catch(function (error) {
      setText(testModeStatus, error.message);
    });
    loadApplicationsStatus().catch(function (error) {
      if (applicationsCloseToggle) {
        applicationsCloseToggle.textContent = "РћС€РёР±РєР°";
      }
    });
  }

  if (adminIsDedicatedPage) {
    setPanelOpen(true);
  }

  if (adminToggle) {
    adminToggle.addEventListener("click", function () {
      setPanelOpen(adminPanel.hidden);
    });
  }

  if (adminClose) {
    adminClose.addEventListener("click", function () {
      setPanelOpen(false);
    });
  }

  if (applicationsToggle) {
    applicationsToggle.addEventListener("click", function () {
      setApplicationsOpen(applicationsPanel ? applicationsPanel.hidden : false);
    });
  }

  if (discordSyncRoles) {
    discordSyncRoles.addEventListener("click", syncDiscordRoles);
  }

  if (newsSync) {
    newsSync.addEventListener("click", syncNews);
  }

  if (testModeEnable) {
    testModeEnable.addEventListener("click", enableTestMode);
  }

  if (testModeDisable) {
    testModeDisable.addEventListener("click", disableTestMode);
  }

  if (testRunsClear) {
    testRunsClear.addEventListener("click", clearTestRuns);
  }

  if (applicationsCloseToggle) {
    applicationsCloseToggle.addEventListener("click", toggleApplicationsStatus);
  }

  if (adminApplicationsOpen) {
    adminApplicationsOpen.addEventListener("click", function () {
      setPanelOpen(true);
      setApplicationsOpen(true);
      adminPanel.scrollIntoView({ behavior: "smooth", block: "start" });
    });
  }

  if (whitelistForm) {
    whitelistForm.addEventListener("submit", async function (event) {
      event.preventDefault();
      const input = whitelistForm.elements.twitchLogin;
      const twitchLogin = input.value.trim().toLowerCase();
      if (!twitchLogin) {
        return;
      }

      setText(whitelistMessage, "Р”РѕР±Р°РІР»СЏСЋ...");
      try {
        await requestJSON("/api/admin/whitelist", {
          method: "POST",
          body: JSON.stringify({ twitchLogin: twitchLogin }),
        });
        input.value = "";
        setText(whitelistMessage, "РРіСЂРѕРє РґРѕР±Р°РІР»РµРЅ РІ whitelist.");
        await loadParticipants();
      } catch (error) {
        setText(whitelistMessage, error.message);
      }
    });
  }

  if (tournamentStart) {
    tournamentStart.addEventListener("click", async function () {
      setText(tournamentMessage, "Р—Р°РїСѓСЃРєР°СЋ...");
      try {
        await requestJSON("/api/admin/tournament", {
          method: "POST",
          body: JSON.stringify({ state: "running" }),
        });
        setText(tournamentMessage, "РўСѓСЂРЅРёСЂ Р·Р°РїСѓС‰РµРЅ. Р”Р°С‚С‹ СЂР°СЃСЃС‡РёС‚Р°РЅС‹ Р°РІС‚РѕРјР°С‚РёС‡РµСЃРєРё.");
        await loadTournament();
      } catch (error) {
        setText(tournamentMessage, error.message);
      }
    });
  }

  if (tournamentScheduleForm) {
    ["startsAt", "qualificationEndsAt", "playoffEndsAt", "endsAt"].forEach(function (fieldName) {
      tournamentScheduleForm.elements[fieldName].addEventListener("change", function () {
        renderTournamentCalendar();
      });
    });

    tournamentScheduleForm.addEventListener("submit", async function (event) {
      event.preventDefault();
      if (!currentTournament) {
        setText(tournamentMessage, "РЎРЅР°С‡Р°Р»Р° СЃРѕР·РґР°Р№С‚Рµ С‚СѓСЂРЅРёСЂ.");
        return;
      }
      if (!currentMatches.length) {
        setText(tournamentMessage, "РњР°С‚С‡Рё РїРѕСЏРІСЏС‚СЃСЏ РїРѕСЃР»Рµ РѕРєРѕРЅС‡Р°РЅРёСЏ РєРІР°Р»РёС„РёРєР°С†РёРё.");
        return;
      }
      setText(tournamentMessage, "РЎРѕС…СЂР°РЅСЏСЋ РєР°Р»РµРЅРґР°СЂСЊ...");
      try {
        await requestJSON("/api/admin/tournament/" + encodeURIComponent(currentTournament.id) + "/matches", {
          method: "PATCH",
          body: JSON.stringify({
            matches: currentMatches.map(function (match) {
              return {
                id: match.id,
                startsAt: match.startsAt,
                endsAt: match.endsAt,
                worldSeed: match.worldSeed || "",
              };
            }),
          }),
        });
        setText(tournamentMessage, "РљР°Р»РµРЅРґР°СЂСЊ РјР°С‚С‡РµР№ СЃРѕС…СЂР°РЅРµРЅ.");
        await loadTournament();
      } catch (error) {
        setText(tournamentMessage, error.message);
      }
    });
  }

  if (tournamentMatchSeedInput) {
    tournamentMatchSeedInput.addEventListener("input", function () {
      const match = selectedMatch();
      if (!match) {
        return;
      }
      const nextSeed = normalizeSeedInput(tournamentMatchSeedInput.value);
      if (nextSeed !== tournamentMatchSeedInput.value) {
        tournamentMatchSeedInput.value = nextSeed;
      }
      match.worldSeed = nextSeed;
      renderTournamentMatchList();
      setText(tournamentMessage, "Seed РјР°С‚С‡Р° РёР·РјРµРЅРµРЅ. РќР°Р¶РјРёС‚Рµ В«РЎРѕС…СЂР°РЅРёС‚СЊ РєР°Р»РµРЅРґР°СЂСЊВ».");
    });
  }

  function updateSelectedMatchTimes() {
    const match = selectedMatch();
    if (!match || !tournamentMatchDateInput || !tournamentMatchStartTimeInput || !tournamentMatchEndTimeInput) {
      return;
    }
    const dateVal = tournamentMatchDateInput.value;
    const startVal = tournamentMatchStartTimeInput.value;
    const endVal = tournamentMatchEndTimeInput.value;
    if (!dateVal || !startVal || !endVal) {
      return;
    }

    let endDateVal = dateVal;
    if (endVal < startVal) {
      const startDate = new Date(dateVal);
      const endDate = new Date(startDate.getTime() + 24 * 3600 * 1000);
      endDateVal = endDate.toISOString().slice(0, 10);
    }

    match.startsAt = new Date(dateVal + "T" + startVal + ":00+03:00").toISOString();
    match.endsAt = new Date(endDateVal + "T" + endVal + ":00+03:00").toISOString();
    renderTournamentCalendar();
    renderTournamentMatchList();
    setText(tournamentMessage, "РљР°Р»РµРЅРґР°СЂСЊ РёР·РјРµРЅРµРЅ. РќР°Р¶РјРёС‚Рµ В«РЎРѕС…СЂР°РЅРёС‚СЊ РєР°Р»РµРЅРґР°СЂСЊВ».");
  }

  if (tournamentMatchDateInput) {
    tournamentMatchDateInput.addEventListener("change", updateSelectedMatchTimes);
  }
  if (tournamentMatchStartTimeInput) {
    tournamentMatchStartTimeInput.addEventListener("change", updateSelectedMatchTimes);
  }
  if (tournamentMatchEndTimeInput) {
    tournamentMatchEndTimeInput.addEventListener("change", updateSelectedMatchTimes);
  }

  if (tournamentStop) {
    tournamentStop.addEventListener("click", async function () {
      setText(tournamentMessage, "РћСЃС‚Р°РЅР°РІР»РёРІР°СЋ...");
      try {
        await requestJSON("/api/admin/tournament/running", { method: "DELETE" });
        setText(tournamentMessage, "РўСѓСЂРЅРёСЂ РѕСЃС‚Р°РЅРѕРІР»РµРЅ.");
        await loadTournament();
      } catch (error) {
        setText(tournamentMessage, error.message);
      }
    });
  }

  if (leaderboardClear) {
    leaderboardClear.addEventListener("click", async function () {
      const confirmed = window.confirm("РћС‡РёСЃС‚РёС‚СЊ С‚Р°Р±Р»РёС†Сѓ Р»РёРґРµСЂРѕРІ? Р’СЃРµ Р·Р°Р±РµРіРё, СЃРїР»РёС‚С‹ Рё Р»СѓС‡С€РёРµ РІСЂРµРјРµРЅР° СѓС‡Р°СЃС‚РЅРёРєРѕРІ Р±СѓРґСѓС‚ СѓРґР°Р»РµРЅС‹.");
      if (!confirmed) {
        return;
      }

      setText(tournamentMessage, "РћС‡РёС‰Р°СЋ С‚Р°Р±Р»РёС†Сѓ...");
      try {
        await requestJSON("/api/admin/leaderboard", { method: "DELETE" });
        setText(tournamentMessage, "РўР°Р±Р»РёС†Р° РѕС‡РёС‰РµРЅР°.");
      } catch (error) {
        setText(tournamentMessage, error.message);
      }
    });
  }

  // РРЅРёС†РёР°Р»РёР·Р°С†РёСЏ С‚Р°Р±РѕРІ Р°РґРјРёРЅ-РїР°РЅРµР»Рё (СЃР°Р№РґР±Р°СЂ)
  const tabTriggers = document.querySelectorAll("[data-tab-trigger]");
  const tabContents = document.querySelectorAll("[data-tab-content]");
  if (tabTriggers.length > 0) {
    tabTriggers.forEach(function (trigger) {
      trigger.addEventListener("click", function () {
        const tabId = trigger.getAttribute("data-tab-trigger");
        tabTriggers.forEach(function (t) { t.classList.remove("is-active"); });
        tabContents.forEach(function (c) { c.classList.remove("is-active"); });
        trigger.classList.add("is-active");
        const activeContent = document.querySelector(`[data-tab-content="${tabId}"]`);
        if (activeContent) {
          activeContent.classList.add("is-active");
        }
      });
    });
  }

  function formatDurationJS(ms) {
    if (!ms || ms <= 0) return "РќРµС‚ СЂРµРєРѕСЂРґРѕРІ";
    const milliseconds = Math.floor(ms % 1000);
    const seconds = Math.floor((ms / 1000) % 60);
    const minutes = Math.floor((ms / (1000 * 60)) % 60);
    const hours = Math.floor(ms / (1000 * 60 * 60));

    const pad = (n, width = 2) => String(n).padStart(width, '0');

    if (hours > 0) {
      return `${hours}:${pad(minutes)}:${pad(seconds)}.${pad(milliseconds, 3)}`;
    }
    return `${pad(minutes)}:${pad(seconds)}.${pad(milliseconds, 3)}`;
  }

  function renderRunsTabParticipants(participants) {
    const grid = document.getElementById("tab-players-grid");
    if (!grid) return;
    grid.innerHTML = "";

    if (!participants || participants.length === 0) {
      grid.innerHTML = `<div class="empty-state"><strong>РќРµС‚ Р·Р°СЂРµРіРёСЃС‚СЂРёСЂРѕРІР°РЅРЅС‹С… СѓС‡Р°СЃС‚РЅРёРєРѕРІ</strong><span>РЎРїРёСЃРѕРє whitelist РїСѓСЃС‚.</span></div>`;
      return;
    }

    participants.forEach(function (participant) {
      if (participant.status === "blocked") return;

      const login = participant.twitchLogin || "";
      const displayName = participant.twitchDisplayName || login;
      const mcNick = participant.minecraftNick || "вЂ”";
      const avatar = participant.avatarURL || "/static/avatar-placeholder.svg";
      const bestTime = formatDurationJS(participant.bestTimeMS);
      const participantID = participant.id || participant.ID || "";

      const card = document.createElement("a");
      card.className = "admin-player-card";
      card.href = "/admin/runs/" + participantID;
      card.setAttribute("data-search-term", (login + " " + displayName + " " + mcNick).toLowerCase());
      card.innerHTML = `
        <div class="admin-player-avatar-col">
          <img class="admin-player-avatar" src="${avatar}" alt="" />
        </div>
        <div class="admin-player-info-col">
          <strong class="admin-player-name">${displayName}</strong>
          <span class="admin-player-nick">MC: ${mcNick}</span>
        </div>
        <div class="admin-player-time-col">
          <span class="time-label">Р›СѓС‡С€РµРµ РІСЂРµРјСЏ</span>
          <strong class="time-value">${bestTime}</strong>
        </div>
      `;
      grid.appendChild(card);
    });

    const searchInput = document.getElementById("admin-runs-tab-search");
    const emptyState = document.getElementById("tab-search-empty-state");
    if (searchInput) {
      const newSearchInput = searchInput.cloneNode(true);
      searchInput.parentNode.replaceChild(newSearchInput, searchInput);
      newSearchInput.addEventListener("input", function () {
        const query = newSearchInput.value.toLowerCase().trim();
        const cards = grid.querySelectorAll(".admin-player-card");
        let visibleCount = 0;
        cards.forEach(function (card) {
          const text = card.getAttribute("data-search-term") || "";
          if (text.includes(query)) {
            card.style.display = "";
            visibleCount++;
          } else {
            card.style.display = "none";
          }
        });
        if (emptyState) {
          emptyState.hidden = visibleCount > 0;
        }
      });
    }
  }
});
