// API credential storage and fetch wrapper.

const KEY = "uplink_api_key";
let apiKey = localStorage.getItem(KEY) || "";

export function getApiKey() {
  return apiKey;
}

export function setApiKey(k) {
  apiKey = k || "";
  if (apiKey) localStorage.setItem(KEY, apiKey);
}

export function clearApiKey() {
  apiKey = "";
  localStorage.removeItem(KEY);
}

export async function apiRequest(endpoint, options = {}) {
  if (!apiKey) throw new Error("No credentials configured");
  const res = await fetch(endpoint, {
    ...options,
    headers: {
      "X-API-Key": apiKey,
      "Content-Type": "application/json",
      ...(options.headers || {}),
    },
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "Request failed" }));
    const e = new Error(err.error || `HTTP ${res.status}`);
    e.status = res.status;
    throw e;
  }
  return res.json();
}

// login exchanges username/password for a session token, which is then used
// exactly like an API key.
export async function login(username, password) {
  const res = await fetch("/api/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "Login failed" }));
    throw new Error(err.error || `HTTP ${res.status}`);
  }
  const data = await res.json();
  setApiKey(data.token);
  return data;
}
