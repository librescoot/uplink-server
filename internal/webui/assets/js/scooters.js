// Two-tier UI: an overview dashboard (fleet KPIs + compact scooter tiles) and a
// per-scooter detail view (controls, events, raw state). The detail view reuses
// the command/state/event modules; the dashboard shows only a curated summary.

import { store } from "./store.js";
import { escapeHtml, formatDuration, formatBytes } from "./format.js";
import { renderCommandsHTML } from "./commands.js";
import { renderBadge, renderState } from "./state.js";
import { loadEvents } from "./events.js";

function container() {
  return document.getElementById("scootersContainer");
}

function scooterInfo(id) {
  return store.scooters.find((s) => s.identifier === id) || { identifier: id };
}

// --- summary derivation (what a glance needs) ---

function batteryLevel(st) {
  const b0 = st["battery:0"];
  if (b0 && b0.present === "true" && b0.charge != null) {
    const n = parseInt(b0.charge, 10);
    return isNaN(n) ? null : n;
  }
  return null;
}

function meterClass(level) {
  if (level < 20) return "low";
  if (level < 50) return "med";
  return "";
}

function summary(id) {
  const s = scooterInfo(id);
  const st = store.states[id] || {};
  const online = !!s.connected;
  const vehicle = st.vehicle || {};
  return {
    online,
    name: s.name || "",
    state: vehicle.state || (online ? "—" : "offline"),
    battery: batteryLevel(st),
    speed: parseFloat((st["engine-ecu"] || {}).speed),
    seatbox: vehicle["seatbox:lock"],
    alarm: st.alarm && st.alarm["alarm-active"] === "true",
  };
}

// --- dashboard ---

function tileHTML(id) {
  const sm = summary(id);
  const meter =
    sm.battery != null
      ? `<div class="meter"><div class="meter-fill ${meterClass(sm.battery)}" style="width:${sm.battery}%"></div></div>`
      : "";
  const facts = [];
  facts.push(sm.battery != null ? `🔋 ${sm.battery}%` : "no battery");
  if (!isNaN(sm.speed) && sm.speed > 0) facts.push(`${sm.speed} km/h`);
  if (sm.seatbox) facts.push(`🔒 ${sm.seatbox}`);
  if (sm.alarm) facts.push("🚨 alarm");

  return `<button class="tile ${sm.online ? "" : "offline"}" id="tile-${escapeHtml(id)}" data-action="open-detail" data-scooter="${escapeHtml(id)}">
    <div class="tile-head">
      <span class="tile-name">${escapeHtml(sm.name || id)}</span>
      <span class="dot ${sm.online ? "online" : ""}"></span>
    </div>
    ${sm.name ? `<div class="tile-vin">${escapeHtml(id)}</div>` : ""}
    <div class="tile-state"><span class="chip">${escapeHtml(sm.state)}</span></div>
    ${meter}
    <div class="tile-facts">${facts.map((f) => `<span>${escapeHtml(f)}</span>`).join("")}</div>
  </button>`;
}

function fleetCounts() {
  const total = store.scooters.length;
  const online = store.scooters.filter((s) => s.connected).length;
  const driving = store.scooters.filter(
    (s) => s.connected && (store.states[s.identifier] || {}).vehicle && store.states[s.identifier].vehicle.state === "ready-to-drive"
  ).length;
  return { total, online, driving, offline: total - online };
}

function renderDashboard() {
  const c = container();
  if (!store.scooters.length) {
    c.innerHTML =
      '<div class="page-head"><h1>Fleet</h1></div><div class="empty-state">No scooters yet. Use the + button to add one.</div>';
    return;
  }
  const k = fleetCounts();
  c.innerHTML = `
    <div class="page-head"><h1>Fleet</h1><div class="sub">${k.total} scooter${k.total === 1 ? "" : "s"}</div></div>
    <div class="kpis">
      <div class="kpi"><div class="kpi-value" id="kpi-total">${k.total}</div><div class="kpi-label">Total</div></div>
      <div class="kpi"><div class="kpi-value online" id="kpi-online">${k.online}</div><div class="kpi-label">Online</div></div>
      <div class="kpi"><div class="kpi-value accent" id="kpi-driving">${k.driving}</div><div class="kpi-label">In ride</div></div>
      <div class="kpi"><div class="kpi-value" id="kpi-offline">${k.offline}</div><div class="kpi-label">Offline</div></div>
    </div>
    <div class="tiles">${store.scooters.map((s) => tileHTML(s.identifier)).join("")}</div>`;
}

function updateKpis() {
  const k = fleetCounts();
  const set = (id, v) => {
    const el = document.getElementById(id);
    if (el) el.textContent = v;
  };
  set("kpi-total", k.total);
  set("kpi-online", k.online);
  set("kpi-driving", k.driving);
  set("kpi-offline", k.offline);
}

function updateTile(id) {
  const el = document.getElementById(`tile-${id}`);
  if (el) el.outerHTML = tileHTML(id);
  else renderDashboard();
  updateKpis();
}

// --- detail ---

function renderDetail(id) {
  const c = container();
  const s = scooterInfo(id);
  c.innerHTML = `
    <div class="detail">
      <div class="detail-top"><button class="btn-back" data-action="back">← All scooters</button></div>
      <div class="detail-head">
        <div>
          <div class="detail-name">${escapeHtml(s.name || id)}</div>
          ${s.name ? `<div class="tile-vin">${escapeHtml(id)}</div>` : ""}
        </div>
        <div class="card-status" id="status-${escapeHtml(id)}"><span class="dot"></span><span>Offline</span></div>
      </div>
      <div class="detail-summary" id="badge-${escapeHtml(id)}"></div>
      <div class="card-stats" id="connection-${escapeHtml(id)}"></div>
      <div class="detail-grid">
        <div>
          <section><h3 class="section-label">Controls</h3>${renderCommandsHTML(id)}</section>
          <section class="events-section" id="events-section-${escapeHtml(id)}" style="display:none">
            <div class="events-head">
              <h3 class="section-label">Events</h3>
              <span class="clear-events" id="clear-events-${escapeHtml(id)}" data-action="clear-events" data-scooter="${escapeHtml(id)}" style="display:none">Clear all</span>
            </div>
            <div id="events-${escapeHtml(id)}"></div>
          </section>
        </div>
        <div>
          <section><h3 class="section-label">State</h3>
            <div class="state-groups" id="state-${escapeHtml(id)}"><p class="muted">No state yet.</p></div>
          </section>
        </div>
      </div>
    </div>`;

  setScooterOnline(id, !!s.connected);
  updateConnectionStats(id, s);
  const st = store.states[id];
  if (st) {
    renderBadge(id, st);
    renderState(id, st, null);
  }
  loadEvents(id);
}

// --- public view control ---

export function render() {
  if (store.view === "detail" && store.currentScooter) renderDetail(store.currentScooter);
  else renderDashboard();
}

export function showDashboard() {
  store.view = "dashboard";
  store.currentScooter = null;
  renderDashboard();
}

export function openDetail(id) {
  store.view = "detail";
  store.currentScooter = id;
  renderDetail(id);
}

export function back() {
  showDashboard();
}

// Called when the scooter list changes (added/removed/reordered).
export function onScootersChanged() {
  if (store.view === "detail") {
    if (!store.scooters.some((s) => s.identifier === store.currentScooter)) {
      showDashboard();
      return;
    }
    // Keep the detail open; refresh its status line.
    setScooterOnline(store.currentScooter, !!scooterInfo(store.currentScooter).connected);
    return;
  }
  renderDashboard();
}

// --- live updates ---

export function applyStateUpdate(id, incoming, updateType) {
  let changed = null;
  if (updateType === "full" || !store.states[id]) {
    store.states[id] = incoming || {};
  } else {
    const cur = store.states[id];
    changed = {};
    for (const [comp, fields] of Object.entries(incoming || {})) {
      if (fields && typeof fields === "object") {
        cur[comp] = cur[comp] || {};
        changed[comp] = {};
        for (const [key, val] of Object.entries(fields)) {
          cur[comp][key] = val;
          changed[comp][key] = val;
        }
      } else {
        cur[comp] = fields;
      }
    }
  }

  if (store.view === "detail") {
    if (id === store.currentScooter) {
      renderBadge(id, store.states[id]);
      renderState(id, store.states[id], changed);
    }
  } else {
    updateTile(id);
  }
}

export function setScooterOnline(id, online) {
  const s = scooterInfo(id);
  s.connected = online;
  if (store.view === "detail" && id === store.currentScooter) {
    const status = document.getElementById(`status-${id}`);
    if (status) {
      status.querySelector(".dot").classList.toggle("online", online);
      status.querySelector("span:last-child").textContent = online ? "Connected" : "Offline";
    }
  } else if (store.view === "dashboard") {
    updateTile(id);
  }
}

export function updateConnectionStats(id, data) {
  const el = document.getElementById(`connection-${id}`);
  if (!el) return;
  data = data || {};
  const s = scooterInfo(id);
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
