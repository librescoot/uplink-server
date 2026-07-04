// Authentication dialog: username/password login and API-key entry.

import { setApiKey, clearApiKey, login } from "./api.js";
import { showStatus } from "./format.js";
import { connectWebSocket, closeWebSocket } from "./ws.js";

export function openAuthDialog() {
  document.getElementById("apiKeyDialog").classList.add("show");
}

export function closeAuthDialog() {
  document.getElementById("apiKeyDialog").classList.remove("show");
}

export function setAuthError(on) {
  document.getElementById("apiKeyBtn").classList.toggle("error", !!on);
}

export async function doLogin() {
  const u = document.getElementById("loginUser").value.trim();
  const p = document.getElementById("loginPass").value;
  if (!u || !p) {
    showStatus("apiKeyStatus", "Enter username and password", "error");
    return;
  }
  try {
    const data = await login(u, p);
    document.getElementById("loginPass").value = "";
    showStatus("apiKeyStatus", `Logged in as ${data.username}`, "success");
    setAuthError(false);
    setTimeout(() => {
      closeAuthDialog();
      connectWebSocket();
    }, 400);
  } catch (e) {
    showStatus("apiKeyStatus", e.message, "error");
  }
}

export function saveKey() {
  const v = document.getElementById("apiKeyInput").value.trim();
  if (!v) {
    showStatus("apiKeyStatus", "Enter an API key", "error");
    return;
  }
  setApiKey(v);
  showStatus("apiKeyStatus", "API key saved", "success");
  setAuthError(false);
  setTimeout(() => {
    closeAuthDialog();
    connectWebSocket();
  }, 400);
}

export function clearKey() {
  clearApiKey();
  closeWebSocket();
  document.getElementById("apiKeyInput").value = "";
  showStatus("apiKeyStatus", "Credentials cleared", "info");
  document.getElementById("scootersContainer").innerHTML =
    '<div class="empty-state">No credentials configured. Log in or enter an API key.</div>';
}
