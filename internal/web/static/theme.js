(function () {
  const key = "tournament_theme";
  const themes = ["nether", "quartz"];

  function readCookie(name) {
    return document.cookie
      .split(";")
      .map((part) => part.trim())
      .find((part) => part.startsWith(name + "="))
      ?.slice(name.length + 1);
  }

  function normalizeTheme(value) {
    return themes.includes(value) ? value : "nether";
  }

  function savedTheme() {
    try {
      const cookieTheme = readCookie(key);
      if (cookieTheme) {
        return normalizeTheme(decodeURIComponent(cookieTheme));
      }
      return normalizeTheme(window.localStorage.getItem(key));
    } catch (_) {
      return "nether";
    }
  }

  function sharedCookieDomain() {
    const host = window.location.hostname;
    return "";
  }

  function persistTheme(theme) {
    try {
      window.localStorage.setItem(key, theme);
    } catch (_) {
      // Local storage can be disabled; the cookie below is enough for normal browsers.
    }

    const secure = window.location.protocol === "https:" ? "; secure" : "";
    document.cookie = key + "=" + encodeURIComponent(theme)
      + "; max-age=31536000; path=/; samesite=lax"
      + sharedCookieDomain()
      + secure;
  }

  function applyTheme(theme) {
    const nextTheme = normalizeTheme(theme);
    document.documentElement.setAttribute("data-theme", nextTheme);
    document.querySelectorAll("[data-theme-toggle]").forEach((button) => {
      button.setAttribute("aria-pressed", nextTheme === "quartz" ? "true" : "false");
      button.setAttribute("data-current-theme", nextTheme);
    });
  }

  applyTheme(savedTheme());

  document.addEventListener("DOMContentLoaded", function () {
    applyTheme(savedTheme());
    document.querySelectorAll("[data-theme-toggle]").forEach((button) => {
      button.addEventListener("click", function () {
        const current = normalizeTheme(document.documentElement.getAttribute("data-theme"));
        const next = current === "nether" ? "quartz" : "nether";
        applyTheme(next);
        persistTheme(next);
      });
    });

    const pushToast = document.getElementById("site-push-toast");
    if (pushToast) {
      const title = pushToast.getAttribute("data-push-title") || "";
      const text = pushToast.getAttribute("data-push-text") || "";
      const hash = btoa(encodeURIComponent(title + "|" + text));
      const storageKey = "tournament_push_dismissed_" + hash;
      
      if (localStorage.getItem(storageKey)) {
        pushToast.style.display = "none";
      } else {
        const closeBtn = pushToast.querySelector("[data-push-close]");
        if (closeBtn) {
          closeBtn.addEventListener("click", function () {
            localStorage.setItem(storageKey, "true");
            pushToast.classList.add("fade-out");
            setTimeout(function () {
              pushToast.remove();
            }, 300);
          });
        }
      }
    }
  });
})();
