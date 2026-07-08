(function () {
  const storageKey = "tournament_telegram_promo_seen";
  const telegramUrl = window.TOURNAMENT_PROFILE?.telegramUrl || "";

  function wasShown() {
    try {
      return window.sessionStorage.getItem(storageKey) === "1";
    } catch (_) {
      return false;
    }
  }

  function markShown() {
    try {
      window.sessionStorage.setItem(storageKey, "1");
    } catch (_) {
      // Session storage may be disabled; the popup can still close normally.
    }
  }

  function hide(card) {
    card.classList.remove("is-visible");
    card.classList.add("is-hiding");
    window.setTimeout(function () {
      card.remove();
    }, 260);
  }

  function createPromo() {
    const card = document.createElement("aside");
    card.className = "telegram-promo-toast";
    card.setAttribute("aria-label", "РџРѕРґРїРёСЃРєР° РЅР° Telegram");
    card.setAttribute("role", "status");
    card.innerHTML = [
      '<button class="telegram-promo-close" type="button" aria-label="Р—Р°РєСЂС‹С‚СЊ">Г—</button>',
      '<span class="telegram-promo-icon" aria-hidden="true">',
      '<svg viewBox="0 0 24 24" focusable="false">',
      '<path d="M21.7 4.2 18.4 20c-.2.9-.8 1.1-1.6.7l-4.9-3.6-2.4 2.3c-.3.3-.5.5-1 .5l.4-5 9-8.1c.4-.4-.1-.6-.6-.2L6.2 13.5 1.4 12c-.9-.3-.9-1 0-1.3L20.1 3.5c.8-.3 1.7.2 1.6.7Z" />',
      "</svg>",
      "</span>",
      '<span class="telegram-promo-copy">',
      "<strong>РџРѕРґРїРёСЃР°С‚СЊСЃСЏ РЅР° Telegram</strong>",
      "<span>РќРѕРІРѕСЃС‚Рё С‚СѓСЂРЅРёСЂР°, Р·Р°СЏРІРєРё Рё РІР°Р¶РЅС‹Рµ РѕР±СЉСЏРІР»РµРЅРёСЏ.</span>",
      "</span>",
      '<a class="button telegram-promo-action" href="' + telegramUrl + '" target="_blank" rel="noopener noreferrer">РџРµСЂРµР№С‚Рё</a>',
    ].join("");
    return card;
  }

  function showPromoOnce(state) {
    if (!telegramUrl) {
      return;
    }
    if (state.shown || wasShown() || document.body.classList.contains("intro-active")) {
      return;
    }
    state.shown = true;

    const card = createPromo();
    document.body.appendChild(card);

    const closeButton = card.querySelector(".telegram-promo-close");
    const action = card.querySelector(".telegram-promo-action");
    const close = function () {
      markShown();
      hide(card);
    };

    closeButton.addEventListener("click", close);
    action.addEventListener("click", markShown);

    window.requestAnimationFrame(function () {
      card.classList.add("is-visible");
    });
    window.setTimeout(close, 10000);
  }

  document.addEventListener("DOMContentLoaded", function () {
    const state = { shown: false };

    if (!document.body.classList.contains("intro-active")) {
      showPromoOnce(state);
      return;
    }

    const wait = window.setInterval(function () {
      if (!document.body.classList.contains("intro-active")) {
        window.clearInterval(wait);
        window.setTimeout(function () {
          showPromoOnce(state);
        }, 600);
      }
    }, 250);
  });
})();
