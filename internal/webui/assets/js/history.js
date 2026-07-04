// History dialog: fetch persisted telemetry and draw single-series line charts.
// Each metric is its own titled chart (no legend needed), line in the theme
// accent, flat 1px axes, with a hover readout.

import { apiRequest } from "./api.js";
import { escapeHtml, showStatus } from "./format.js";

let currentScooter = null;
let seq = 0;
const charts = new Map(); // svgId -> { pts:[{px,py,vx,vy}], fmt }

export function openHistory(id) {
  currentScooter = id;
  document.getElementById("historyTitle").textContent = `History — ${id}`;
  document.getElementById("historyCharts").innerHTML = "";
  document.getElementById("historyDialog").classList.add("show");
  loadHistory(id);
}

export function reloadHistory() {
  loadHistory(currentScooter);
}

async function loadHistory(id) {
  if (!id) return;
  const hours = parseInt(document.getElementById("historyRange").value, 10) || 24;
  const to = new Date();
  const from = new Date(to.getTime() - hours * 3600 * 1000);
  showStatus("historyStatus", "Loading…", "info");
  try {
    const data = await apiRequest(
      `/api/scooters/${encodeURIComponent(id)}/history?from=${from.toISOString()}&to=${to.toISOString()}&limit=2000`
    );
    renderCharts(data.history || []);
    showStatus("historyStatus", `${data.count || 0} data points`, "info");
  } catch (e) {
    showStatus("historyStatus", e.message, "error");
  }
}

function fieldNum(data, hash, field) {
  try {
    const f = parseFloat(data[hash][field]);
    return isNaN(f) ? null : f;
  } catch {
    return null;
  }
}

function renderCharts(rows) {
  const container = document.getElementById("historyCharts");
  charts.clear();
  if (!rows.length) {
    container.innerHTML = '<p class="muted">No data in this range.</p>';
    return;
  }
  const pts = rows.slice().reverse(); // API returns newest-first
  const t = pts.map((p) => new Date(p.timestamp).getTime());
  const series = [
    { title: "Speed (km/h)", ys: pts.map((p) => (p.speed != null ? p.speed : null)) },
    { title: "Battery 0 charge (%)", ys: pts.map((p) => fieldNum(p.data, "battery:0", "charge")) },
    { title: "Battery 1 charge (%)", ys: pts.map((p) => fieldNum(p.data, "battery:1", "charge")) },
  ];
  container.innerHTML = series.map((s) => chartSVG(s.title, t, s.ys)).join("");
  // Wire hover after insertion.
  for (const [id, meta] of charts) attachHover(id, meta);
}

function chartSVG(title, xs, ys) {
  const W = 620, H = 190, padL = 46, padR = 14, padT = 14, padB = 26;
  const valid = xs.map((x, i) => [x, ys[i]]).filter((p) => p[1] != null && !isNaN(p[1]));
  if (valid.length < 2) {
    return `<div class="chart"><h4>${escapeHtml(title)}</h4><p class="muted">Not enough data.</p></div>`;
  }
  const xmin = valid[0][0], xmax = valid[valid.length - 1][0];
  let ymin = Math.min(...valid.map((p) => p[1]));
  let ymax = Math.max(...valid.map((p) => p[1]));
  if (ymin === ymax) { ymin -= 1; ymax += 1; }
  const sx = (x) => padL + (xmax === xmin ? 0 : (x - xmin) / (xmax - xmin)) * (W - padL - padR);
  const sy = (y) => padT + (1 - (y - ymin) / (ymax - ymin)) * (H - padT - padB);

  const scaled = valid.map((p) => ({ px: sx(p[0]), py: sy(p[1]), vx: p[0], vy: p[1] }));
  const d = scaled.map((p, i) => `${i ? "L" : "M"}${p.px.toFixed(1)} ${p.py.toFixed(1)}`).join(" ");
  const tf = (ms) => new Date(ms).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  const id = `chart-${seq++}`;
  charts.set(id, { pts: scaled, ymin, ymax });

  return `<div class="chart">
    <h4>${escapeHtml(title)}</h4>
    <svg id="${id}" viewBox="0 0 ${W} ${H}" preserveAspectRatio="xMidYMid meet">
      <line class="axis" x1="${padL}" y1="${padT}" x2="${padL}" y2="${H - padB}"></line>
      <line class="axis" x1="${padL}" y1="${H - padB}" x2="${W - padR}" y2="${H - padB}"></line>
      <path class="line" d="${d}"></path>
      <text class="lbl" x="6" y="${padT + 4}">${ymax.toFixed(0)}</text>
      <text class="lbl" x="6" y="${H - padB}">${ymin.toFixed(0)}</text>
      <text class="lbl" x="${padL}" y="${H - 8}">${tf(xmin)}</text>
      <text class="lbl" x="${W - padR}" y="${H - 8}" text-anchor="end">${tf(xmax)}</text>
      <g class="hover" style="display:none">
        <line class="axis" y1="${padT}" y2="${H - padB}"></line>
        <circle r="3" fill="var(--accent)"></circle>
        <text class="lbl"></text>
      </g>
      <rect x="${padL}" y="${padT}" width="${W - padL - padR}" height="${H - padT - padB}" fill="transparent"></rect>
    </svg>
  </div>`;
}

function attachHover(id, meta) {
  const svg = document.getElementById(id);
  if (!svg) return;
  const rect = svg.querySelector("rect");
  const hover = svg.querySelector("g.hover");
  const vline = hover.querySelector("line");
  const dot = hover.querySelector("circle");
  const label = hover.querySelector("text");

  const toSvgX = (evt) => {
    const box = svg.getBoundingClientRect();
    const vb = svg.viewBox.baseVal;
    return ((evt.clientX - box.left) / box.width) * vb.width;
  };

  rect.addEventListener("mousemove", (evt) => {
    const x = toSvgX(evt);
    let nearest = meta.pts[0];
    for (const p of meta.pts) if (Math.abs(p.px - x) < Math.abs(nearest.px - x)) nearest = p;
    hover.style.display = "";
    vline.setAttribute("x1", nearest.px);
    vline.setAttribute("x2", nearest.px);
    dot.setAttribute("cx", nearest.px);
    dot.setAttribute("cy", nearest.py);
    const anchor = nearest.px > 520 ? "end" : "start";
    label.setAttribute("text-anchor", anchor);
    label.setAttribute("x", nearest.px + (anchor === "end" ? -6 : 6));
    label.setAttribute("y", Math.max(14, nearest.py - 6));
    label.textContent = `${nearest.vy} · ${new Date(nearest.vx).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`;
  });
  rect.addEventListener("mouseleave", () => (hover.style.display = "none"));
}
