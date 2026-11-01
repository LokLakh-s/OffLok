// offlok.org: the visitor's system first, the Linux tabs, the copy buttons, the header's line and what appears as one
// scrolls. The page reads well without it; nothing here talks to the network.
const root = document.documentElement;
root.classList.add("js");

// The visitor's system, from what the browser says. Phones before Linux: Android's user agent says "Linux"; an
// iPad says it is a Mac, with a touch screen.
function detect() {
  const ua = navigator.userAgent;
  const platform = (navigator.userAgentData && navigator.userAgentData.platform) || navigator.platform || "";
  if (/Android/i.test(ua)) return "android";
  if (/iPhone|iPad|iPod/i.test(ua) || (/Mac/i.test(platform) && navigator.maxTouchPoints > 1)) return "ios";
  if (/Win/i.test(platform) || /Windows/i.test(ua)) return "windows";
  if (/Mac/i.test(platform) || /Mac OS X/i.test(ua)) return "mac";
  if (/Linux|X11|CrOS/i.test(platform + ua)) return "linux";
  return "";
}
const os = detect();
if (os) root.dataset.os = os;

// The hero's button names the visitor's system (its labels are in the page, in its language).
const cta = document.querySelector("[data-cta]");
if (cta && os && cta.dataset["label" + os[0].toUpperCase() + os.slice(1)]) {
  cta.querySelector("span").textContent = cta.dataset["label" + os[0].toUpperCase() + os.slice(1)];
}

// Linux: .deb or .rpm. A distribution that names itself in the user agent picks its tab.
const tabs = [...document.querySelectorAll(".tabs [role=tab]")];
function select(tab) {
  for (const t of tabs) {
    const on = t === tab;
    t.setAttribute("aria-selected", String(on));
    t.tabIndex = on ? 0 : -1;
    document.getElementById(t.getAttribute("aria-controls")).hidden = !on;
  }
}
if (tabs.length) {
  select(/Fedora|Red Hat|openSUSE|SUSE|CentOS|Rocky|Alma/i.test(navigator.userAgent) ? tabs[1] : tabs[0]);
  tabs.forEach((tab, i) => {
    tab.addEventListener("click", () => select(tab));
    tab.addEventListener("keydown", (e) => {
      if (e.key !== "ArrowRight" && e.key !== "ArrowLeft") return;
      const next = tabs[(i + (e.key === "ArrowRight" ? 1 : tabs.length - 1)) % tabs.length];
      select(next);
      next.focus();
    });
  });
}

// Copy a command block.
for (const button of document.querySelectorAll(".copy")) {
  button.addEventListener("click", async () => {
    const text = button.parentElement.querySelector("pre").innerText.trim() + "\n";
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      const range = document.createRange();
      range.selectNodeContents(button.parentElement.querySelector("pre"));
      getSelection().removeAllRanges();
      getSelection().addRange(range);
      return;
    }
    button.classList.add("copied");
    setTimeout(() => button.classList.remove("copied"), 1800);
  });
}

// The phone menu closes once a link is followed, or on a click elsewhere.
const menu = document.querySelector(".menu");
if (menu) {
  menu.addEventListener("click", (e) => { if (e.target.closest("a")) menu.open = false; });
  document.addEventListener("click", (e) => { if (menu.open && !menu.contains(e.target)) menu.open = false; });
}

// A line under the header once the page moves.
const header = document.querySelector(".site-header");
const onScroll = () => header.classList.toggle("scrolled", scrollY > 8);
addEventListener("scroll", onScroll, { passive: true });
onScroll();

// What appears as one scrolls.
const reveal = document.querySelectorAll("[data-reveal]");
if ("IntersectionObserver" in window) {
  const seen = new IntersectionObserver((entries) => {
    for (const entry of entries) {
      if (!entry.isIntersecting) continue;
      entry.target.classList.add("in");
      seen.unobserve(entry.target);
    }
  }, { rootMargin: "0px 0px -8% 0px", threshold: 0.12 });
  reveal.forEach((el) => seen.observe(el));
} else {
  reveal.forEach((el) => el.classList.add("in"));
}
