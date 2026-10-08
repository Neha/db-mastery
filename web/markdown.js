// Minimal Markdown renderer tailored to the lesson files.
// Supports: headings, paragraphs, nested lists, fenced code (with highlighting),
// tables, blockquote callouts, hr, inline code/bold/italic/links, ```quiz blocks,
// figures (`![caption](diagrams/x.svg)` — SVGs are inlined by app.js so they follow the theme)
// and collapsible answers (a blockquote starting with **Model answer** / **Solution**).
(function () {
  const esc = (s) =>
    s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");

  const slug = (s) =>
    s.toLowerCase().replace(/<[^>]+>/g, "").replace(/[^\w\s-]/g, "").trim().replace(/\s+/g, "-");

  // ---------- Syntax highlighting ----------
  const words = (s) => new Set(s.split(/\s+/));
  const KW = {
    go: words(`break case chan const continue default defer else fallthrough for func go goto if import
      interface map package range return select struct switch type var nil true false iota error string int
      int64 int32 uint64 byte bool float64 any make new len cap append panic recover`),
    sql: words(`select from where insert into values update set delete create table index unique primary key
      foreign references join inner left right full outer on group by order having limit offset as and or not
      null is in exists between like begin commit rollback transaction isolation level read committed
      repeatable serializable explain analyze with recursive over partition window distinct count sum avg min
      max case when then else end union all alter add drop constraint default int integer bigint text varchar
      timestamp timestamptz numeric boolean serial bigserial jsonb using gin btree desc asc returning true false
      merge matched vacuum grant to cascade if replace view materialized`),
    js: words(`const let var function return if else for while of in new await async true false null undefined
      import export from class this db`),
    python: words(`def return if else elif for while in import from as class with try except finally lambda
      None True False and or not is pass yield`),
    bash: words(`if then else fi for do done case esac function export echo cd docker go git curl make kubectl oc helm`),
    yaml: words(`true false null`),
  };
  KW.ts = KW.js;
  KW.javascript = KW.js;
  KW.sh = KW.bash;
  KW.shell = KW.bash;
  KW.py = KW.python;
  KW.yml = KW.yaml;
  KW.scala = KW.js;

  const reEsc = (s) => s.replace(/[.*+?^${}()|[\]\\/]/g, "\\$&");

  function highlight(code, lang) {
    const kw = KW[lang];
    if (!kw) return esc(code);
    const hashCom = ["bash", "sh", "shell", "yaml", "yml", "python", "py"].includes(lang);
    const lineCom = lang === "sql" ? "--" : hashCom ? "#" : "//";
    const block = hashCom ? "" : "|/\\*[\\s\\S]*?\\*/";
    const re = new RegExp(
      `(${reEsc(lineCom)}.*$${block})` +
        `|("(?:\\\\.|[^"\\\\\\n])*"|'(?:\\\\.|[^'\\\\\\n])*'|\`[^\`]*\`)` +
        `|\\b(\\d+(?:\\.\\d+)?)\\b` +
        `|\\b([A-Za-z_]\\w*)\\b(?=(\\s*\\()?)`,
      "gm"
    );
    let out = "";
    let last = 0;
    let m;
    while ((m = re.exec(code))) {
      out += esc(code.slice(last, m.index));
      const [tok, com, str, num, ident, call] = m;
      if (com) out += `<span class="tok-com">${esc(tok)}</span>`;
      else if (str) out += `<span class="tok-str">${esc(tok)}</span>`;
      else if (num) out += `<span class="tok-num">${tok}</span>`;
      else if (ident) {
        const k = lang === "sql" ? ident.toLowerCase() : ident;
        if (kw.has(k)) out += `<span class="tok-kw">${ident}</span>`;
        else if (call !== undefined) out += `<span class="tok-fn">${ident}</span>`;
        else out += ident;
      }
      last = m.index + tok.length;
      if (tok.length === 0) re.lastIndex++;
    }
    return out + esc(code.slice(last));
  }

  // ---------- Inline ----------
  function inline(s) {
    const codes = [];
    s = s.replace(/`([^`]+)`/g, (_, c) => {
      codes.push(c);
      return `\u0000${codes.length - 1}\u0000`;
    });
    s = esc(s);
    s = s.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
    s = s.replace(/(^|[^*\w])\*([^*\s][^*]*?)\*(?!\w)/g, "$1<em>$2</em>");
    s = s.replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, (_, text, url) => {
      const ext = /^https?:/.test(url);
      return `<a href="${url}"${ext ? ' target="_blank" rel="noopener"' : ""}>${text}</a>`;
    });
    return s.replace(/\u0000(\d+)\u0000/g, (_, i) => `<code>${esc(codes[+i])}</code>`);
  }

  // ---------- Quiz ----------
  function parseQuiz(lines) {
    const qs = [];
    let cur = null;
    for (const raw of lines) {
      const line = raw.trim();
      if (!line) continue;
      if (line.startsWith("Q:")) {
        cur = { q: line.slice(2).trim(), opts: [], explain: "" };
        qs.push(cur);
      } else if (!cur) continue;
      else if (line.startsWith("+ ")) cur.opts.push({ text: line.slice(2), correct: true });
      else if (line.startsWith("- ")) cur.opts.push({ text: line.slice(2), correct: false });
      else if (line.startsWith("? ")) cur.explain += (cur.explain ? " " : "") + line.slice(2);
    }
    return qs;
  }

  let quizCounter = 0;
  function renderQuiz(lines) {
    const qs = parseQuiz(lines);
    const id = `quiz-${quizCounter++}`;
    const body = qs
      .map(
        (q, qi) => `
      <div class="q" data-q="${qi}">
        <div class="q-text">${qi + 1}. ${inline(q.q)}</div>
        <div class="q-opts">${q.opts
          .map((o) => `<button type="button" data-correct="${o.correct}">${inline(o.text)}</button>`)
          .join("")}</div>
        <div class="q-explain" hidden>${q.explain ? `<b>Why:</b> ${inline(q.explain)}` : ""}</div>
      </div>`
      )
      .join("");
    return `<section class="quiz" id="${id}" data-total="${qs.length}">
      <h3>🧠 Check your understanding</h3>
      <div class="score">Answer all ${qs.length} questions</div>${body}</section>`;
  }

  // ---------- Blocks ----------
  const isListLine = (l) => /^\s*([-*]|\d+\.)\s+/.test(l);
  const FIGURE = /^!\[([^\]]*)\]\(([^)\s]+)\)\s*$/;
  const REVEAL = /^\*\*(Model answer|Answer|Solution|Show answer)([^*]*)\*\*:?\s*/i;
  const isBlockStart = (l) =>
    /^```/.test(l) || /^#{1,4}\s/.test(l) || /^>/.test(l) || /^---+\s*$/.test(l) || isListLine(l) || /^\|/.test(l) ||
    FIGURE.test(l);

  function buildList(items, i) {
    const { indent, ordered } = items[i];
    const tag = ordered ? "ol" : "ul";
    let html = `<${tag}>`;
    while (i < items.length && items[i].indent >= indent) {
      if (items[i].indent > indent) {
        const r = buildList(items, i);
        html = html.replace(/<\/li>$/, r.html + "</li>");
        i = r.i;
        continue;
      }
      const text = items[i].text.replace(/^\[ \]\s*/, "☐ ").replace(/^\[x\]\s*/i, "☑ ");
      html += `<li>${inline(text)}</li>`;
      i++;
    }
    return { html: html + `</${tag}>`, i };
  }

  function splitRow(line) {
    return line.trim().replace(/^\|/, "").replace(/\|$/, "").split("|").map((c) => c.trim());
  }

  function render(src, toc) {
    const lines = src.replace(/\r\n/g, "\n").split("\n");
    let html = "";
    let i = 0;
    while (i < lines.length) {
      const line = lines[i];

      if (!line.trim()) { i++; continue; }

      const fence = line.match(/^```\s*([\w+-]*)\s*$/);
      if (fence) {
        const lang = fence[1].toLowerCase();
        const body = [];
        i++;
        while (i < lines.length && !/^```\s*$/.test(lines[i])) body.push(lines[i++]);
        i++;
        if (lang === "quiz") { html += renderQuiz(body); continue; }
        const code = body.join("\n");
        html += `<pre>${lang ? `<span class="lang">${esc(lang)}</span>` : ""}<button class="copy" type="button">Copy</button><code>${highlight(code, lang)}</code></pre>`;
        continue;
      }

      const h = line.match(/^(#{1,4})\s+(.*)$/);
      if (h) {
        const level = Math.max(2, h[1].length);
        const text = inline(h[2]);
        const id = slug(h[2]);
        if (toc && level <= 3) toc.push({ id, text: h[2].replace(/[`*]/g, ""), level });
        html += `<h${level} id="${id}">${text}</h${level}>`;
        i++;
        continue;
      }

      if (/^---+\s*$/.test(line)) { html += "<hr>"; i++; continue; }

      const fig = line.match(FIGURE);
      if (fig) {
        const [, caption, src] = fig;
        const isSvg = /\.svg$/i.test(src);
        html += `<figure class="diagram"${isSvg ? ` data-src="${esc(src)}"` : ""}>
          <div class="diagram-body">${isSvg ? "" : `<img src="${esc(src)}" alt="${esc(caption)}" loading="lazy">`}</div>
          ${caption ? `<figcaption>${inline(caption)}</figcaption>` : ""}</figure>`;
        i++;
        continue;
      }

      if (/^\|/.test(line) && i + 1 < lines.length && /^\|?\s*:?-{2,}/.test(lines[i + 1])) {
        const head = splitRow(line);
        i += 2;
        const rows = [];
        while (i < lines.length && /^\|/.test(lines[i])) rows.push(splitRow(lines[i++]));
        html += `<table><thead><tr>${head.map((c) => `<th>${inline(c)}</th>`).join("")}</tr></thead><tbody>${rows
          .map((r) => `<tr>${r.map((c) => `<td>${inline(c)}</td>`).join("")}</tr>`)
          .join("")}</tbody></table>`;
        continue;
      }

      if (/^>/.test(line)) {
        const body = [];
        while (i < lines.length && /^>/.test(lines[i])) body.push(lines[i++].replace(/^>\s?/, ""));
        const first = body[0] || "";
        const reveal = first.match(REVEAL);
        if (reveal) {
          body[0] = first.slice(reveal[0].length);
          const label = (reveal[1] + reveal[2]).trim();
          html += `<details class="reveal"><summary>${inline(label)}</summary><div>${render(body.join("\n"))}</div></details>`;
          continue;
        }
        let cls = "";
        if (/^\*\*(Tip|Remember|Key idea)/i.test(first)) cls = "tip";
        else if (/^\*\*(Warning|Gotcha|Pitfall|Note)/i.test(first)) cls = "warn";
        else if (/^\*\*(SA angle|Interview|Customer)/i.test(first)) cls = "sa";
        else if (/^\*\*(Lab|Exercise|Try it|Hands-on)/i.test(first)) cls = "lab";
        else if (/^\*\*(Case study|Real world|Incident|Postmortem|War story)/i.test(first)) cls = "case";
        html += `<blockquote class="${cls}">${render(body.join("\n"))}</blockquote>`;
        continue;
      }

      if (isListLine(line)) {
        const items = [];
        while (i < lines.length) {
          const l = lines[i];
          const m = l.match(/^(\s*)([-*]|\d+\.)\s+(.*)$/);
          if (m) {
            items.push({ indent: m[1].length, ordered: /\d/.test(m[2]), text: m[3] });
            i++;
          } else if (l.trim() && /^\s+/.test(l) && items.length) {
            items[items.length - 1].text += " " + l.trim();
            i++;
          } else break;
        }
        html += buildList(items, 0).html;
        continue;
      }

      const para = [lines[i++]];
      while (i < lines.length && lines[i].trim() && !isBlockStart(lines[i])) para.push(lines[i++]);
      html += `<p>${inline(para.join(" "))}</p>`;
    }
    return html;
  }

  window.Markdown = { render, highlight, slug };
})();
