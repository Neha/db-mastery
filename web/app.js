(function () {
  const C = window.CURRICULUM;
  const $ = (s, r = document) => r.querySelector(s);
  const main = $("#main");
  const sidebar = $("#sidebar");

  const lessons = [];
  const byId = {};
  for (const t of C.tracks) {
    for (const l of t.lessons) {
      const entry = { ...l, track: t };
      lessons.push(entry);
      byId[l.id] = entry;
    }
  }

  // ---------- State ----------
  const KEY = "dbm:v1";
  const state = Object.assign(
    { done: {}, quiz: {}, notes: {}, focus: "all", theme: "dark" },
    JSON.parse(localStorage.getItem(KEY) || "{}")
  );
  if (!C.paths[state.focus]) state.focus = "all";
  const save = () => localStorage.setItem(KEY, JSON.stringify(state));
  document.documentElement.dataset.theme = state.theme;

  const pathIds = () => {
    const p = C.paths[state.focus];
    return p && p.lessons ? p.lessons.filter((id) => byId[id]) : lessons.map((l) => l.id);
  };
  const inPath = (id) => pathIds().includes(id);
  const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
  const levelBadge = (lvl) => `<span class="badge ${lvl}">${lvl}</span>`;
  const pathsFor = (id) =>
    Object.entries(C.paths).filter(([k, p]) => k !== "all" && p.lessons.includes(id)).map(([k]) => k);

  // ---------- Sidebar ----------
  function updateProgressPill() {
    const ids = pathIds();
    const done = ids.filter((id) => state.done[id]).length;
    const label = C.paths[state.focus].label;
    $("#progressPill").textContent = `${C.paths[state.focus].icon} ${label}: ${done}/${ids.length}`;
  }

  function renderSidebar(activeId) {
    const activeTrack = activeId && byId[activeId] ? byId[activeId].track.id : null;
    let html = `<a class="nav-home ${activeId ? "" : "active"}" href="#/">🏠 Dashboard</a>`;
    for (const t of C.tracks) {
      const done = t.lessons.filter((l) => state.done[l.id]).length;
      const pct = Math.round((done / t.lessons.length) * 100);
      html += `<details class="track" ${t.id === activeTrack ? "open" : ""}>
        <summary><span>${t.icon}</span><span>${esc(t.title)}</span><span class="chev">▶</span></summary>
        <div class="mini-bar"><span style="width:${pct}%"></span></div>
        <ul>${t.lessons
          .map((l) => {
            const cls = [state.done[l.id] ? "done" : "", l.id === activeId ? "active" : "", inPath(l.id) ? "" : "focus-dim"].join(" ");
            return `<li><a class="${cls}" href="#/lesson/${l.id}"><span class="dot">${state.done[l.id] ? "✓" : "○"}</span><span>${esc(l.title)}</span></a></li>`;
          })
          .join("")}</ul>
      </details>`;
    }
    sidebar.innerHTML = html;
    updateProgressPill();
  }

  // ---------- Home ----------
  function renderHome() {
    const ids = pathIds();
    const done = ids.filter((id) => state.done[id]).length;
    const pct = ids.length ? Math.round((done / ids.length) * 100) : 0;
    const remaining = ids.filter((id) => !state.done[id]).reduce((s, id) => s + byId[id].minutes, 0);
    const next = ids.find((id) => !state.done[id]);
    const quizzes = Object.values(state.quiz);
    const qc = quizzes.reduce((s, q) => s + q.score, 0);
    const qt = quizzes.reduce((s, q) => s + q.total, 0);

    const focusBtns = Object.entries(C.paths)
      .map(([k, p]) => `<button type="button" data-focus="${k}" aria-pressed="${state.focus === k}">${p.icon} ${esc(p.label)}</button>`)
      .join("");

    const pathList =
      state.focus === "all"
        ? ""
        : `<h2 class="section-title">${C.paths[state.focus].icon} Your ${esc(C.paths[state.focus].label)} path</h2>
           <ol class="path-list">${ids
             .map((id) => {
               const l = byId[id];
               return `<li><a class="${state.done[id] ? "done" : ""}" href="#/lesson/${id}"><span class="t">${esc(l.title)}</span>
                 <span class="tr">${l.track.icon} ${l.minutes} min</span></a></li>`;
             })
             .join("")}</ol>`;

    const cards = C.tracks
      .map((t) => {
        const d = t.lessons.filter((l) => state.done[l.id]).length;
        const mins = t.lessons.reduce((s, l) => s + l.minutes, 0);
        return `<a class="card" href="#/track/${t.id}">
          <h3><span>${t.icon}</span>${esc(t.title)}</h3>
          <p>${esc(t.summary)}</p>
          <div class="bar"><span style="width:${Math.round((d / t.lessons.length) * 100)}%"></span></div>
          <div class="meta"><span>${d}/${t.lessons.length} lessons</span><span>~${Math.round(mins / 60)} h</span></div>
        </a>`;
      })
      .join("");

    main.innerHTML = `
      <section class="hero">
        <h1>Databases: from beginner to Solutions Architect</h1>
        <p>Theory and internals in the spirit of <em>Designing Data-Intensive Applications</em>, hands-on Go labs against
        real PostgreSQL and MongoDB, the lakehouse, real-world production problems and postmortems, system design,
        and SA interview prep for MongoDB, Databricks and Red Hat.</p>
        <div class="actions">
          ${next ? `<a class="btn primary" href="#/lesson/${next}">${done ? "Continue" : "Start"}: ${esc(byId[next].title)} →</a>` : `<span class="btn success">🎉 Path complete!</span>`}
          <a class="btn" href="#/lesson/setup">🛠️ Lab setup</a>
          <a class="btn" href="#/track/realworld">🚨 Real-world problems</a>
          <a class="btn" href="#/lesson/problem-interview-drill">⚡ Rapid-fire drill</a>
          <a class="btn" href="#/lesson/interview-question-bank">🎯 Interview bank</a>
        </div>
        <div class="stats">
          <div class="stat"><b>${pct}%</b><span>of current path complete</span></div>
          <div class="stat"><b>${done}/${ids.length}</b><span>lessons done</span></div>
          <div class="stat"><b>${(remaining / 60).toFixed(1)} h</b><span>estimated time remaining</span></div>
          <div class="stat"><b>${qt ? Math.round((qc / qt) * 100) + "%" : "—"}</b><span>quiz accuracy (${qt} answered)</span></div>
        </div>
      </section>
      <h2 class="section-title">🎯 Choose your focus</h2>
      <div class="focus-picker">${focusBtns}</div>
      ${pathList}
      <h2 class="section-title">📚 Tracks</h2>
      <div class="grid">${cards}</div>`;

    main.querySelectorAll("[data-focus]").forEach((b) =>
      b.addEventListener("click", () => {
        state.focus = b.dataset.focus;
        save();
        route();
      })
    );
    renderSidebar(null);
  }

  // ---------- Track ----------
  function renderTrack(id) {
    const t = C.tracks.find((x) => x.id === id);
    if (!t) return renderHome();
    main.innerHTML = `
      <div class="crumbs"><a href="#/">Dashboard</a> / ${esc(t.title)}</div>
      <h1>${t.icon} ${esc(t.title)}</h1>
      <p style="color:var(--muted);max-width:760px">${esc(t.summary)}</p>
      <div class="grid">${t.lessons
        .map(
          (l) => `<a class="card" href="#/lesson/${l.id}">
            <h3>${state.done[l.id] ? "✅" : "📘"} ${esc(l.title)}</h3>
            <p>${esc(l.summary)}</p>
            <div class="meta">${levelBadge(l.level)}<span>${l.minutes} min</span></div>
          </a>`
        )
        .join("")}</div>`;
    renderSidebar(t.lessons[0].id);
  }

  // ---------- Lesson ----------
  const mdCache = {};
  async function loadLesson(id) {
    if (!mdCache[id]) {
      const res = await fetch(`lessons/${id}.md`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      mdCache[id] = await res.text();
    }
    return mdCache[id];
  }

  const svgCache = {};
  function hydrateDiagrams(root) {
    root.querySelectorAll("figure.diagram[data-src]").forEach(async (fig) => {
      const src = fig.dataset.src;
      const body = fig.querySelector(".diagram-body");
      try {
        if (!svgCache[src]) {
          const res = await fetch(src);
          if (!res.ok) throw new Error(`HTTP ${res.status}`);
          svgCache[src] = await res.text();
        }
        body.innerHTML = svgCache[src];
        const svg = body.querySelector("svg");
        if (svg) {
          svg.setAttribute("role", "img");
          svg.setAttribute("aria-label", fig.querySelector("figcaption")?.textContent || "Diagram");
          svg.removeAttribute("width");
          svg.removeAttribute("height");
        }
      } catch (e) {
        body.innerHTML = `<p class="diagram-missing">Diagram unavailable (${esc(src)})</p>`;
      }
    });
  }

  function openLightbox(fig) {
    const box = document.createElement("div");
    box.className = "lightbox";
    box.setAttribute("role", "dialog");
    box.setAttribute("aria-modal", "true");
    box.innerHTML = `<div class="lightbox-inner">${fig.innerHTML}</div><button class="icon-btn lightbox-close" type="button" aria-label="Close">✕</button>`;
    const close = () => { box.remove(); document.removeEventListener("keydown", onKey); };
    const onKey = (e) => { if (e.key === "Escape") close(); };
    box.addEventListener("click", (e) => { if (e.target === box || e.target.closest(".lightbox-close")) close(); });
    document.addEventListener("keydown", onKey);
    document.body.appendChild(box);
    box.querySelector(".lightbox-close").focus();
  }

  async function renderLesson(id) {
    const l = byId[id];
    if (!l) return renderHome();
    renderSidebar(id);
    main.innerHTML = `<p style="color:var(--muted)">Loading…</p>`;
    let src;
    try {
      src = await loadLesson(id);
    } catch (e) {
      main.innerHTML = `<p>Could not load lesson <code>${esc(id)}</code>: ${esc(e.message)}</p>`;
      return;
    }
    const toc = [];
    const body = Markdown.render(src, toc);
    const order = pathIds().includes(id) ? pathIds() : lessons.map((x) => x.id);
    const idx = order.indexOf(id);
    const prev = idx > 0 ? byId[order[idx - 1]] : null;
    const next = idx < order.length - 1 ? byId[order[idx + 1]] : null;
    const best = state.quiz[id];
    const paths = pathsFor(id).map((k) => `<span class="badge ${k}">${C.paths[k].icon} ${C.paths[k].label}</span>`).join(" ");

    main.innerHTML = `
      <div class="lesson-wrap">
        <article>
          <div class="crumbs"><a href="#/">Dashboard</a> / <a href="#/track/${l.track.id}">${esc(l.track.title)}</a></div>
          <header class="lesson-head">
            <h1>${esc(l.title)}</h1>
            <div class="tags">${levelBadge(l.level)} <span>⏱ ${l.minutes} min</span> ${paths}</div>
          </header>
          <div class="md">${body}</div>
          <div class="lesson-actions">
            <button class="btn ${state.done[id] ? "success" : "primary"}" id="doneBtn" type="button">
              ${state.done[id] ? "✓ Completed" : "Mark as complete"}</button>
            ${best ? `<span class="badge">Best quiz score: ${best.score}/${best.total}</span>` : ""}
          </div>
          <div class="notes">
            <label for="notes"><b>📝 My notes</b> <small id="notesSaved"></small></label>
            <textarea id="notes" placeholder="Write what you learned, questions to research, or how you'd explain this to a customer…">${esc(state.notes[id] || "")}</textarea>
          </div>
          <nav class="pager">
            ${prev ? `<a href="#/lesson/${prev.id}"><small>← Previous</small>${esc(prev.title)}</a>` : "<span></span>"}
            ${next ? `<a class="next" href="#/lesson/${next.id}"><small>Next →</small>${esc(next.title)}</a>` : "<span></span>"}
          </nav>
        </article>
        <aside class="toc">${
          toc.length
            ? `<h4>On this page</h4>${toc.map((h) => `<a class="h${h.level}" href="#" data-anchor="${h.id}">${esc(h.text)}</a>`).join("")}`
            : ""
        }</aside>
      </div>`;

    hydrateDiagrams(main);

    $("#doneBtn").addEventListener("click", () => {
      state.done[id] = !state.done[id];
      if (!state.done[id]) delete state.done[id];
      save();
      renderLesson(id).then(() => { if (state.done[id] && next) location.hash = `#/lesson/${next.id}`; });
    });

    let t;
    $("#notes").addEventListener("input", (e) => {
      clearTimeout(t);
      t = setTimeout(() => {
        state.notes[id] = e.target.value;
        save();
        $("#notesSaved").textContent = "saved";
      }, 400);
    });

    main.querySelectorAll("[data-anchor]").forEach((a) =>
      a.addEventListener("click", (e) => {
        e.preventDefault();
        document.getElementById(a.dataset.anchor)?.scrollIntoView({ behavior: "smooth", block: "start" });
      })
    );
    window.scrollTo(0, 0);
  }

  // ---------- Delegated interactions: copy + quizzes ----------
  main.addEventListener("click", (e) => {
    const copy = e.target.closest(".copy");
    if (copy) {
      const code = copy.parentElement.querySelector("code").innerText;
      navigator.clipboard?.writeText(code).then(() => {
        copy.textContent = "Copied!";
        setTimeout(() => (copy.textContent = "Copy"), 1200);
      });
      return;
    }
    const fig = e.target.closest("figure.diagram");
    if (fig && fig.querySelector("svg, img")) { openLightbox(fig); return; }
    const btn = e.target.closest(".q-opts button");
    if (!btn || btn.disabled) return;
    const q = btn.closest(".q");
    const quiz = btn.closest(".quiz");
    const correct = btn.dataset.correct === "true";
    q.querySelectorAll("button").forEach((b) => {
      b.disabled = true;
      if (b.dataset.correct === "true") b.classList.add("correct");
    });
    if (!correct) btn.classList.add("wrong");
    q.dataset.answered = correct ? "1" : "0";
    q.querySelector(".q-explain").hidden = false;

    const total = +quiz.dataset.total;
    const answered = quiz.querySelectorAll(".q[data-answered]");
    const score = [...answered].filter((x) => x.dataset.answered === "1").length;
    const scoreEl = quiz.querySelector(".score");
    if (answered.length < total) {
      scoreEl.textContent = `${score}/${answered.length} correct so far · ${total - answered.length} to go`;
      return;
    }
    scoreEl.innerHTML = `<b>Score: ${score}/${total}</b> ${score === total ? "🎉 Perfect!" : score / total >= 0.7 ? "👍 Solid." : "📖 Worth a re-read."}`;
    const id = (location.hash.match(/^#\/lesson\/(.+)$/) || [])[1];
    if (id) {
      const prev = state.quiz[id];
      if (!prev || score / total >= prev.score / prev.total) state.quiz[id] = { score, total };
      save();
    }
  });

  // ---------- Search ----------
  const search = $("#search");
  const results = $("#searchResults");
  let indexLoaded = false;
  let sel = -1;

  async function ensureIndex() {
    if (indexLoaded) return;
    indexLoaded = true;
    await Promise.all(lessons.map((l) => loadLesson(l.id).catch(() => "")));
  }

  function doSearch(q) {
    q = q.trim().toLowerCase();
    if (q.length < 2) { results.hidden = true; return; }
    const terms = q.split(/\s+/);
    const hits = [];
    for (const l of lessons) {
      const title = l.title.toLowerCase();
      const meta = (l.summary + " " + l.track.title).toLowerCase();
      const text = (mdCache[l.id] || "").toLowerCase();
      let score = 0;
      for (const term of terms) {
        if (title.includes(term)) score += 10;
        if (meta.includes(term)) score += 5;
        const occ = text.split(term).length - 1;
        if (occ) score += 1 + Math.min(occ, 20) / 4;
        else if (!title.includes(term) && !meta.includes(term)) { score = 0; break; }
      }
      if (score > 0) {
        const at = text.indexOf(terms[0]);
        let snippet = at >= 0 ? (mdCache[l.id] || "").slice(Math.max(0, at - 50), at + 90).replace(/[#`*>|]/g, "").replace(/\s+/g, " ") : l.summary;
        snippet = esc(snippet).replace(new RegExp(terms.map((t) => t.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")).join("|"), "gi"), (m) => `<mark>${m}</mark>`);
        hits.push({ l, score, snippet });
      }
    }
    hits.sort((a, b) => b.score - a.score);
    sel = -1;
    results.innerHTML = hits.length
      ? hits.slice(0, 10).map((h) => `<a href="#/lesson/${h.l.id}">${h.l.track.icon} ${esc(h.l.title)}<small>…${h.snippet}…</small></a>`).join("")
      : `<a>No results for “${esc(q)}”</a>`;
    results.hidden = false;
  }

  search.addEventListener("focus", () => ensureIndex().then(() => search.value && doSearch(search.value)));
  search.addEventListener("input", () => ensureIndex().then(() => doSearch(search.value)));
  search.addEventListener("keydown", (e) => {
    const items = [...results.querySelectorAll("a[href]")];
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      if (!items.length) return;
      sel = (sel + (e.key === "ArrowDown" ? 1 : -1) + items.length) % items.length;
      items.forEach((a, i) => a.classList.toggle("active", i === sel));
    } else if (e.key === "Enter") {
      const target = items[sel >= 0 ? sel : 0];
      if (target) { location.hash = target.getAttribute("href"); closeSearch(); }
    } else if (e.key === "Escape") closeSearch();
  });
  results.addEventListener("click", (e) => { if (e.target.closest("a[href]")) closeSearch(); });
  document.addEventListener("click", (e) => { if (!e.target.closest(".search-wrap")) results.hidden = true; });
  function closeSearch() { results.hidden = true; search.value = ""; search.blur(); }
  document.addEventListener("keydown", (e) => {
    if (e.key === "/" && !/INPUT|TEXTAREA/.test(document.activeElement.tagName)) { e.preventDefault(); search.focus(); }
  });

  // ---------- Theme + mobile menu ----------
  $("#themeBtn").addEventListener("click", () => {
    state.theme = state.theme === "dark" ? "light" : "dark";
    document.documentElement.dataset.theme = state.theme;
    save();
  });
  const menuBtn = $("#menuBtn");
  menuBtn.addEventListener("click", () => {
    const open = sidebar.classList.toggle("open");
    menuBtn.setAttribute("aria-expanded", open);
  });

  // ---------- Router ----------
  function route() {
    sidebar.classList.remove("open");
    menuBtn.setAttribute("aria-expanded", "false");
    const h = location.hash;
    let m;
    if ((m = h.match(/^#\/lesson\/([\w-]+)/))) return renderLesson(m[1]);
    if ((m = h.match(/^#\/track\/([\w-]+)/))) return renderTrack(m[1]);
    renderHome();
  }
  window.addEventListener("hashchange", route);
  route();
})();
