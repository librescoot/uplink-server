// Entry point: wire the UI and connect.

import { getApiKey, apiRequest } from "./api.js";
import { store } from "./store.js";
import { showStatus } from "./format.js";
import { connectWebSocket } from "./ws.js";
import { sendCommand } from "./commands.js";
import { openHistory, reloadHistory } from "./history.js";
import { openScootersDialog, addScooter, deleteScooter, copyToken } from "./registry.js";
import { dismissEvent, clearAllEvents } from "./events.js";
import { openAuthDialog, setAuthError, doLogin, saveKey, clearKey } from "./auth.js";

function refreshAll() {
  store.scooters.forEach((s) => sendCommand(s.identifier, "get_state"));
}

async function verifyAndConnect() {
  try {
    await apiRequest("/api/registry"); // cheap authenticated probe
    setAuthError(false);
    connectWebSocket();
  } catch (e) {
    if (e.status === 401) {
      setAuthError(true);
      showStatus("apiKeyStatus", "Invalid credentials. Please re-authenticate.", "error");
      openAuthDialog();
    } else {
      connectWebSocket();
    }
  }
}

function wire() {
  // Header buttons.
  document.getElementById("scootersBtn").addEventListener("click", openScootersDialog);
  document.getElementById("apiKeyBtn").addEventListener("click", openAuthDialog);
  document.getElementById("refreshBtn").addEventListener("click", refreshAll);

  // Auth dialog.
  document.getElementById("loginBtn").addEventListener("click", doLogin);
  document.getElementById("loginPass").addEventListener("keydown", (e) => e.key === "Enter" && doLogin());
  document.getElementById("saveKeyBtn").addEventListener("click", saveKey);
  document.getElementById("clearKeyBtn").addEventListener("click", clearKey);

  // Manage-scooters + history dialogs.
  document.getElementById("addScooterBtn").addEventListener("click", addScooter);
  document.getElementById("historyReloadBtn").addEventListener("click", reloadHistory);

  // Dialog close buttons and overlay click-to-close.
  document.querySelectorAll("[data-close]").forEach((b) =>
    b.addEventListener("click", () => document.getElementById(b.dataset.close).classList.remove("show"))
  );
  ["apiKeyDialog", "scootersDialog", "historyDialog"].forEach((id) => {
    const o = document.getElementById(id);
    o.addEventListener("click", (e) => {
      if (e.target === o) o.classList.remove("show");
    });
  });

  // Delegated actions for dynamically-rendered elements.
  document.addEventListener("click", (e) => {
    const btn = e.target.closest("[data-cmd],[data-action]");
    if (!btn) return;
    const scooter = btn.dataset.scooter;
    if (btn.dataset.cmd) {
      if (btn.dataset.confirm && !confirm(btn.dataset.confirm)) return;
      let params = {};
      if (btn.dataset.params) {
        try {
          params = JSON.parse(btn.dataset.params);
        } catch (_) {}
      }
      sendCommand(scooter, btn.dataset.cmd, params);
      return;
    }
    switch (btn.dataset.action) {
      case "history":
        openHistory(scooter);
        break;
      case "dismiss-event":
        dismissEvent(scooter, btn.dataset.event);
        break;
      case "clear-events":
        clearAllEvents(scooter);
        break;
      case "delete-scooter":
        deleteScooter(scooter);
        break;
      case "copy-token":
        copyToken(btn);
        break;
    }
  });
}

document.addEventListener("DOMContentLoaded", () => {
  wire();
  if (getApiKey()) {
    document.getElementById("apiKeyInput").value = getApiKey();
    verifyAndConnect();
  } else {
    document.getElementById("scootersContainer").innerHTML =
      '<div class="empty-state">Log in or enter an API key to view scooters.</div>';
    openAuthDialog();
  }
});
