// Live data over /ws/web: connect, dispatch, reconnect with backoff.

import { getApiKey } from "./api.js";
import { store, upsertScooter } from "./store.js";
import { renderScooters, setScooterOnline, updateConnectionStats, applyStateUpdate } from "./scooters.js";
import { addEventToDisplay } from "./events.js";

let ws = null;
let reconnectDelay = 1000;

export function connectWebSocket() {
  const key = getApiKey();
  if (!key) return;
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  ws = new WebSocket(`${proto}//${location.host}/ws/web?api_key=${encodeURIComponent(key)}`);

  ws.onopen = () => {
    reconnectDelay = 1000;
  };
  ws.onmessage = (e) => {
    try {
      handleMessage(JSON.parse(e.data));
    } catch (_) {
      /* ignore malformed frame */
    }
  };
  ws.onclose = () => {
    for (const s of store.scooters) setScooterOnline(s.identifier, false);
    setTimeout(connectWebSocket, reconnectDelay);
    reconnectDelay = Math.min(reconnectDelay * 2, 30000);
  };
}

export function closeWebSocket() {
  if (ws) {
    ws.onclose = null;
    ws.close();
    ws = null;
  }
}

function handleMessage(msg) {
  switch (msg.type) {
    case "scooter_list":
      store.scooters = (msg.scooters || []).map((s) => ({ ...s }));
      renderScooters();
      break;
    case "state_update":
      applyStateUpdate(msg.scooter_id, msg.state, msg.update_type);
      updateConnectionStats(msg.scooter_id, msg);
      break;
    case "event":
      addEventToDisplay(msg.scooter_id, {
        event: msg.event,
        data: msg.event_data,
        timestamp: msg.timestamp,
        id: msg.event_id,
      });
      break;
    case "scooter_online": {
      const info = msg.scooter || { identifier: msg.scooter_id, connected: true };
      info.connected = true;
      upsertScooter(info);
      renderScooters();
      setScooterOnline(info.identifier, true);
      break;
    }
    case "scooter_offline":
      setScooterOnline(msg.scooter_id, false);
      break;
  }
}
