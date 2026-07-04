// Small formatting and DOM helpers shared across modules.

export function escapeHtml(s) {
  return String(s == null ? "" : s).replace(/[&<>"]/g, (c) => ({
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    '"': "&quot;",
  }[c]));
}

export function formatDuration(seconds) {
  seconds = Math.floor(seconds || 0);
  if (seconds < 60) return `${seconds}s`;
  const m = Math.floor(seconds / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  return `${h}h ${m % 60}m`;
}

export function formatBytes(n) {
  n = n || 0;
  if (n < 1024) return `${n}B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)}KB`;
  return `${(n / (1024 * 1024)).toFixed(1)}MB`;
}

export function formatTime(ts) {
  const d = ts ? new Date(ts) : new Date();
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

// showStatus toggles a .status line (success|error|info).
export function showStatus(elementId, message, type) {
  const el = document.getElementById(elementId);
  if (!el) return;
  el.className = `status ${type || "info"}`;
  el.textContent = message;
  el.classList.remove("hidden");
}

export function hideStatus(elementId) {
  const el = document.getElementById(elementId);
  if (el) el.classList.add("hidden");
}
