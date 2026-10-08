// Validates lessons and diagrams: node scripts/check-content.mjs [lesson-id ...]
import { readFileSync, readdirSync, existsSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { join } from "node:path";
import vm from "node:vm";

const WEB = new URL("../web/", import.meta.url).pathname;
const ctx = { window: {} };
vm.runInNewContext(readFileSync(join(WEB, "curriculum.js"), "utf8"), ctx);
const C = ctx.window.CURRICULUM;

const errors = [];
const warn = [];
const err = (f, m) => errors.push(`${f}: ${m}`);

const allIds = C.tracks.flatMap((t) => t.lessons.map((l) => l.id));
const dupes = allIds.filter((id, i) => allIds.indexOf(id) !== i);
if (dupes.length) err("curriculum.js", `duplicate lesson ids: ${dupes.join(", ")}`);
for (const [k, p] of Object.entries(C.paths)) {
  for (const id of p.lessons || []) if (!allIds.includes(id)) err("curriculum.js", `path ${k} references unknown lesson ${id}`);
}

const only = process.argv.slice(2);
const ids = only.length ? only : allIds;
const usedDiagrams = new Set();

for (const id of ids) {
  const f = join(WEB, "lessons", `${id}.md`);
  if (!existsSync(f)) { err(`lessons/${id}.md`, "missing file"); continue; }
  const src = readFileSync(f, "utf8");
  const name = `lessons/${id}.md`;

  const fences = (src.match(/^```/gm) || []).length;
  if (fences % 2) err(name, "unbalanced ``` fences");

  for (const m of src.matchAll(/^!\[([^\]]*)\]\(([^)\s]+)\)\s*$/gm)) {
    const ref = m[2];
    usedDiagrams.add(ref);
    if (!existsSync(join(WEB, ref))) err(name, `figure references missing file ${ref}`);
    if (!m[1].trim()) warn.push(`${name}: figure ${ref} has no caption`);
  }
  for (const m of src.matchAll(/!\[[^\]]*\]\([^)]+\)/g)) {
    const line = src.slice(0, m.index).split("\n").length;
    const full = src.split("\n")[line - 1];
    if (!/^!\[[^\]]*\]\([^)\s]+\)\s*$/.test(full)) err(name, `line ${line}: figures must be on their own line`);
  }

  for (const m of src.matchAll(/^```quiz\n([\s\S]*?)^```/gm)) {
    const qs = m[1].split(/^Q:/m).slice(1);
    qs.forEach((q, i) => {
      const correct = (q.match(/^\s*\+ /gm) || []).length;
      const opts = (q.match(/^\s*[+-] /gm) || []).length;
      if (correct !== 1) err(name, `quiz question ${i + 1} has ${correct} correct answers (need exactly 1)`);
      if (opts < 3) err(name, `quiz question ${i + 1} has only ${opts} options`);
      if (!/^\s*\? /m.test(q)) warn.push(`${name}: quiz question ${i + 1} has no explanation`);
    });
  }
}

const diagDir = join(WEB, "diagrams");
for (const file of existsSync(diagDir) ? readdirSync(diagDir).filter((x) => x.endsWith(".svg")) : []) {
  const p = join(diagDir, file);
  const name = `diagrams/${file}`;
  const buf = readFileSync(p);
  if (buf.some((b) => b > 127)) err(name, "contains non-ASCII bytes; use numeric entities such as &#8594;");
  try {
    execFileSync("xmllint", ["--noout", p], { stdio: "pipe" });
  } catch (e) {
    err(name, `invalid XML: ${String(e.stderr).split("\n")[0]}`);
  }
  const s = buf.toString();
  if (!/<svg[^>]*viewBox=/.test(s)) err(name, "root <svg> needs a viewBox");
  if (/(fill|stroke)="#|(fill|stroke):\s*#|<style/i.test(s)) err(name, "hard-coded colours or <style>; use the theme classes");
  if (/font-family|font-size=/.test(s)) warn.push(`${name}: inline font settings; prefer t-* classes`);
  if (!only.length && !usedDiagrams.has(`diagrams/${file}`)) warn.push(`${name}: not referenced by any lesson`);
}

for (const w of warn) console.log(`warn  ${w}`);
for (const e of errors) console.log(`ERROR ${e}`);
console.log(`\nChecked ${ids.length} lessons. ${errors.length} errors, ${warn.length} warnings.`);
process.exit(errors.length ? 1 : 0);
