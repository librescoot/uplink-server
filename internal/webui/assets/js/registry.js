// Manage-scooters dialog: list, add (issue token), delete.

import { apiRequest } from "./api.js";
import { escapeHtml, showStatus } from "./format.js";

export function openScootersDialog() {
  document.getElementById("newScooterResult").classList.add("hidden");
  document.getElementById("scootersDialog").classList.add("show");
  loadRegistry();
}

export async function loadRegistry() {
  const el = document.getElementById("registryList");
  try {
    const data = await apiRequest("/api/registry");
    renderRegistry(data.scooters || []);
  } catch (e) {
    el.innerHTML = `<p class="status error">${escapeHtml(e.message)}</p>`;
  }
}

function renderRegistry(list) {
  const el = document.getElementById("registryList");
  if (!list.length) {
    el.innerHTML = '<p class="muted">No scooters registered yet.</p>';
    return;
  }
  el.innerHTML = list
    .map(
      (s) => `<div class="registry-row">
      <span>
        <span class="rid">${escapeHtml(s.identifier)}</span>
        ${s.name ? `<span class="rmeta">${escapeHtml(s.name)}</span>` : ""}
        <span class="rmeta">${s.connected ? "● online" : "○ offline"}</span>
      </span>
      <button class="cmd-btn" data-action="delete-scooter" data-scooter="${escapeHtml(s.identifier)}">Remove</button>
    </div>`
    )
    .join("");
}

export async function addScooter() {
  const id = document.getElementById("newScooterId").value.trim();
  const name = document.getElementById("newScooterName").value.trim();
  if (!id) {
    showStatus("addScooterStatus", "Identifier is required", "error");
    return;
  }
  try {
    const res = await apiRequest("/api/scooters", {
      method: "POST",
      body: JSON.stringify({ identifier: id, name }),
    });
    document.getElementById("newScooterId").value = "";
    document.getElementById("newScooterName").value = "";
    showStatus("addScooterStatus", "Scooter added", "success");
    showNewScooterResult(res);
    loadRegistry();
  } catch (e) {
    showStatus("addScooterStatus", e.message, "error");
  }
}

function showNewScooterResult(res) {
  const cfg = `uplink:\n  server_url: "ws://${window.location.host}/ws"\nscooter:\n  identifier: "${res.identifier}"\n  token: "${res.token}"`;
  const el = document.getElementById("newScooterResult");
  el.classList.remove("hidden");
  el.innerHTML = `
    <p class="section-label" style="margin-top:10px">Client config — token shown once, copy it now</p>
    <div class="code-block" id="newTokenBlock">${escapeHtml(cfg)}</div>
    <button class="cmd-btn" data-action="copy-token">Copy client config</button>`;
}

export async function deleteScooter(id) {
  if (!confirm(`Remove scooter ${id}? Its token will stop working.`)) return;
  try {
    await apiRequest(`/api/scooters/${encodeURIComponent(id)}`, { method: "DELETE" });
    loadRegistry();
  } catch (e) {
    showStatus("addScooterStatus", e.message, "error");
  }
}

export function copyToken(btn) {
  const block = document.getElementById("newTokenBlock");
  if (!block || !navigator.clipboard) return;
  navigator.clipboard.writeText(block.textContent).then(() => {
    const t = btn.textContent;
    btn.textContent = "Copied!";
    setTimeout(() => (btn.textContent = t), 1200);
  });
}
