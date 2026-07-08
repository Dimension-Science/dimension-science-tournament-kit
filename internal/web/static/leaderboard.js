document.addEventListener("DOMContentLoaded", function () {
  const intro = document.querySelector("[data-site-intro]");
  const introCanvas = document.querySelector("[data-intro-canvas]");
  const buttons = Array.from(document.querySelectorAll("[data-leaderboard-toggle]"));
  const detailRows = Array.from(document.querySelectorAll(".leaderboard-detail-row"));
  const searchInput = document.querySelector("[data-leaderboard-search]");
  const leaderboardRows = Array.from(document.querySelectorAll("[data-leaderboard-row]"));
  const emptyState = document.querySelector("[data-leaderboard-empty]");
  const splash = document.querySelector("[data-minecraft-splash]");
  const achievementFeed = document.querySelector("[data-achievement-feed]");

  function setExpanded(targetId, expanded) {
    buttons.forEach(function (button) {
      if (button.getAttribute("data-leaderboard-toggle") === targetId) {
        button.setAttribute("aria-expanded", expanded ? "true" : "false");
      } else {
        button.setAttribute("aria-expanded", "false");
      }
    });
    leaderboardRows.forEach(function (row) {
      if (row.getAttribute("data-leaderboard-target") === targetId) {
        row.setAttribute("aria-expanded", expanded ? "true" : "false");
      } else {
        row.setAttribute("aria-expanded", "false");
      }
    });
  }

  function toggleDetail(detailId) {
    const detailRow = document.getElementById(detailId);
    if (!detailRow) {
      return;
    }

    const shouldOpen = detailRow.hasAttribute("hidden");
    detailRows.forEach(function (row) {
      row.setAttribute("hidden", "hidden");
    });

    if (shouldOpen) {
      detailRow.removeAttribute("hidden");
    }
    setExpanded(detailId, shouldOpen);
  }

  buttons.forEach(function (button) {
    button.addEventListener("click", function (event) {
      event.stopPropagation();
      toggleDetail(button.getAttribute("data-leaderboard-toggle"));
    });
  });

  leaderboardRows.forEach(function (row) {
    row.addEventListener("click", function (event) {
      if (event.target.closest("a, button, input, label, select, textarea")) {
        return;
      }
      toggleDetail(row.getAttribute("data-leaderboard-target"));
    });
    row.addEventListener("keydown", function (event) {
      if (event.key !== "Enter" && event.key !== " ") {
        return;
      }
      event.preventDefault();
      toggleDetail(row.getAttribute("data-leaderboard-target"));
    });
  });

  if (searchInput) {
    searchInput.addEventListener("input", function () {
      const query = searchInput.value.trim().toLowerCase();
      let visibleCount = 0;

      detailRows.forEach(function (row) {
        row.setAttribute("hidden", "hidden");
      });
      setExpanded("", false);

      leaderboardRows.forEach(function (row) {
        const haystack = (row.getAttribute("data-player-search") || "").toLowerCase();
        const isVisible = query === "" || haystack.includes(query);
        row.hidden = !isVisible;
        if (isVisible) {
          visibleCount += 1;
        }
      });

      if (emptyState) {
        emptyState.hidden = visibleCount > 0;
      }
    });
  }

  if (intro && introCanvas) {
    setupIntro(intro, introCanvas);
  }

  if (splash) {
    setupSplash(splash);
  }

  if (achievementFeed) {
    setupAchievementFeed(achievementFeed);
  }
});

function setupAchievementFeed(root) {
  const pollIntervalMs = 10000;
  const toastLifetimeMs = 5600;
  const seen = new Set();
  let cursor = "";
  let initialized = false;

  function newestUnlockedAt(items) {
    let newest = "";
    items.forEach(function (item) {
      if (!item || !item.unlockedAt) {
        return;
      }
      if (!newest || new Date(item.unlockedAt).getTime() > new Date(newest).getTime()) {
        newest = item.unlockedAt;
      }
    });
    return newest;
  }

  function remember(items) {
    items.forEach(function (item) {
      if (item && item.id) {
        seen.add(item.id);
      }
    });
  }

  function updateCursor(items) {
    const newest = newestUnlockedAt(items);
    if (newest) {
      cursor = newest;
    }
  }

  function scheduleNextPoll() {
    window.setTimeout(poll, pollIntervalMs);
  }

  async function poll() {
    try {
      const url = cursor
        ? "/api/achievements/feed?since=" + encodeURIComponent(cursor)
        : "/api/achievements/feed";
      const response = await fetch(url, {
        cache: "no-store",
        headers: { "Accept": "application/json" },
      });
      if (!response.ok) {
        throw new Error("achievement feed failed");
      }
      const payload = await response.json();
      const items = Array.isArray(payload.items) ? payload.items : [];

      if (!initialized) {
        initialized = true;
        remember(items);
        cursor = payload.serverTime || newestUnlockedAt(items) || new Date().toISOString();
        return;
      }

      const freshItems = items
        .filter(function (item) {
          return item && item.id && !seen.has(item.id);
        })
        .sort(function (left, right) {
          return new Date(left.unlockedAt).getTime() - new Date(right.unlockedAt).getTime();
        });

      remember(items);
      updateCursor(items);
      freshItems.forEach(function (item) {
        showAchievementFeedToast(root, item, toastLifetimeMs);
      });
    } catch (error) {
      // The feed is decorative; a temporary network error should stay quiet.
    } finally {
      scheduleNextPoll();
    }
  }

  poll();
}

function showAchievementFeedToast(root, item, lifetimeMs) {
  const toast = document.createElement("article");
  toast.className = "achievement-feed-toast";
  toast.setAttribute("role", "status");

  const icon = document.createElement("span");
  icon.className = "achievement-feed-icon";
  if (item.achievementSlug) {
    icon.classList.add("achievement-icon-" + item.achievementSlug);
  }
  icon.setAttribute("aria-hidden", "true");

  const copy = document.createElement("span");
  copy.className = "achievement-feed-copy";

  const name = document.createElement("strong");
  name.textContent = item.displayName || item.twitchLogin || "Runner";

  const action = document.createElement("span");
  action.textContent = "получает достижение";

  const achievement = document.createElement("em");
  achievement.textContent = item.achievementName || "Достижение";

  copy.append(name, action, achievement);
  toast.append(icon, copy);
  root.prepend(toast);

  while (root.children.length > 4) {
    root.lastElementChild.remove();
  }

  window.setTimeout(function () {
    toast.remove();
  }, lifetimeMs);
}

function setupSplash(splash) {
  const phrases = [
    "Кто сегодня украдет первое место?",
    "Дракон снова не в безопасности.",
    "Таблица ждет новый рекорд.",
    "Минус секунды, плюс легенда.",
    "Незер уже прогрет.",
  ];
  const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  function setPhrase(phrase, animate) {
    if (splash.textContent === phrase) {
      return;
    }
    splash.textContent = phrase;
    if (reduceMotion || !animate) {
      return;
    }
    splash.classList.remove("is-popping");
    window.requestAnimationFrame(function () {
      splash.classList.add("is-popping");
    });
  }

  function refreshDailyPhrase(animate) {
    const phrase = phrases[moscowDayNumber(new Date()) % phrases.length];
    setPhrase(phrase, animate);

    window.setTimeout(function () {
      refreshDailyPhrase(true);
    }, msUntilNextMoscowMidnight());
  }

  refreshDailyPhrase(false);
}

function moscowDateParts(date) {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: "Europe/Moscow",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(date);
  return {
    year: Number(parts.find(function (part) { return part.type === "year"; }).value),
    month: Number(parts.find(function (part) { return part.type === "month"; }).value),
    day: Number(parts.find(function (part) { return part.type === "day"; }).value),
  };
}

function moscowDayNumber(date) {
  const parts = moscowDateParts(date);
  return Math.floor(Date.UTC(parts.year, parts.month - 1, parts.day) / 86400000);
}

function msUntilNextMoscowMidnight() {
  const now = new Date();
  const parts = moscowDateParts(now);
  const nextMidnightUTC = Date.UTC(parts.year, parts.month - 1, parts.day + 1, -3, 0, 0, 0);
  return Math.max(1000, nextMidnightUTC - now.getTime() + 250);
}

function setupIntro(intro, canvas) {
  const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const ctx = canvas.getContext("2d");
  let started = false;
  let frame = 0;

  function resize() {
    const scale = window.devicePixelRatio || 1;
    canvas.width = Math.max(1, Math.floor(window.innerWidth * scale));
    canvas.height = Math.max(1, Math.floor(window.innerHeight * scale));
    canvas.style.width = window.innerWidth + "px";
    canvas.style.height = window.innerHeight + "px";
    ctx.setTransform(scale, 0, 0, scale, 0, 0);
  }

  function cellNoise(x, y) {
    const value = Math.sin(x * 12.9898 + y * 78.233) * 43758.5453;
    return value - Math.floor(value);
  }

  function smoothstep(edge0, edge1, value) {
    const x = Math.min(1, Math.max(0, (value - edge0) / (edge1 - edge0)));
    return x * x * (3 - 2 * x);
  }

  function distanceToSegment(px, py, ax, ay, bx, by) {
    const vx = bx - ax;
    const vy = by - ay;
    const wx = px - ax;
    const wy = py - ay;
    const lengthSq = vx * vx + vy * vy;
    const t = lengthSq === 0 ? 0 : Math.min(1, Math.max(0, (wx * vx + wy * vy) / lengthSq));
    const dx = px - (ax + t * vx);
    const dy = py - (ay + t * vy);
    return Math.sqrt(dx * dx + dy * dy);
  }

  function distanceToLines(x, y, lines) {
    let best = 1;
    lines.forEach(function (line) {
      for (let i = 0; i < line.length - 1; i += 1) {
        const current = line[i];
        const next = line[i + 1];
        best = Math.min(best, distanceToSegment(x, y, current[0], current[1], next[0], next[1]));
      }
    });
    return best;
  }

  function drawPixelBurst(progress) {
    const tile = Math.max(8, Math.min(15, Math.floor(window.innerWidth / 132)));
    const cols = Math.ceil(window.innerWidth / tile) + 2;
    const rows = Math.ceil(window.innerHeight / tile) + 2;
    const lineSets = [
      [[0.02, -0.02], [0.08, 0.02], [0.12, 0.15], [0.14, 0.29], [0.2, 0.42], [0.3, 0.55], [0.42, 0.7], [0.55, 1.04]],
      [[0.2, -0.04], [0.22, 0.1], [0.29, 0.19], [0.38, 0.21], [0.43, 0.16], [0.42, 0.04], [0.46, -0.03]],
      [[0.58, -0.03], [0.56, 0.08], [0.6, 0.17], [0.68, 0.24], [0.78, 0.24], [0.84, 0.18], [0.88, 0.06], [0.96, -0.03]],
      [[0.88, -0.02], [0.82, 0.08], [0.8, 0.21], [0.83, 0.35], [0.91, 0.42], [0.93, 0.55], [0.88, 0.65], [0.78, 0.72], [0.76, 0.86], [0.8, 1.04]],
      [[1.04, 0.38], [0.92, 0.36], [0.86, 0.43], [0.9, 0.53], [0.95, 0.61], [0.91, 0.7], [0.84, 0.74]],
      [[-0.04, 0.94], [0.07, 0.88], [0.18, 0.86], [0.28, 0.9], [0.39, 0.98], [0.5, 1.04]],
    ];
    const layers = [
      { color: "#32a9df", edge: 0.035 },
      { color: "#ffc62b", edge: 0.07 },
      { color: "#9b53d5", edge: 0.105 },
      { color: "#59d793", edge: 0.14 },
    ];
    const maxErode = 0.38;
    const erode = smoothstep(0, 1, progress) * maxErode;

    ctx.clearRect(0, 0, window.innerWidth, window.innerHeight);

    if (progress >= 0.995) {
      return;
    }

    for (let y = -1; y < rows; y += 1) {
      for (let x = -1; x < cols; x += 1) {
        const noise = cellNoise(x, y);
        const nx = (x + 0.5) / cols;
        const ny = (y + 0.5) / rows;
        const distance = distanceToLines(nx, ny, lineSets);
        const ragged = (noise - 0.5) * 0.018 + (cellNoise(x - 19, y + 37) - 0.5) * 0.01;
        const depth = erode - distance + ragged;

        if (depth <= 0) {
          ctx.globalAlpha = 1;
          ctx.fillStyle = "#2a2a2a";
          ctx.fillRect(x * tile, y * tile, tile + 1, tile + 1);
          continue;
        }

        if (depth > layers[layers.length - 1].edge) {
          continue;
        }

        const layerIndex = layers.findIndex(function (layer) {
          return depth <= layer.edge;
        });
        const layer = layers[Math.max(0, layerIndex)];
        const previousEdge = layerIndex === 0 ? 0 : layers[layerIndex - 1].edge;
        const withinLayer = (depth - previousEdge) / (layer.edge - previousEdge);
        ctx.globalAlpha = 0.45 + smoothstep(0, 0.22, withinLayer) * 0.55;
        ctx.fillStyle = layer.color;
        ctx.fillRect(x * tile, y * tile, tile + 1, tile + 1);
      }
    }

    ctx.globalAlpha = 1;
  }

  function finishIntro() {
    document.body.classList.remove("intro-active");
    intro.classList.add("is-hidden");
    window.setTimeout(function () {
      intro.remove();
    }, 560);
  }

  function animate() {
    frame += 1;
    const progress = Math.min(1, frame / 165);
    drawPixelBurst(progress);

    if (progress < 1) {
      window.requestAnimationFrame(animate);
      return;
    }

    window.setTimeout(finishIntro, 120);
  }

  function startIntro() {
    if (started) {
      return;
    }
    started = true;
    intro.classList.add("is-animating");
    window.setTimeout(function () {
      intro.classList.add("is-revealing");
      document.body.classList.remove("intro-active");
    }, 220);

    if (reduceMotion) {
      finishIntro();
      return;
    }

    resize();
    animate();
  }

  resize();
  window.addEventListener("resize", resize);
  intro.addEventListener("click", startIntro);
  intro.addEventListener("keydown", function (event) {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      startIntro();
    }
  });
}
