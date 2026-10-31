// The window: shows offlokd's status and sends its commands. The backend is the Go app (api.js); ?preview=<state>
// shows made-up data instead, for screenshots.
import { t } from "./strings.js";
import { api } from "./api.js";

const $ = (s) => document.querySelector(s);

const tones = {
  direct: "teal",
  "fallback-dot": "orange",
  "fallback-network": "orange",
  "strict-blocked": "orange",
  offline: "grey",
  off: "grey",
  error: "red",
  starting: "grey",
};
const chips = {
  direct: "chip.direct",
  "fallback-dot": "chip.fallback",
  "fallback-network": "chip.fallback",
  "strict-blocked": "chip.strict",
  offline: "chip.offline",
  off: "chip.off",
  error: "chip.error",
  starting: "chip.off",
};

function translateStatic() {
  document.documentElement.lang = navigator.language || "en";
  for (const el of document.querySelectorAll("[data-i18n]")) el.textContent = t(el.dataset.i18n);
  for (const el of document.querySelectorAll("[data-i18n-placeholder]")) el.placeholder = t(el.dataset.i18nPlaceholder);
}

function render(st) {
  const state = st.state || "starting";
  if (st.platform) document.documentElement.dataset.platform = st.platform;
  const head = $(".status");
  head.dataset.state = state;
  head.dataset.tone = tones[state] || "grey";
  $(".chip").textContent = t(chips[state] || "chip.off");
  $(".title").textContent = t(`state.${state}.title`);
  const dot = st.settings?.dot?.name || "";
  // offlokd's error messages are English and technical: a known code gets the window's own words.
  const code = st.error_code ? `error.code.${st.error_code}` : "";
  const error = code && t(code) !== code ? t(code) : st.error || "";
  let text = t(`state.${state}.text`, { dot, error });
  if (state === "error" && st.settings?.strict) text = t("state.error.strict", { error });
  $(".text").textContent = text;

  const signin = $(".signin");
  signin.hidden = state !== "strict-blocked" && !st.signin_until;
  $("[data-action=signin]").textContent = t("signin.button");
  const minutes = st.signin_until ? Math.max(1, Math.ceil((Date.parse(st.signin_until) - Date.now()) / 60000)) : 0;
  $(".signin .hint").textContent = minutes ? t("signin.active", { minutes }) : t("signin.text");
  $("[data-action=signin]").disabled = minutes > 0;

  const on = state !== "off";
  $("[data-action=power]").setAttribute("aria-checked", String(on));
  $(".power-state").textContent = t(on ? "power.on" : "power.off");
  $("[data-action=strict]").setAttribute("aria-checked", String(!!st.settings?.strict));
  const input = $(".dot input");
  if (document.activeElement !== input) input.value = dot;
  $("[data-action=dot-remove]").hidden = !dot;
  const access = st.settings?.settings_access || "admins";
  for (const b of document.querySelectorAll("[data-access]")) b.setAttribute("aria-checked", String(b.dataset.access === access));
  $(".admin-hint").hidden = st.settings?.settings_access === "everyone" || st.admin;

  $("[data-action=updates]").setAttribute("aria-checked", String(st.settings?.updates !== false));
  renderUpdate(st.update || {});

  const servers = st.network?.servers || [];
  $(".network").textContent = t("network.line", { servers: servers.length ? servers.join(", ") : t("network.none") });
  $(".version").textContent = [
    st.version && t("footer.offlok", { version: st.version }),
    st.unbound && t("footer.unbound", { version: st.unbound }),
  ].filter(Boolean).join(" · ");
  renderBlocking(st);
  lastStatus = st;
}

// The update offlokd found: what it is, then its download and install.
function renderUpdate(u) {
  $(".update").hidden = !u.available;
  if (!u.available) return;
  const busy = u.state === "downloading" || u.state === "installing";
  $(".update .name").textContent = t(busy ? `update.${u.state}` : "update.available", { version: u.available });
  $(".update .sub").textContent = u.error ? t("update.failed", { error: u.error }) : t(busy ? "update.wait" : "update.text");
  $(".update .sub").classList.toggle("failed", !!u.error && !busy);
  $("[data-action=update]").hidden = busy;
  $("[data-action=notes]").hidden = !u.page;
  $(".update-actions").hidden = busy && !u.page;
}

// The categories this build offers (Google Play's has no ads); the preview's made-up status names none.
const offered = (st) => st.blocking?.categories || ["ads", "trackers", "malicious", "custom"];
let lastStatus = {};

function el(tag, attrs = {}, ...children) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === "class") e.className = v;
    else if (k.startsWith("data-") || k.startsWith("aria-") || k === "type" || k === "role") e.setAttribute(k, v);
    else e[k] = v;
  }
  e.append(...children);
  return e;
}

function categoryRow(cat, st) {
  const on = !!st.settings?.blocking?.[cat];
  const lists = (st.blocking?.lists || []).filter((l) => l.category === cat);
  let sub = t(`cat.${cat}.sub`);
  if (cat === "custom") {
    const n = st.settings?.custom_lists?.length || 0;
    sub = n === 0 ? t("cat.custom.none") : n === 1 ? t("cat.custom.one") : t("cat.custom.some", { n });
  }
  const label = el("div", { class: "label" }, el("span", { class: "name", textContent: t(`cat.${cat}`) }), el("span", { class: "sub", textContent: sub }));
  if (on && lists.length) {
    const names = lists.reduce((n, l) => n + (l.names || 0), 0);
    const failed = lists.some((l) => l.error);
    const text = failed ? t("lists.failed") : names ? t("lists.names", { n: names.toLocaleString() }) : t("lists.pending");
    label.append(el("span", { class: failed ? "count failed" : "count", textContent: text }));
  }
  const sw = el("button", { class: "switch", type: "button", role: "switch", "aria-checked": String(on) }, el("span"));
  sw.addEventListener("click", () => act("block", `${cat}=${on ? "off" : "on"}`));
  return el("div", { class: "row" }, label, sw);
}

function renderBlocking(st) {
  $(".cats").replaceChildren(...offered(st).map((c) => categoryRow(c, st)));
  const anyOn = offered(st).some((c) => st.settings?.blocking?.[c]);
  const paused = st.blocking?.paused_until ? new Date(st.blocking.paused_until) : null;
  const pause = $("[data-action=pause]");
  pause.hidden = !anyOn;
  pause.textContent = t(paused ? "blocking.resume" : "blocking.pause");
  $(".paused").hidden = !paused;
  if (paused) $(".paused").textContent = t("blocking.paused", { time: paused.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) });

  const customOn = !!st.settings?.blocking?.custom;
  const urls = st.settings?.custom_lists || [];
  $(".custom").hidden = !customOn;
  const lists = $(".custom-lists");
  lists.hidden = !customOn || !urls.length;
  lists.replaceChildren(...urls.map((u) => {
    const rm = el("button", { class: "remove", type: "button", textContent: "×", "aria-label": t("custom.remove", { list: u }) });
    rm.addEventListener("click", () => act("list", `remove ${u}`));
    return el("li", {}, el("span", { textContent: u, title: u }), rm);
  }));

  const trusted = st.settings?.trusted || [];
  $(".trusted-row").hidden = !trusted.length;
  $(".trusted").replaceChildren(...trusted.map((site) => {
    const rm = el("button", { class: "remove", type: "button", textContent: "×", "aria-label": t("trusted.remove", { site }) });
    rm.addEventListener("click", () => act("untrust", site));
    return el("li", {}, el("span", { textContent: site }), rm);
  }));
}

async function checkSite(site) {
  const box = $(".check-result");
  try {
    showError(null);
    const c = await api.check(site);
    let key = "check.free";
    if (c.match) key = c.trusted ? "check.trusted" : c.paused ? "check.paused" : "check.blocked";
    box.dataset.blocked = String(!!c.blocked);
    box.querySelector("p").textContent = t(key, { site: c.name, list: c.match?.source || "", category: c.match ? t(`cat.${c.match.category}`) : "" });
    const trust = $("[data-action=trust]");
    trust.hidden = !c.blocked;
    trust.dataset.site = c.name;
    box.hidden = false;
  } catch (err) {
    showError(err);
  }
}

function showError(err) {
  const box = $(".error");
  box.hidden = !err;
  box.textContent = err ? (err.denied ? t("error.denied") : err.notRunning ? t("error.not-running") : err.message) : "";
}

async function act(cmd, value) {
  try {
    showError(null);
    render(await api.send(cmd, value));
  } catch (err) {
    showError(err);
    refresh();
  }
}

async function refresh() {
  try {
    render(await api.status());
  } catch (err) {
    showError(err);
  }
}

translateStatic();
$("[data-action=power]").addEventListener("click", (e) => act(e.currentTarget.getAttribute("aria-checked") === "true" ? "off" : "on"));
$("[data-action=strict]").addEventListener("click", (e) => act("strict", e.currentTarget.getAttribute("aria-checked") === "true" ? "off" : "on"));
for (const b of document.querySelectorAll("[data-access]")) {
  b.addEventListener("click", () => b.getAttribute("aria-checked") !== "true" && act("access", b.dataset.access));
}
$("[data-action=signin]").addEventListener("click", () => act("signin"));
$("[data-action=updates]").addEventListener("click", (e) => act("updates", e.currentTarget.getAttribute("aria-checked") === "true" ? "off" : "on"));
$("[data-action=update]").addEventListener("click", () => act("update"));
$("[data-action=notes]").addEventListener("click", () => api.notes().catch(showError));
$("[data-action=dot-remove]").addEventListener("click", () => act("dot", "none"));
$("[data-action=pause]").addEventListener("click", () => act("pause", lastStatus.blocking?.paused_until ? "off" : ""));
$(".custom").addEventListener("submit", (e) => {
  e.preventDefault();
  const v = $(".custom input").value.trim();
  if (v) act("list", `add ${v}`).then(() => ($(".custom input").value = ""));
});
$(".check").addEventListener("submit", (e) => {
  e.preventDefault();
  const v = $(".check input").value.trim();
  if (v) checkSite(v);
});
$("[data-action=trust]").addEventListener("click", async (e) => {
  const site = e.currentTarget.dataset.site;
  await act("trust", site);
  checkSite(site);
});
$(".dot").addEventListener("submit", (e) => {
  e.preventDefault();
  const v = $(".dot input").value.trim();
  if (v) act("dot", v);
});
refresh();
setInterval(refresh, 3000);
// ?check=<site> runs "Is this site blocked?" at once (screenshots).
const preset = new URLSearchParams(location.search).get("check");
if (preset) {
  $(".check input").value = preset;
  checkSite(preset);
}
