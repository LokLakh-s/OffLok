// The pictures a link to offlok.org shows where it is shared (og:image): 1200 × 630, French and English, in the
// site's dark colours, its font and its headline. No platform named: the card does not age with each release. Into
// site/img/share-<lang>.jpg:
//   NODE_PATH=<node_modules with playwright> node site/share-cards.cjs
const { chromium } = require("playwright");
const http = require("http");
const fs = require("fs");
const path = require("path");

const site = __dirname;
const cards = {
  fr: {
    chip: "Logiciel libre · gratuit, sans compte",
    title: 'L’annuaire d’Internet,<br><span class="grad">chez toi</span>.',
    lead: "OffLok fait de ton ordinateur son propre DNS : plus de fournisseur d’accès, de Google ou de Cloudflare au milieu pour recevoir la liste des sites que tu visites.",
  },
  en: {
    chip: "Free and open source · no account",
    title: 'The internet’s<br>address book,<br><span class="grad">at home</span>.',
    size: 58,
    lead: "OffLok makes your computer its own DNS: no more internet provider, Google or Cloudflare in the middle, receiving the list of every site you visit.",
  },
};

const page = (c) => `<!doctype html><html><head><meta charset="utf-8"><style>
@font-face { font-family: Outfit; src: url("/assets/fonts/outfit.woff2") format("woff2"); font-weight: 100 900; }
* { box-sizing: border-box; margin: 0; }
body { width: 1200px; height: 630px; overflow: hidden; font-family: Outfit, sans-serif; color: #e9f1f0;
  background:
    radial-gradient(circle at 935px 315px, rgb(44 201 184 / 22%), transparent 360px),
    radial-gradient(rgb(110 231 214 / 9%) 1.5px, transparent 1.6px) 0 0 / 28px 28px,
    #0c1314; }
.text { position: absolute; left: 80px; top: 78px; width: 640px; }
.chip { display: inline-block; padding: 9px 18px; border-radius: 999px; background: #113634; color: #6ee7d6;
  font-size: 22px; font-weight: 600; letter-spacing: 0.01em; }
h1 { margin: 30px 0 26px; font-size: 64px; line-height: 1.04; letter-spacing: -0.025em; font-weight: 700; }
.grad { background: linear-gradient(110deg, #6ee7d6, #2cc9b8); -webkit-background-clip: text; background-clip: text;
  color: transparent; }
p { font-size: 25px; line-height: 1.42; color: #a0b5b6; }
.site { position: absolute; left: 80px; bottom: 62px; font-size: 27px; font-weight: 650; color: #6ee7d6;
  letter-spacing: 0.01em; }
.site b { color: #e9f1f0; font-weight: 700; }
.mark { position: absolute; left: 755px; top: 135px; width: 360px; height: 360px;
  filter: drop-shadow(0 24px 50px rgb(0 0 0 / 45%)); }
</style></head><body>
<div class="text"><span class="chip">${c.chip}</span><h1 style="font-size: ${c.size || 64}px">${c.title}</h1><p>${c.lead}</p></div>
<div class="site"><b>OffLok</b> · offlok.org</div>
<img class="mark" src="/img/offlok-mark.png" alt="">
</body></html>`;

const types = { ".woff2": "font/woff2", ".png": "image/png", ".html": "text/html" };
const server = http.createServer((req, res) => {
  const url = new URL(req.url, "http://x");
  const lang = url.pathname.match(/^\/card-(\w+)\.html$/)?.[1];
  if (lang) {
    res.writeHead(200, { "content-type": "text/html" });
    res.end(page(cards[lang]));
    return;
  }
  const file = path.join(site, path.normalize(decodeURIComponent(url.pathname)));
  if (!file.startsWith(site)) {
    res.writeHead(403);
    res.end();
    return;
  }
  fs.readFile(file, (err, data) => {
    if (err) {
      res.writeHead(404);
      res.end();
      return;
    }
    res.writeHead(200, { "content-type": types[path.extname(file)] || "application/octet-stream" });
    res.end(data);
  });
});

server.listen(0, "127.0.0.1", async () => {
  const base = `http://127.0.0.1:${server.address().port}`;
  const browser = await chromium.launch();
  const tab = await browser.newPage({ viewport: { width: 1200, height: 630 } });
  for (const lang of Object.keys(cards)) {
    await tab.goto(`${base}/card-${lang}.html`);
    await tab.evaluate(() => document.fonts.ready);
    await tab.screenshot({ path: path.join(site, "img", `share-${lang}.jpg`), type: "jpeg", quality: 88 });
    console.log(`site/img/share-${lang}.jpg`);
  }
  await browser.close();
  server.close();
});
