// Command definitions (data-driven) plus send + response polling.

import { apiRequest } from "./api.js";
import { escapeHtml } from "./format.js";

// Quick actions shown inline on every card.
const QUICK = [
  { label: "Lock", cmd: "lock", variant: "primary" },
  { label: "Unlock", cmd: "unlock" },
  { label: "Open Seatbox", cmd: "open_seatbox" },
  { label: "Refresh", cmd: "get_state" },
  { label: "History", action: "history" },
];

// Collapsible command groups.
const GROUPS = [
  {
    label: "Access",
    commands: [
      { label: "Lock + Hibernate", cmd: "lock_hibernate" },
      { label: "Force Lock", cmd: "force_lock", variant: "danger" },
      { label: "Handlebar Lock", cmd: "handlebar_lock" },
      { label: "Handlebar Unlock", cmd: "handlebar_unlock" },
    ],
  },
  {
    label: "Lights",
    commands: [
      { label: "Blinker ←", cmd: "blinker_left" },
      { label: "Blinker →", cmd: "blinker_right" },
      { label: "Blinker ⚠", cmd: "blinker_both" },
      { label: "Blinker Off", cmd: "blinker_off" },
      { label: "Dashboard On", cmd: "dashboard_on" },
      { label: "Dashboard Off", cmd: "dashboard_off" },
    ],
  },
  {
    label: "Alarm",
    commands: [
      { label: "Arm", cmd: "alarm_arm" },
      { label: "Disarm", cmd: "alarm_disarm" },
      { label: "Stop", cmd: "alarm_stop" },
      { label: "Enable", cmd: "alarm_enable" },
      { label: "Disable", cmd: "alarm_disable" },
    ],
  },
  {
    label: "Power",
    commands: [
      { label: "Hibernate", cmd: "hibernate", confirm: "Put scooter into hibernate mode?" },
      { label: "Hibernate (manual)", cmd: "hibernate_manual" },
      { label: "Reboot", cmd: "reboot", variant: "danger", confirm: "Reboot the scooter?" },
      { label: "Engine On", cmd: "engine_on" },
      { label: "Engine Off", cmd: "engine_off" },
    ],
  },
  {
    label: "Diagnostics",
    commands: [
      { label: "Get State", cmd: "get_state" },
      { label: "Ping", cmd: "ping" },
      // uplink-service honk reads params.duration (ms).
      { label: "Honk 500ms", cmd: "honk", params: { duration: 500 } },
    ],
  },
];

function btnHTML(scooterId, c) {
  const attrs = [
    `class="cmd-btn"`,
    c.variant ? `data-variant="${c.variant}"` : "",
    `data-scooter="${escapeHtml(scooterId)}"`,
    c.action ? `data-action="${c.action}"` : `data-cmd="${c.cmd}"`,
    c.params ? `data-params='${escapeHtml(JSON.stringify(c.params))}'` : "",
    c.confirm ? `data-confirm="${escapeHtml(c.confirm)}"` : "",
  ].filter(Boolean).join(" ");
  return `<button ${attrs}>${escapeHtml(c.label)}</button>`;
}

export function renderCommandsHTML(scooterId) {
  const quick = QUICK.map((c) => btnHTML(scooterId, c)).join("");
  const groups = GROUPS.map((g) => `
    <details class="cmd-group">
      <summary>${escapeHtml(g.label)}</summary>
      <div class="cmd-buttons">${g.commands.map((c) => btnHTML(scooterId, c)).join("")}</div>
    </details>`).join("");
  return `
    <div class="cmd-quick">${quick}</div>
    <div class="cmd-groups">${groups}</div>
    <div id="response-${escapeHtml(scooterId)}" class="cmd-response hidden"></div>`;
}

function respEl(scooterId) {
  return document.getElementById(`response-${scooterId}`);
}

function showResp(el, text, cls) {
  if (!el) return;
  el.className = `cmd-response ${cls || ""}`.trim();
  el.textContent = text;
  el.classList.remove("hidden");
}

export async function sendCommand(scooterId, command, params = {}) {
  const el = respEl(scooterId);
  showResp(el, `${command}…`, "");
  try {
    const res = await apiRequest("/api/commands", {
      method: "POST",
      body: JSON.stringify({ scooter_id: scooterId, command, params }),
    });
    if (res.request_id) pollResponse(res.request_id, el, command);
    else showResp(el, res.status || "sent", "success");
  } catch (e) {
    showResp(el, `✗ ${command}: ${e.message}`, "error");
  }
}

async function pollResponse(requestId, el, command) {
  try {
    const data = await apiRequest(`/api/commands/${encodeURIComponent(requestId)}`);
    if (data.status === "pending") {
      setTimeout(() => pollResponse(requestId, el, command), 700);
      return;
    }
    if (data.status === "success" || data.status === "completed") {
      showResp(el, `✓ ${command}`, "success");
      setTimeout(() => el && el.classList.add("hidden"), 5000);
    } else {
      showResp(el, `✗ ${command}: ${data.error || data.status}`, "error");
    }
  } catch (e) {
    showResp(el, `✗ ${command}: ${e.message}`, "error");
  }
}
