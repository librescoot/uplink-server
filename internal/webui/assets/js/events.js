// Event formatting, display, dismiss, and clear.

import { apiRequest } from "./api.js";
import { escapeHtml, formatTime } from "./format.js";

export function formatEventData(name, data) {
  data = data || {};
  switch (name) {
    case "battery_critical":
      return `🪫 Battery critical (${data.charge ?? "?"}%)`;
    case "cb_battery_critical":
      return `🪫 CB battery critical (${data.charge ?? "?"}%)`;
    case "power_state_change":
      return `⚡ Power state → ${data.state ?? "?"}`;
    case "connectivity_lost":
      return "📡 Connectivity lost";
    case "connectivity_regained":
      return "📡 Connectivity regained";
    case "lock_state_change":
      return `🔒 Lock state changed`;
    case "gps_fix_lost":
      return "🛰️ GPS fix lost";
    case "gps_fix_regained":
      return "🛰️ GPS fix regained";
    case "temperature_warning":
      return `🌡️ Temperature warning (${data.temperature ?? "?"})`;
    case "fault":
      return `❗ Fault: ${data.description || data.code || "unknown"}`;
    case "nrf_reset":
      return `🔁 NRF reset (${data.reason ?? "?"})`;
    case "ota_status_change":
      return `📦 OTA → ${data.status ?? "?"}`;
    case "alarm_triggered":
      return "🚨 Alarm triggered";
    case "alarm_cleared":
      return "🚨 Alarm cleared";
    case "alarm_state_change":
      return `🚨 Alarm state → ${data.status ?? "?"}`;
    case "unauthorized_movement":
      return `🚨 Unauthorized movement (${data.status ?? "?"})`;
    default:
      return `${name}: ${escapeHtml(JSON.stringify(data))}`;
  }
}

function eventItemHTML(scooterId, ev) {
  const id = ev.id || ev.event_id || "";
  const dismiss = id
    ? `<button class="event-dismiss" data-action="dismiss-event" data-scooter="${escapeHtml(scooterId)}" data-event="${escapeHtml(id)}" title="Dismiss">×</button>`
    : "";
  return `<div class="event-item" data-event-id="${escapeHtml(id)}">
    <span class="event-body">${escapeHtml(formatEventData(ev.event, ev.data))}</span>
    <span class="event-time">${escapeHtml(formatTime(ev.timestamp))}</span>
    ${dismiss}
  </div>`;
}

function section(scooterId) {
  return document.getElementById(`events-section-${scooterId}`);
}
function list(scooterId) {
  return document.getElementById(`events-${scooterId}`);
}
function clearBtn(scooterId) {
  return document.getElementById(`clear-events-${scooterId}`);
}

function updateVisibility(scooterId) {
  const l = list(scooterId);
  const sec = section(scooterId);
  const cb = clearBtn(scooterId);
  if (!l || !sec) return;
  const count = l.children.length;
  sec.style.display = count ? "" : "none";
  if (cb) cb.style.display = count > 1 ? "" : "none";
}

export function displayEvents(scooterId, events) {
  const l = list(scooterId);
  if (!l) return;
  l.innerHTML = (events || []).map((ev) => eventItemHTML(scooterId, ev)).join("");
  updateVisibility(scooterId);
}

export function addEventToDisplay(scooterId, ev) {
  const l = list(scooterId);
  if (!l) return;
  l.insertAdjacentHTML("afterbegin", eventItemHTML(scooterId, ev));
  updateVisibility(scooterId);
}

export async function loadEvents(scooterId) {
  try {
    const data = await apiRequest(`/api/scooters/${encodeURIComponent(scooterId)}/events`);
    displayEvents(scooterId, data.events || []);
  } catch (e) {
    /* offline / no events — ignore */
  }
}

export async function dismissEvent(scooterId, eventId) {
  try {
    await apiRequest(`/api/scooters/${encodeURIComponent(scooterId)}/events/${encodeURIComponent(eventId)}`, {
      method: "DELETE",
    });
  } catch (e) {
    /* ignore */
  }
  const l = list(scooterId);
  if (l) {
    const item = l.querySelector(`[data-event-id="${CSS.escape(eventId)}"]`);
    if (item) item.remove();
  }
  updateVisibility(scooterId);
}

export async function clearAllEvents(scooterId) {
  try {
    await apiRequest(`/api/scooters/${encodeURIComponent(scooterId)}/events`, { method: "DELETE" });
  } catch (e) {
    /* ignore */
  }
  const l = list(scooterId);
  if (l) l.innerHTML = "";
  updateVisibility(scooterId);
}
