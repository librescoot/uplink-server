// Shared client-side state.

export const store = {
  scooters: [], // ScooterInfo list (connected + registered)
  states: {},   // scooterId -> merged component state map
};

export function upsertScooter(info) {
  const i = store.scooters.findIndex((s) => s.identifier === info.identifier);
  if (i >= 0) store.scooters[i] = { ...store.scooters[i], ...info };
  else store.scooters.push(info);
}

export function removeScooter(identifier) {
  store.scooters = store.scooters.filter((s) => s.identifier !== identifier);
}
