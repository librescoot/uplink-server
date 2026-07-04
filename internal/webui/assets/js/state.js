// Grouped, collapsible scooter-state rendering (replaces the flat key/value
// table) plus the condensed badge chips.

import { escapeHtml } from "./format.js";

// Top-level groups roll the many Redis-hash components into a few expandable
// sections.
const STATE_GROUPS = [
  { label: "Vehicle", keys: ["vehicle"] },
  { label: "Batteries", keys: ["battery:0", "battery:1", "aux-battery", "cb-battery"] },
  { label: "Location", keys: ["gps", "navigation"] },
  { label: "Powertrain", keys: ["engine-ecu", "power-manager", "power-mux"] },
  { label: "Connectivity", keys: ["internet", "modem", "ble"] },
  { label: "System", keys: ["system", "dashboard", "keycard", "ota", "alarm", "meta"] },
];

const NAMED = new Set(STATE_GROUPS.flatMap((g) => g.keys));

function present(state, name) {
  const v = state[name];
  return v && typeof v === "object" && Object.keys(v).length > 0;
}

function componentsForGroup(state, group) {
  if (group.keys) return group.keys.filter((k) => present(state, k));
  // "Other": present components not claimed by a named group.
  return Object.keys(state).filter((k) => !NAMED.has(k) && present(state, k));
}

export function renderState(scooterId, state, changed) {
  const container = document.getElementById(`state-${scooterId}`);
  if (!container) return;

  const openKeys = new Set(
    [...container.querySelectorAll("details[data-key]")].filter((d) => d.open).map((d) => d.dataset.key)
  );
  const firstRender = container.querySelector("details") === null;

  const groups = STATE_GROUPS.concat([{ label: "Other", keys: null }]);
  const html = groups
    .map((g) => {
      const comps = componentsForGroup(state, g);
      if (!comps.length) return "";
      const gkey = `g:${g.label}`;
      const gopen = firstRender ? g.label === "Vehicle" : openKeys.has(gkey);

      const compsHTML = comps
        .map((name) => {
          const fields = state[name];
          const ckey = `c:${name}`;
          const copen = firstRender ? false : openKeys.has(ckey);
          const changedComp = changed && changed[name];
          const rows = Object.entries(fields)
            .map(([k, v]) => {
              const isCh = changedComp && Object.prototype.hasOwnProperty.call(changedComp, k);
              return `<span class="kv-key">${escapeHtml(k)}</span><span class="kv-val${isCh ? " value-updated" : ""}">${escapeHtml(v)}</span>`;
            })
            .join("");
          const flash = changedComp && Object.keys(changedComp).length > 0 && !copen ? " summary-updated" : "";
          return `<details class="state-comp" data-key="${ckey}"${copen ? " open" : ""}>
            <summary class="${flash.trim()}">${escapeHtml(name)}<span class="summary-count">${Object.keys(fields).length}</span></summary>
            <div class="kv">${rows}</div>
          </details>`;
        })
        .join("");

      return `<details class="state-group" data-key="${gkey}"${gopen ? " open" : ""}>
        <summary>${escapeHtml(g.label)}<span class="summary-count">${comps.length}</span></summary>
        <div>${compsHTML}</div>
      </details>`;
    })
    .join("");

  container.innerHTML = html || '<p class="muted">No state yet.</p>';
}

function chip(text, cls) {
  return `<span class="chip${cls ? " " + cls : ""}">${escapeHtml(text)}</span>`;
}

export function renderBadge(scooterId, state) {
  const badge = document.getElementById(`badge-${scooterId}`);
  if (!badge) return;
  const chips = [];
  const vehicle = state.vehicle || {};
  if (vehicle.state) chips.push(chip(vehicle.state));

  const b0 = state["battery:0"];
  const b1 = state["battery:1"];
  if (!b0 || b0.present !== "true") {
    chips.push(chip("🔋 –"));
  } else {
    let t = `🔋 ${b0.charge}%`;
    if (b1 && b1.present === "true" && b1.charge) t += ` / ${b1.charge}%`;
    chips.push(chip(t));
  }
  if (vehicle["seatbox:lock"]) chips.push(chip(`🔒 ${vehicle["seatbox:lock"]}`));
  if (state.alarm && state.alarm["alarm-active"] === "true") chips.push(chip("🚨 alarm", "alarm"));
  const ota = state.ota || {};
  if (ota.status && ota.status !== "none") {
    let t = `📦 ${ota.status}`;
    if (ota["install-progress"]) t += ` ${ota["install-progress"]}%`;
    chips.push(chip(t));
  }
  badge.innerHTML = chips.join("") || '<span class="chip muted">no data</span>';
}
