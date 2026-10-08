# Authoring guide

How lessons and diagrams are written for DB Mastery. Follow this for every change so the portal stays consistent.

## Lessons

- One file per lesson: `web/lessons/<id>.md`, listed in `web/curriculum.js` (title, level, minutes, summary live there, so lessons start at `##`, never `#`).
- Audience: an experienced AWS Solutions Architect with front-end/Go background, preparing for MongoDB, Databricks and Red Hat SA interviews. Assume smart, not specialist.
- Voice: British English (modelling, behaviour, optimise), second person, plain and direct. Short paragraphs. Explain *why* before *how*. No filler, no hype.
- Depth bar: a principal SA who has read *Designing Data-Intensive Applications* should find nothing wrong and the occasional thing new. Prefer mechanisms and trade-offs over lists of features. Give concrete numbers where they're well established (latency numbers, page sizes, defaults) and say "roughly" when they vary.
- Accuracy over flourish. Don't invent benchmarks, customer names, quotes or dates. Public incidents and company case studies must match their published postmortems/engineering blogs; if unsure of a detail, leave it out or phrase it generally. Product features change: describe capabilities, avoid pricing and "newest" claims.
- Cross-link other lessons with `[text](#/lesson/<id>)`.
- Code: fenced blocks with a language (`sql`, `go`, `js` for mongosh, `python`, `bash`, `yaml`, `text`). Snippets should run against the lab databases where possible (Postgres `dbm` on port 5442, MongoDB on 27017).

### Building blocks

| Markdown | Renders as |
|---|---|
| `> **Key idea:** …`, `> **Tip:** …`, `> **Remember:** …` | green callout |
| `> **Warning:** …`, `> **Gotcha:** …`, `> **Pitfall:** …` | amber callout |
| `> **SA angle:** …`, `> **Interview:** …`, `> **Customer:** …` | purple callout |
| `> **Lab:** …`, `> **Try it:** …` | lab callout |
| `> **Case study:** …`, `> **Real world:** …`, `> **Incident:** …` | teal callout |
| `> **Model answer:** …` (or `**Solution**`, `**Answer**`) | collapsible block, closed by default. Text after the bold label becomes the body; you can add `**Model answer — 2 minutes**` style labels |
| `![Caption sentence.](diagrams/<file>.svg)` on its own line | themed diagram with caption, click to zoom |
| ```` ```quiz ```` | interactive quiz (see below) |

Multi-paragraph callouts: keep every line prefixed with `>` (use a bare `>` for blank lines).

### Quiz format

```text
Q: Question text?
- wrong option
+ the single correct option
- wrong option
- wrong option
? One- or two-sentence explanation of why.
```

Exactly one `+` per question, three or four options, vary the position of the correct answer, and make the wrong options plausible. Aim for 4–6 questions per lesson, ending the lesson.

### Real-world problem template

Use this shape for troubleshooting and "how would you…" content:

```markdown
### Problem: Checkout is slow every day at 9am

> **Real world:** One-paragraph scenario as a customer or interviewer would describe it.

**Symptoms.** What you'd observe (metrics, errors, user reports).

**Diagnose.** The questions you ask and the commands/queries you run, in order.

**Root causes.** The usual suspects, most likely first.

**Fix.** Immediate mitigation, then the durable fix. Show code/config.

**Prevent.** Monitoring, guardrails, design changes.

> **Model answer — what to say in the interview:** A crisp 4–6 sentence answer.
```

## Diagrams

Diagrams are hand-written SVG files in `web/diagrams/`, inlined into the page at runtime so they pick up the site theme (dark and light). The look is Apple-like: calm, flat, generous whitespace, rounded shapes, one idea per diagram.

### File rules (enforced by `make check`)

- Name: `web/diagrams/<lesson-id>-<topic>.svg`.
- Root element exactly: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 760 H">`. Width is always 760; pick H (usually 220–480). No `width`/`height` attributes.
- **ASCII only.** Write every non-ASCII character as a numeric entity: → `&#8594;`, ← `&#8592;`, ↔ `&#8596;`, ↓ `&#8595;`, ↑ `&#8593;`, ⇒ `&#8658;`, ✓ `&#10003;`, ✗ `&#10007;`, · `&#183;`, … `&#8230;`, ≈ `&#8776;`, ≤ `&#8804;`, ≥ `&#8805;`, × `&#215;`, — `&#8212;`, – `&#8211;`, │ `&#9474;`, µ `&#181;`. Escape `<`, `>` and `&` as `&lt;`, `&gt;`, `&amp;`. No emoji.
- **No colours, fonts or `<style>` in the file.** Use only the classes below. No gradients, filters or images.
- Don't define `<marker>`s; use the shared ones: `marker-end="url(#arr)"`, `url(#arr-blue)`, `url(#arr-green)`, `url(#arr-orange)`, `url(#arr-red)`, `url(#arr-purple)`. `marker-start` works too (they auto-reverse).

### Classes

| Element | Classes |
|---|---|
| Shapes (`rect`, `circle`, `path`) | `box` (neutral card) plus optional tint `blue` `green` `orange` `red` `purple` `teal` `gray`; `box ghost` (dashed outline); `box panel` (subtle group background) |
| Solid fills (dots, bars, legend swatches) | `fill-blue` `fill-green` `fill-orange` `fill-red` `fill-purple` `fill-teal` `fill-gray` `fill-surface` `fill-line` |
| Lines (`path`, `line`) | `ln` plus optional `thin` `thick` `dash` `dot` `faint` and a colour `blue` `green` `orange` `red` `purple` `teal` |
| Text | default 13px body; `t-sm` (11px muted), `t-xs` (10px muted), `t-lg` (15px semibold), `t-xl` (19px bold), `t-b` (semibold), `t-mono` (11.5px monospace), `t-muted`, `t-label` (10.5px uppercase section label), `t-on` (white, for text on solid fills), colours `t-blue` `t-green` `t-orange` `t-red` `t-purple` `t-teal` |

### Visual language

- Colour has meaning; use at most three accents per diagram. **Blue** = the main path, primary, request. **Green** = success, healthy, the chosen/fast path. **Orange** = slow, warning, contention. **Red** = failure, conflict, data loss. **Purple** = replication, async, background work. **Teal** = storage, logs, durable state. **Gray** = clients and external actors.
- Corners: `rx="10"`–`"14"` on boxes, `rx="16"` on panels, pills use `rx` = half the height.
- Spacing: at least 16px between boxes and 20px padding inside panels. Align to a grid; centre text with `text-anchor="middle"`; text baseline ≈ box top + box height/2 + 4.
- Text fits: budget about 7px per character at 13px, 6px at 11px and 7px for monospace. Keep labels short (2–4 words), and push explanation into the caption and the prose.
- Prefer left-to-right flow for pipelines and top-to-bottom for hierarchies and timelines. Use curved paths (`C`) for fan-out, straight for main flow.
- Put a short caption in the Markdown that says what to notice: `![Writes go to one leader; followers replay its log.](diagrams/replication-leader.svg)`.
- Replace ASCII-art diagrams in ```` ```text ```` blocks with SVGs when the picture carries the idea. Keep `text` blocks for things that are genuinely text (EXPLAIN output, logs, file layouts).

### Checking your work

```bash
node scripts/check-content.mjs <lesson-id> ...                      # structure, quizzes, diagram rules
node scripts/preview-diagram.mjs web/diagrams/<file>.svg [--light]  # PNG in _preview/ to eyeball
```

Look at every preview: no overlapping text, nothing clipped at the edges, labels inside their boxes, arrows ending at box edges.
