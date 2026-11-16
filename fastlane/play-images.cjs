// The store listing's pictures, French and English, into fastlane/metadata/android/<lang>/images/: the feature
// graphic (1024 × 500, the site's share card made for a phone) and four phone screenshots (1080 × 1920) of the
// window's ?preview= mode on Android. Google Play's version has no ad blocking: no "Ads" row, as on the site.
//   NODE_PATH=<node_modules with playwright> node fastlane/play-images.cjs
const { chromium } = require("playwright");
const http = require("http");
const fs = require("fs");
const path = require("path");

const root = path.join(__dirname, "..");
const roots = { site: path.join(root, "site"), app: path.join(root, "desktop", "frontend") };
const langs = {
  fr: {
    locale: "fr-FR",
    dir: "fr-FR",
    site: "stats.exemple.fr",
    chip: "Logiciel libre · gratuit, sans compte",
    title: 'Ton téléphone,<br>son propre <span class="grad">DNS</span>.',
    lead: "Chaque adresse trouvée à la source, sans intermédiaire qui reçoit la liste de tes visites.",
  },
  en: {
    locale: "en-US",
    dir: "en-US",
    site: "stats.example.org",
    chip: "Free and open source · no account",
    title: 'Your phone,<br>its own <span class="grad">DNS</span>.',
    lead: "Every address looked up at the source, with no middleman receiving the list of your visits.",
  },
};

const feature = (c) => `<!doctype html><html><head><meta charset="utf-8"><style>
@font-face { font-family: Outfit; src: url("/site/assets/fonts/outfit.woff2") format("woff2"); font-weight: 100 900; }
* { box-sizing: border-box; margin: 0; }
body { width: 1024px; height: 500px; overflow: hidden; font-family: Outfit, sans-serif; color: #e9f1f0;
  background:
    radial-gradient(circle at 805px 250px, rgb(44 201 184 / 22%), transparent 300px),
    radial-gradient(rgb(110 231 214 / 9%) 1.5px, transparent 1.6px) 0 0 / 26px 26px,
    #0c1314; }
.text { position: absolute; left: 72px; top: 70px; width: 560px; }
.chip { display: inline-block; padding: 8px 16px; border-radius: 999px; background: #113634; color: #6ee7d6;
  font-size: 19px; font-weight: 600; letter-spacing: 0.01em; }
h1 { margin: 24px 0 20px; font-size: 62px; line-height: 1.04; letter-spacing: -0.025em; font-weight: 700; }
.grad { background: linear-gradient(110deg, #6ee7d6, #2cc9b8); -webkit-background-clip: text; background-clip: text;
  color: transparent; }
p { font-size: 22px; line-height: 1.42; color: #a0b5b6; }
.mark { position: absolute; left: 660px; top: 100px; width: 300px; height: 300px;
  filter: drop-shadow(0 22px 44px rgb(0 0 0 / 45%)); }
</style></head><body>
<div class="text"><span class="chip">${c.chip}</span><h1>${c.title}</h1><p>${c.lead}</p></div>
<img class="mark" src="/site/img/offlok-mark.png" alt="">
</body></html>`;

// The window on Android, its blocking card as Google Play's version shows it: trackers and malicious sites on, and
// "Is this site blocked?" naming a tracker list; strict mode on a network that blocks the root servers.
function playStatus(body) {
  return body.replace('name.includes("pub")', 'name.startsWith("stats.")').replace(
    '"strict-blocked": { state: "strict-blocked", settings: { strict: true } },',
    `"strict-blocked": { state: "strict-blocked", platform: "android", backend: "android",
      settings: { strict: true, blocking: { trackers: true, malicious: true } },
      blocking: { categories: ["trackers", "malicious", "custom"], lists: [
        { category: "trackers", name: "ShadowWhisperer Tracking List", names: 118095 },
        { category: "trackers", name: "HaGeZi's native trackers", names: 21304 },
        { category: "malicious", name: "URLhaus", names: 9214 },
      ] } },`,
  ).replace(
    'match: { category: "ads", source: "AdGuard DNS filter", entry: name }',
    'match: { category: "trackers", source: "ShadowWhisperer Tracking List", entry: name }').replace(
    'android: { state: "direct", platform: "android", backend: "android" },',
    `android: { state: "direct", platform: "android", backend: "android", version: "0.6.4", unbound: "1.26.2",
      network: { servers: ["192.168.1.254"], domains: [] },
      settings: { blocking: { trackers: true, malicious: true } },
      blocking: { categories: ["trackers", "malicious", "custom"], lists: [
        { category: "trackers", name: "ShadowWhisperer Tracking List", names: 118095 },
        { category: "trackers", name: "HaGeZi's native trackers", names: 21304 },
        { category: "malicious", name: "URLhaus", names: 9214 },
      ] } },`,
  );
}

const types = { ".woff2": "font/woff2", ".ttf": "font/ttf", ".png": "image/png", ".webp": "image/webp",
  ".html": "text/html", ".js": "text/javascript", ".css": "text/css" };
const server = http.createServer((req, res) => {
  const url = new URL(req.url, "http://x");
  const lang = url.pathname.match(/^\/feature-(\w+)\.html$/)?.[1];
  if (lang) {
    res.writeHead(200, { "content-type": "text/html" });
    res.end(feature(langs[lang]));
    return;
  }
  const [, top, rest = ""] = url.pathname.match(/^\/(site|app)(\/.*)?$/) || [];
  const base = roots[top];
  const file = base && path.join(base, path.normalize(decodeURIComponent(rest || "/index.html")));
  if (!file || !file.startsWith(base)) {
    res.writeHead(404);
    res.end();
    return;
  }
  fs.readFile(file, (err, data) => {
    if (err) {
      res.writeHead(404);
      res.end();
      return;
    }
    let body = data;
    if (file.endsWith(path.join("frontend", "api.js"))) body = playStatus(data.toString());
    res.writeHead(200, { "content-type": types[path.extname(file)] || "application/octet-stream" });
    res.end(body);
  });
});

server.listen(0, "127.0.0.1", async () => {
  const base = `http://127.0.0.1:${server.address().port}`;
  const browser = await chromium.launch();
  for (const [lang, c] of Object.entries(langs)) {
    const out = path.join(__dirname, "metadata", "android", c.dir, "images");
    fs.mkdirSync(path.join(out, "phoneScreenshots"), { recursive: true });

    const card = await browser.newPage({ viewport: { width: 1024, height: 500 } });
    await card.goto(`${base}/feature-${lang}.html`);
    await card.evaluate(() => document.fonts.ready);
    await card.screenshot({ path: path.join(out, "featureGraphic.png") });
    await card.close();

    // 360 × 640 at 3×: 1080 × 1920. 1: the state and the Quick Settings tile; 2: blocking and the settings; 3: a
    // blocked site checked, dark; 4: strict mode where the root servers are blocked, "Sign in to this network", dark.
    const shots = [
      { scheme: "light", query: "preview=android&tile=ask", at: null },
      { scheme: "light", query: "preview=android&tile=dismissed", at: "section.blocking" },
      { scheme: "dark", query: `preview=android&tile=dismissed&check=${c.site}`, at: "section.blocking" },
      { scheme: "dark", query: "preview=strict-blocked&tile=dismissed", at: null },
    ];
    for (const [i, s] of shots.entries()) {
      const page = await browser.newPage({ viewport: { width: 360, height: 640 }, deviceScaleFactor: 3, colorScheme: s.scheme, locale: c.locale });
      await page.goto(`${base}/app/index.html?${s.query}`);
      await page.waitForTimeout(700);
      if (s.at) await page.locator(s.at).evaluate((e) => window.scrollTo(0, e.getBoundingClientRect().top + window.scrollY - 16));
      await page.screenshot({ path: path.join(out, "phoneScreenshots", `${i + 1}.png`) });
      await page.close();
    }
    console.log(path.relative(root, out));
  }
  await browser.close();
  server.close();
});
