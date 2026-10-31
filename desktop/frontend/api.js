// The window's backend. In the computers' app, the Go side answers through Wails; in Android's, the app's core
// answers through the OffLokAndroid bridge; with ?preview=<state> in the address, made-up data stands in
// (screenshots, design reviews). Errors carry .denied or .notRunning for the window's messages.
const params = new URLSearchParams(location.search);
const preview = params.get("preview");
if (params.get("theme")) document.documentElement.dataset.theme = params.get("theme");

const box = { servers: ["192.168.1.254"], domains: ["home"] };
const examples = {
  direct: { state: "direct" },
  "fallback-dot": { state: "fallback-dot", settings: { dot: { name: "dns.exemple.fr", port: 853 } } },
  "fallback-network": { state: "fallback-network" },
  "strict-blocked": { state: "strict-blocked", settings: { strict: true } },
  signin: { state: "fallback-network", settings: { strict: true }, signin_until: new Date(Date.now() + 4 * 60000).toISOString() },
  offline: { state: "offline", network: { servers: [] } },
  off: { state: "off" },
  error: { state: "error", error: "127.0.0.1:53 is already used by another program", error_code: "port-taken" },
  update: { state: "direct", update: { available: "0.6.0", page: "https://github.com/LokLakh-s/OffLok/releases/tag/v0.6.0" } },
  android: { state: "direct", platform: "android", backend: "android" },
  "android-private-dns": { state: "direct", platform: "android", backend: "android", warning: "android-private-dns" },
  "android-other-vpn": { state: "off", platform: "android", backend: "android", warning: "android-other-vpn" },
  updating: { state: "direct", update: { available: "0.6.0", state: "downloading" } },
  blocking: {
    state: "direct",
    settings: {
      blocking: { ads: true, trackers: true, custom: true },
      trusted: ["video.exemple.fr", "boutique.exemple.org"],
      custom_lists: ["https://listes.exemple.fr/ma-liste.txt"],
    },
    blocking: {
      lists: [
        { category: "ads", name: "AdGuard DNS filter", names: 184805 },
        { category: "trackers", name: "ShadowWhisperer Tracking List", names: 118095 },
        { category: "trackers", name: "HaGeZi's Apple Tracker Blocklist", names: 104 },
        { category: "custom", name: "https://listes.exemple.fr/ma-liste.txt", names: 0, error: "HTTP 404" },
      ],
    },
  },
};
examples.paused = { ...examples.blocking, blocking: { ...examples.blocking.blocking, paused_until: new Date(Date.now() + 7 * 60000).toISOString() } };

function made_up(state) {
  const ex = examples[state] || examples.direct;
  return {
    backend: "systemd-resolved",
    version: "0.5.0",
    unbound: "1.19.2",
    network: box,
    ...ex,
    settings: { enabled: ex.state !== "off", strict: false, settings_access: "admins", updates: true, ...(ex.settings || {}) },
  };
}

// A made-up "Is this site blocked?" for the preview: names with "pub" in them are.
function made_up_check(site) {
  const name = site.replace(/^https?:\/\//, "").split("/")[0];
  return name.includes("pub")
    ? { name, blocked: true, match: { category: "ads", source: "AdGuard DNS filter", entry: name } }
    : { name, blocked: false };
}

// Android's app (android/app/.../MainActivity.kt): each call gets its answer later, through offlokAnswer, as the
// first "on" waits for the user to let OffLok start a VPN.
const android = globalThis.OffLokAndroid;
const waiting = new Map();
let nextCall = 0;
globalThis.offlokAnswer = (id, result) => {
  waiting.get(id)?.(JSON.parse(result));
  waiting.delete(id);
};
function androidCall(cmd, value) {
  return new Promise((resolve) => {
    const id = ++nextCall;
    waiting.set(id, resolve);
    android.call(id, cmd, value);
  });
}

// The app's Go service (desktop/main.go, App), through the runtime Wails serves, or Android's core. Imported only in
// the computers' app: the preview has no Wails.
let runtime;
async function call(method, ...args) {
  let r;
  if (android) {
    if (method === "OpenNotes") return { ok: true }; // Android's stores update the app
    const [cmd, value = ""] = method === "Status" ? ["status"] : args;
    r = await androidCall(cmd, value);
  } else {
    runtime ??= await import("/wails/runtime.js").catch(() => null);
    if (!runtime) throw Object.assign(new Error("no backend"), { notRunning: true });
    r = await runtime.Call.ByName(`main.App.${method}`, ...args);
  }
  if (!r.ok) throw Object.assign(new Error(r.error || "error"), { denied: r.denied, notRunning: r.not_running });
  return r;
}

export const api = preview
  ? {
      status: async () => made_up(preview),
      send: async () => made_up(preview),
      check: async (site) => made_up_check(site),
      notes: async () => {},
      settings: () => {},
    }
  : {
      status: async () => (await call("Status")).status,
      send: async (cmd, value = "") => (await call("Send", cmd, value)).status,
      check: async (site) => (await call("Send", "check", site)).data,
      notes: async () => {
        await call("OpenNotes");
      },
      settings: (which) => android?.openSettings(which),
    };
