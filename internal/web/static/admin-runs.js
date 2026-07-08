document.addEventListener("DOMContentLoaded", function () {
  // 1. Search filter logic
  const searchInput = document.getElementById("admin-runs-search");
  const playersGrid = document.getElementById("players-grid");
  const searchEmptyState = document.getElementById("search-empty-state");

  if (searchInput && playersGrid) {
    const cards = Array.from(playersGrid.querySelectorAll(".admin-player-card"));

    searchInput.addEventListener("input", function () {
      const query = searchInput.value.trim().toLowerCase();
      let visibleCount = 0;

      cards.forEach(function (card) {
        const haystack = (card.getAttribute("data-player-search") || "").toLowerCase();
        const matches = query === "" || haystack.includes(query);
        card.hidden = !matches;
        if (matches) {
          visibleCount++;
        }
      });

      if (searchEmptyState) {
        searchEmptyState.hidden = visibleCount > 0;
      }
    });
  }

  // 2. Reject / Approve AJAX logic
  const runsTable = document.querySelector(".admin-runs-table");
  if (runsTable) {
    runsTable.addEventListener("click", async function (event) {
      const target = event.target;
      if (!target || !target.classList.contains("button-action")) {
        return;
      }

      const action = target.getAttribute("data-action-btn");
      const row = target.closest(".admin-run-row");
      if (!row) {
        return;
      }

      const runID = row.getAttribute("data-run-id");
      if (!runID) {
        return;
      }

      const originalText = target.textContent;
      target.disabled = true;
      target.textContent = "...";

      try {
        const url = `/api/admin/runs/${encodeURIComponent(runID)}/${action === "reject" ? "reject" : "approve"}`;
        const response = await fetch(url, {
          method: "POST",
          headers: {
            "Accept": "application/json",
            "Content-Type": "application/json"
          }
        });

        if (!response.ok) {
          const errData = await response.json().catch(() => ({}));
          throw new Error(errData.error || "Ошибка при выполнении запроса");
        }

        const data = await response.json();
        
        // Update row class
        row.className = `admin-run-row is-status-${data.status}`;

        // Update status badge
        const badge = row.querySelector(".run-status-badge");
        if (badge) {
          badge.className = `run-status-badge badge-${data.status}`;
          badge.textContent = data.status === "approved" ? "Одобрен" : "Отменен";
        }

        // Toggle buttons
        const rejectBtn = row.querySelector('[data-action-btn="reject"]');
        const approveBtn = row.querySelector('[data-action-btn="approve"]');
        if (rejectBtn && approveBtn) {
          if (data.status === "approved") {
            rejectBtn.style.display = "";
            approveBtn.style.display = "none";
          } else {
            rejectBtn.style.display = "none";
            approveBtn.style.display = "";
          }
        }

        // Update personal best time
        const pbDisplay = document.getElementById("player-pb");
        if (pbDisplay) {
          pbDisplay.textContent = data.pb || "Нет";
        }

      } catch (error) {
        window.alert(error.message);
      } finally {
        target.disabled = false;
        target.textContent = originalText;
      }
    });
  }

  // 3. Add Run Form AJAX logic
  const addRunForm = document.getElementById("admin-add-run-form");
  const addRunMessage = document.getElementById("admin-add-run-message");
  if (addRunForm) {
    addRunForm.addEventListener("submit", async function (event) {
      event.preventDefault();
      
      const submitBtn = addRunForm.querySelector('button[type="submit"]');
      if (submitBtn) {
        submitBtn.disabled = true;
      }
      if (addRunMessage) {
        addRunMessage.hidden = true;
        addRunMessage.textContent = "";
      }

      try {
        const formData = new FormData(addRunForm);
        const payload = {
          time: formData.get("time"),
          netherSplit: formData.get("netherSplit"),
          endSplit: formData.get("endSplit"),
          phase: formData.get("phase"),
          matchId: formData.get("matchId")
        };

        const response = await fetch(addRunForm.action, {
          method: "POST",
          headers: {
            "Accept": "application/json",
            "Content-Type": "application/json"
          },
          body: JSON.stringify(payload)
        });

        if (!response.ok) {
          const errData = await response.json().catch(() => ({}));
          throw new Error(errData.error || "Ошибка при добавлении забега");
        }

        const data = await response.json();
        window.location.reload();
      } catch (error) {
        if (addRunMessage) {
          addRunMessage.textContent = error.message;
          addRunMessage.hidden = false;
        } else {
          window.alert(error.message);
        }
      } finally {
        if (submitBtn) {
          submitBtn.disabled = false;
        }
      }
    });
  }
});
