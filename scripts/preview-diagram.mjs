// Renders diagrams with the real site CSS to PNGs for visual review.
//   node scripts/preview-diagram.mjs web/diagrams/foo.svg [more.svg ...] [--light]
// Writes _preview/<name>.png (needs Google Chrome; run outside the sandbox).
import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { basename, join, resolve } from "node:path";

const ROOT = new URL("../", import.meta.url).pathname;
const CHROME = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
const args = process.argv.slice(2);
const theme = args.includes("--light") ? "light" : "dark";
const files = args.filter((a) => !a.startsWith("--"));
const out = join(ROOT, "_preview");
mkdirSync(out, { recursive: true });

const index = readFileSync(join(ROOT, "web/index.html"), "utf8");
const defs = index.match(/<svg class="svg-defs"[\s\S]*?<\/svg>/)[0];
const css = readFileSync(join(ROOT, "web/styles.css"), "utf8");

for (const f of files) {
  const svg = readFileSync(resolve(f), "utf8");
  const name = basename(f, ".svg") + (theme === "light" ? "-light" : "");
  const html = `<!doctype html><html data-theme="${theme}"><head><meta charset="utf-8"><style>${css}
    body{padding:24px;width:820px} .md figure.diagram{margin:0}</style></head>
    <body>${defs}<div class="md"><figure class="diagram"><div class="diagram-body">${svg}</div>
    <figcaption>${name}</figcaption></figure></div></body></html>`;
  const htmlPath = join(out, `${name}.html`);
  writeFileSync(htmlPath, html);
  const png = join(out, `${name}.png`);
  execFileSync(CHROME, [
    "--headless=new", "--disable-gpu", "--hide-scrollbars", "--force-device-scale-factor=1.5",
    `--screenshot=${png}`, "--window-size=870,1000", `file://${htmlPath}`,
  ], { stdio: "pipe" });
  console.log(png);
}
