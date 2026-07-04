// Scooter card rendering, reconciliation, live online/offline + stats, and
// applying full/delta state updates.

import { store } from "./store.js";
import { escapeHtml, formatDuration, formatBytes } from "./format.js";
import { renderCommandsHTML } from "./commands.js";
import { renderBadge, renderState } from "./state.js";

function cardHTML(s) {
  const id = s.identifier;
  return `<div class="scooter-card offline" id="scooter-${escapeHtml(id)}">
    <div class="card-head">
      <div class="card-title">
        <div class="card-name">${escapeHtml(s.name || id)}</div>
        ${s.name ? `<div class="card-vin">${escapeHtml(id)}</div>` : ""}
      </div>
      <div class="card-status" id="status-${escapeHtml(id)}"><span class="dot"></span><span>Offline</span></div>
    </div>
    <div class="card-badge" id="badge-${escapeHtml(id)}"></div>
    <div class="card-stats" id="connection-${escapeHtml(id)}"></div>
    <div class="card-section">${renderCommandsHTML(id)}</div>
    <div class="card-section events-section" id="events-section-${escapeHtml(id)}" style="display:none">
      <div class="events-head">
        <span class="section-label">Events</span>
        <span class="clear-events" id="clear-events-${escapeHtml(id)}" data-action="clear-events" data-scooter="${escapeHtml(id)}" style="display:none">Clear all</span>
      </div>
      <div id="events-${escapeHtml(id)}"></div>
    </div>
    <div class="card-section">
      <div class="section-label" style="margin-bottom:6px">State</div>
      <div class="state-groups" id="state-${escapeHtml(id)}"><p class="muted">No state yet.</p></div>
    </div>
  </div>`;
}

export function renderScooters() {
  const container = document.getElementById("scootersContainer");
  const placeholder = container.querySelector(".loading, .empty-state");
  if (placeholder) placeholder.remove();

  if (!store.scooters.length) {
    container.innerHTML = '<div class="empty-state">No scooters yet. Use the + button to add one.</div>';
    return;
  }

  const seen = new Set();
  for (const s of store.scooters) {
    seen.add(s.identifier);
    let card = document.getElementById(`scooter-${s.identifier}`);
    if (!card) {
      container.insertAdjacentHTML("beforeend", cardHTML(s));
    }
    setScooterOnline(s.identifier, !!s.connected);
    if (store.states[s.identifier]) {
      renderBadge(s.identifier, store.states[s.identifier]);
      renderState(s.identifier, store.states[s.identifier], null);
    }
    updateConnectionStats(s.identifier, s);
  }

  for (const card of [...container.querySelectorAll(".scooter-card")]) {
    const id = card.id.replace("scooter-", "");
    if (!seen.has(id)) card.remove();
  }
}

export function setScooterOnline(id, online) {
  const card = document.getElementById(`scooter-${id}`);
  const status = document.getElementById(`status-${id}`);
  if (card) card.classList.toggle("offline", !online);
  if (status) {
    status.querySelector(".dot").classList.toggle("online", online);
    status.querySelector("span:last-child").textContent = online ? "Connected" : "Offline";
  }
}

export function updateConnectionStats(id, data) {
  const el = document.getElementById(`connection-${id}`);
  if (!el) return;
  data = data || {};
  const s = store.scooters.find((x) => x.identifier === id) || {};
  const version = data.version || s.version;
  const uptimeSec = data.uptime_seconds || s.uptime_seconds;
  const wireS = data.wire_bytes_sent ?? s.wire_bytes_sent;
  const wireR = data.wire_bytes_received ?? s.wire_bytes_received;
  const tel = data.telemetry_received ?? s.telemetry_received;
  const cmd = data.commands_sent ?? s.commands_sent;

  if (version === undefined && wireS === undefined && tel === undefined) {
    el.textContent = "";
    return;
  }
  const parts = [];
  if (version) parts.push(version);
  parts.push(uptimeSec ? formatDuration(uptimeSec) : "—");
  if (wireS !== undefined || wireR !== undefined) parts.push(`↑${formatBytes(wireS)} ↓${formatBytes(wireR)}`);
  parts.push(`Tel:${tel || 0} Cmd:${cmd || 0}`);
  el.textContent = parts.join(" · ");
}

// applyStateUpdate merges a full snapshot or a delta and re-renders.
export function applyStateUpdate(id, incoming, updateType) {
  if (updateType === "full" || !store.states[id]) {
    store.states[id] = incoming || {};
    renderBadge(id, store.states[id]);
    renderState(id, store.states[id], null);
    return;
  }
  const cur = store.states[id];
  const changed = {};
  for (const [comp, fields] of Object.entries(incoming || {})) {
    if (fields && typeof fields === "object") {
      cur[comp] = cur[comp] || {};
      changed[comp] = {};
      for (const [k, v] of Object.entries(fields)) {
        cur[comp][k] = v;
        changed[comp][k] = v;
      }
    } else {
      cur[comp] = fields;
    }
  }
  renderBadge(id, cur);
  renderState(id, cur, changed);
}
