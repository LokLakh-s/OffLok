// The website's pictures of the window: its ?preview= mode, the "blocking" example without the failed custom list
// and the trusted sites, cropped after the blocking card. The site does not show ad blocking (the Google Play version
// has none, and Google reviews the site): the "Ads" row is left out. Serve desktop/frontend/ over HTTP, then:
//   NODE_PATH=<node_modules with playwright> node site/app-shots.cjs http://127.0.0.1:<port> <folder>
// and turn the PNGs into site/img/app-<lang>-<scheme>.webp (site/build.sh does not).
const { chromium } = require("playwright");
const [base, out] = process.argv.slice(2);
(async () => {
  const browser = await chromium.launch();
  for (const scheme of ["light", "dark"]) {
    for (const locale of ["fr-FR", "en-US"]) {
      const page = await browser.newPage({ viewport: { width: 440, height: 900 }, deviceScaleFactor: 2, colorScheme: scheme, locale });
      await page.route("**/api.js", async (route) => {
        const res = await route.fetch();
        let body = await res.text();
        body = body
          .replace('blocking: { ads: true, trackers: true, custom: true }', 'blocking: { trackers: true, malicious: true }')
          .replace(/\{ category: "ads"[^}]*\},/, "")
          .replace(/trusted: \[[^\]]*\],/, "trusted: [],")
          .replace(/custom_lists: \[[^\]]*\],/, "custom_lists: [],")
          .replace(/\{ category: "custom"[^}]*\},/, '{ category: "malicious", name: "URLhaus", names: 9214 },');
        await route.fulfill({ response: res, body });
      });
      await page.route("**/main.js", async (route) => {
        const res = await route.fetch();
        const body = (await res.text()).replace('const categories = ["ads", ', "const categories = [");
        await route.fulfill({ response: res, body });
      });
      await page.goto(`${base}/index.html?preview=blocking`);
      await page.waitForTimeout(700);
      const box = await page.locator("section.blocking").boundingBox();
      const lang = locale.slice(0, 2);
      await page.screenshot({ path: `${out}/app-${lang}-${scheme}.png`, clip: { x: 0, y: 0, width: 440, height: Math.ceil(box.y + box.height + 8) } });
      await page.close();
    }
  }
  await browser.close();
})();
