// Housenka web UI: selection, keyboard shortcuts (like the TUI), modals,
// confirmations, copying and the theme picker. Everything else is HTMX.
(() => {
  "use strict";

  let selectedId = null;
  let yankedId = null;
  let pending = ""; // first key of a two-key sequence (gg, dd, yy)
  let pendingTimer = null;

  const $ = (sel, root = document) => root.querySelector(sel);
  const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];
  const rows = () => $$("#main .entry");
  const selectedRow = () => rows().find((r) => r.dataset.id === selectedId) || null;
  const openDialog = () => $("dialog[open]");

  // --- selection -------------------------------------------------------------

  function select(row, scroll = true) {
    rows().forEach((r) => r.classList.toggle("selected", r === row));
    selectedId = row ? row.dataset.id : null;
    if (row && scroll) row.scrollIntoView({ block: "nearest" });
  }

  function restoreSelection() {
    const all = rows();
    select(all.find((r) => r.dataset.id === selectedId) || all[0] || null, false);
  }

  function move(delta) {
    const all = rows();
    if (!all.length) return;
    const i = all.findIndex((r) => r.dataset.id === selectedId);
    select(all[Math.max(0, Math.min(all.length - 1, (i < 0 ? 0 : i + delta)))]);
  }

  document.addEventListener("click", (e) => {
    const row = e.target.closest(".entry");
    if (row) select(row, false);
    // Close "more" menus when clicking elsewhere or on a menu item.
    $$("details.more[open]").forEach((d) => {
      if (!d.contains(e.target) || e.target.closest(".menu")) d.open = false;
    });
  });

  document.addEventListener("dblclick", (e) => {
    const row = e.target.closest(".entry");
    if (row && !e.target.closest("a, button, summary")) clickKey(row, "e");
  });

  // The server names the entry to select after saving (HX-Trigger).
  document.addEventListener("housenka:select", (e) => { selectedId = e.detail.value; });

  // --- modal -------------------------------------------------------------------

  document.addEventListener("htmx:afterSwap", (e) => {
    if (e.detail.target.id === "modal") {
      const dlg = $("#modal dialog");
      if (dlg && !dlg.open) {
        dlg.showModal();
        const bad = $(".hint.err", dlg);
        const focus = (bad && $("input:not([type=hidden]):not(.picker), textarea", bad.closest(".field"))) || $("[autofocus]", dlg);
        if (focus) {
          focus.focus();
          if (focus.setSelectionRange && focus.value) focus.setSelectionRange(focus.value.length, focus.value.length);
        }
      }
    }
  });

  document.addEventListener("htmx:afterSettle", () => restoreSelection());

  // A closed modal is removed so the next one starts fresh.
  document.addEventListener("close", (e) => {
    if (e.target.closest && e.target.closest("#modal")) $("#modal").innerHTML = "";
  }, true);

  document.addEventListener("click", (e) => {
    const close = e.target.closest("[data-close]");
    if (close) close.closest("dialog").close();
    // Clicking the backdrop closes a modal too.
    if (e.target.matches("#modal dialog")) {
      const r = e.target.getBoundingClientRect();
      if (e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom) e.target.close();
    }
  });

  // --- confirmation ------------------------------------------------------------------

  document.addEventListener("htmx:confirm", (e) => {
    if (!e.detail.question) return;
    e.preventDefault();
    const dlg = $("#confirm");
    $("#confirm-text").textContent = e.detail.question;
    dlg.returnValue = "";
    dlg.addEventListener("close", () => {
      if (dlg.returnValue === "yes") e.detail.issueRequest(true);
    }, { once: true });
    dlg.showModal();
  });

  // --- toast ---------------------------------------------------------------------------

  let toastTimer = null;
  function scheduleToastHide() {
    clearTimeout(toastTimer);
    const t = $("#toast .toast");
    if (!t) return;
    toastTimer = setTimeout(() => t.remove(), $("a", t) ? 12000 : 5000);
  }
  document.addEventListener("htmx:oobAfterSwap", (e) => {
    if (e.detail.target.id === "toast") scheduleToastHide();
  });
  document.addEventListener("click", (e) => {
    if (e.target.closest(".toast-close")) e.target.closest(".toast").remove();
  });

  function toast(text, err = false) {
    const box = $("#toast");
    box.innerHTML = "";
    const t = document.createElement("div");
    t.className = "toast" + (err ? " err" : "");
    t.setAttribute("role", "status");
    t.textContent = text;
    box.appendChild(t);
    scheduleToastHide();
  }

  document.addEventListener("htmx:responseError", (e) => {
    const text = (e.detail.xhr.responseText || "").trim() || "Chyba " + e.detail.xhr.status;
    toast(text, true);
  });
  document.addEventListener("htmx:sendError", () => toast("Housenka neodpovídá – běží ještě server?", true));

  // --- copying -------------------------------------------------------------------------------

  // The Clipboard API needs a secure context (localhost, *.localhost);
  // names from the hosts file such as housenka.local fall back to execCommand.
  function legacyCopy(text) {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    (openDialog() || document.body).appendChild(ta);
    ta.select();
    const ok = document.execCommand("copy");
    ta.remove();
    if (!ok) throw new Error("copy failed");
  }

  async function copy(text, label) {
    try {
      if (navigator.clipboard && window.isSecureContext) await navigator.clipboard.writeText(text);
      else legacyCopy(text);
      toast((label || "Text") + " zkopírován do schránky");
    } catch {
      toast("Kopírování se nepovedlo", true);
    }
  }

  document.addEventListener("click", (e) => {
    const c = e.target.closest("[data-copy]");
    if (c) copy(c.dataset.copy, c.dataset.copyLabel);
    const y = e.target.closest("[data-yank]");
    if (y) yank(y.dataset.yank);
  });

  function yank(id) {
    yankedId = id;
    toast("Záznam zapamatován – p vloží kopii na vybraný den");
  }

  function paste() {
    if (!yankedId) return toast("Nejdřív si záznam zapamatuj (yy)", true);
    const row = selectedRow();
    const date = row ? row.dataset.date : $("#viewstate [name=v_date]").value;
    htmx.ajax("GET", `/entries/new?copy=${encodeURIComponent(yankedId)}&date=${date}`, { target: "#modal" });
  }

  // --- theme -------------------------------------------------------------------------------------

  document.addEventListener("change", (e) => {
    if (e.target.id !== "theme") return;
    document.documentElement.dataset.theme = e.target.value;
    document.cookie = `housenka_theme=${e.target.value}; path=/; max-age=31536000; SameSite=Strict`;
  });

  // --- form ----------------------------------------------------------------------------------------

  document.addEventListener("change", (e) => {
    if (!e.target.matches(".picker")) return;
    const input = $("#f-date");
    if (input && e.target.value) {
      input.value = e.target.value;
      input.dispatchEvent(new Event("input", { bubbles: true }));
      input.focus();
    }
  });

  // --- settings: project form -------------------------------------------------------

  const colorNames = ["red", "orange", "yellow", "green", "teal", "cyan", "blue", "purple", "pink"];

  // updateProjectForm keeps the badge preview, the rename note and the
  // default activity choices in line with what is filled in.
  function updateProjectForm(form) {
    const code = $("[name=code]", form).value.trim();
    const old = $("[name=old]", form).value;
    const preview = $("#color-preview", form);
    const picked = $("[name=color]:checked", form);
    if (preview) {
      preview.textContent = code || "KÓD";
      preview.className = "badge proj preview";
      preview.style.removeProperty("--pc");
      let color = picked ? picked.value : "";
      if (color === "") color = picked && picked.dataset.derived ? picked.dataset.derived : "";
      if (color === "custom") preview.style.setProperty("--pc", $("[name=custom]", form).value);
      else if (colorNames.includes(color)) preview.classList.add("pc-" + color);
    }
    const note = $("[data-rename-note]", form);
    if (note) note.hidden = !old || code === old;
    const select = $("[name=default_activity]", form);
    if (!select) return; // the status form has a color only
    const allowed = $$("[name=activities]:checked", form).map((c) => c.value);
    for (const opt of select.options) {
      opt.hidden = opt.value !== "" && allowed.length > 0 && !allowed.includes(opt.value);
    }
    if (select.selectedOptions[0] && select.selectedOptions[0].hidden) select.value = "";
  }

  for (const ev of ["input", "change"]) {
    document.addEventListener(ev, (e) => {
      const form = e.target.closest && e.target.closest("form.project-form, form.status-form");
      if (!form) return;
      if (e.target.name === "custom") $("[name=color][value=custom]", form).checked = true;
      updateProjectForm(form);
    });
  }
  document.addEventListener("htmx:afterSwap", (e) => {
    const form = $("form.project-form, form.status-form", e.detail.target);
    if (form) updateProjectForm(form);
  });

  // --- issue overview -------------------------------------------------------------------------------

  let selectedIssue = null;
  const icards = () => $$("#issues .icard");
  const selectedCard = () => icards().find((c) => c.dataset.issue === selectedIssue) || null;

  function selectCard(card, scroll = true) {
    icards().forEach((c) => c.classList.toggle("selected", c === card));
    selectedIssue = card ? card.dataset.issue : null;
    if (card && scroll) card.scrollIntoView({ block: "nearest", inline: "nearest" });
  }

  function restoreCard() {
    if (!$("#issues")) return;
    const all = icards();
    selectCard(all.find((c) => c.dataset.issue === selectedIssue) || all[0] || null, false);
  }

  // The cards of the selected card's column, across repository lanes; the
  // new and hidden tabs are one list.
  function columnOf(card) {
    const col = card && card.closest(".column");
    if (!col) return icards();
    return $$(`#issues .column[data-status="${CSS.escape(col.dataset.status)}"] .icard`);
  }

  function moveCard(delta) {
    const card = selectedCard();
    if (!card) return selectCard(icards()[0] || null);
    const list = columnOf(card);
    const i = list.indexOf(card);
    selectCard(list[Math.max(0, Math.min(list.length - 1, i + delta))]);
  }

  // moveColumn selects the card at about the same place in the next column
  // with cards; in the lists h and l move like k and j.
  function moveColumn(delta) {
    const card = selectedCard();
    const col = card && card.closest(".column");
    if (!col) return moveCard(delta);
    const board = col.closest(".board");
    const cols = $$(".column", board);
    const row = $$(".icard", col).indexOf(card);
    for (let i = cols.indexOf(col) + delta; i >= 0 && i < cols.length; i += delta) {
      const cs = $$(".icard", cols[i]);
      if (cs.length) return selectCard(cs[Math.min(row, cs.length - 1)]);
    }
  }

  document.addEventListener("click", (e) => {
    const card = e.target.closest(".icard");
    if (card) selectCard(card, false);
  });
  document.addEventListener("htmx:afterSettle", () => restoreCard());

  function issuesKey(e) {
    const card = selectedCard();
    const filter = $("#filter"), sort = $("#sort"); // missing while issues.yaml is broken
    let handled = true;
    switch (e.key) {
      case "j": case "ArrowDown": moveCard(1); break;
      case "k": case "ArrowUp": moveCard(-1); break;
      case "h": case "ArrowLeft": moveColumn(-1); break;
      case "l": case "ArrowRight": moveColumn(1); break;
      case "G": case "End": { const l = columnOf(card); selectCard(l[l.length - 1] || null); break; }
      case "Home": selectCard(columnOf(card)[0] || null); break;
      case "g": sequence("g", () => selectCard(columnOf(card)[0] || null)); break;
      case "x":
        if (pending === "g" && card) { pending = ""; clickKey(card, "o"); } else handled = clickKey(card, "x");
        break;
      case "/": if (filter) filter.focus(); break;
      case "Escape":
        // Like the TUI: Esc clears the filter, else goes back to the entries.
        if (filter && filter.value) {
          filter.value = "";
          filter.dispatchEvent(new Event("search", { bubbles: true }));
        } else {
          clickKey(null, "T");
        }
        break;
      case "O":
        if (sort) {
          sort.selectedIndex = (sort.selectedIndex + 1) % sort.options.length;
          sort.dispatchEvent(new Event("change", { bubbles: true }));
        }
        break;
      case "s": handled = clickKey(card, "Enter"); break;
      default:
        handled = clickKey(card, e.key);
    }
    if (handled) e.preventDefault();
  }

  // Dragging a card onto a column puts it there, before the card under the pointer.
  let dragged = null;
  const clearDrop = () => $$(".drop-before, .col-body.over").forEach((el) => el.classList.remove("drop-before", "over"));

  function dropTarget(body, y) {
    return $$(".icard", body).find((c) => c !== dragged && y < c.getBoundingClientRect().top + c.offsetHeight / 2) || null;
  }

  document.addEventListener("dragstart", (e) => {
    const card = e.target.closest && e.target.closest(".icard");
    if (!card) return;
    dragged = card;
    selectCard(card, false);
    e.dataTransfer.effectAllowed = "move";
    e.dataTransfer.setData("text/plain", card.dataset.issue);
    requestAnimationFrame(() => card.classList.add("dragging"));
  });
  document.addEventListener("dragend", () => {
    if (dragged) dragged.classList.remove("dragging");
    dragged = null;
    clearDrop();
  });
  document.addEventListener("dragover", (e) => {
    const body = dragged && e.target.closest && e.target.closest(".col-body");
    if (!body) return;
    e.preventDefault();
    clearDrop();
    body.classList.add("over");
    const before = dropTarget(body, e.clientY);
    if (before) before.classList.add("drop-before");
  });
  document.addEventListener("drop", (e) => {
    const body = dragged && e.target.closest && e.target.closest(".col-body");
    if (!body) return;
    e.preventDefault();
    const status = body.closest(".column").dataset.status;
    const sameColumn = dragged.closest(".column") && dragged.closest(".column").dataset.status === status;
    const sort = $("#viewstate [name=v_sort]");
    clearDrop();
    if (sameColumn && sort && sort.value !== "") {
      toast("Pořadí jde měnit jen ve vlastním řazení (O)", true);
      return;
    }
    const before = dropTarget(body, e.clientY);
    const values = { key: dragged.dataset.issue, status, before: before ? before.dataset.issue : "" };
    htmx.ajax("POST", "/issues/status", { source: body, values, swap: "none" });
  });

  // --- keyboard ------------------------------------------------------------------------------------

  // clickKey activates the element bound to key: in root first, then in the toolbar.
  function clickKey(root, key) {
    const sel = `[data-key="${CSS.escape(key)}"]`;
    const el = (root && $(sel, root)) || $(`.toolbar ${sel}, .topbar ${sel}`);
    if (!el || el.classList.contains("disabled")) return false;
    el.click();
    return true;
  }

  function sequence(key, action) {
    if (pending === key) {
      pending = "";
      action();
    } else {
      pending = key;
      clearTimeout(pendingTimer);
      pendingTimer = setTimeout(() => { pending = ""; }, 800);
    }
  }

  document.addEventListener("keydown", (e) => {
    const t = e.target;
    const dlg = openDialog();

    // Form: Ctrl+Enter or Ctrl+S saves.
    if (dlg && (e.ctrlKey || e.metaKey) && (e.key === "Enter" || e.key === "s")) {
      const form = $("form.entry-form", dlg) || $("form.notes-form", dlg);
      if (form) {
        e.preventDefault();
        form.requestSubmit();
      }
      return;
    }
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    if (t.matches("input, textarea, select")) {
      if (e.key === "Escape" && t.id === "filter") {
        t.value = "";
        t.dispatchEvent(new Event("search", { bubbles: true }));
        t.blur();
      }
      if (e.key === "Enter" && t.id === "filter") t.blur();
      return;
    }
    if (dlg) {
      // Modal shortcuts (status: h/l/y/m; issue: i opens the issue).
      const el = $(`[data-modal-key~="${CSS.escape(e.key)}"]`, dlg);
      const notes = $("textarea[name=notes]", dlg);
      if (el) {
        e.preventDefault();
        el.click();
      } else if (e.key === "n" && notes) {
        e.preventDefault(); // the issue detail: n edits the notes, like the TUI
        notes.focus();
      } else if (e.key === "q") {
        dlg.close();
      }
      return;
    }

    if ($("#issues")) return issuesKey(e);
    if (!$("#main")) return; // the shortcuts belong to the entries page

    const row = selectedRow();
    let handled = true;
    switch (e.key) {
      case "j": case "ArrowDown": move(1); break;
      case "k": case "ArrowUp": move(-1); break;
      case "G": case "End": { const all = rows(); select(all[all.length - 1]); break; }
      case "Home": select(rows()[0]); break;
      case "g": sequence("g", () => select(rows()[0])); break;
      case "ArrowLeft": clickKey(null, "h"); break;
      case "ArrowRight": clickKey(null, "l"); break;
      case "Tab": {
        const tabs = $$(".tabs .tab");
        const i = tabs.findIndex((x) => x.classList.contains("active"));
        tabs[(i + (e.shiftKey ? 2 : 1)) % tabs.length].click();
        break;
      }
      case "/": $("#filter").focus(); break;
      case "Escape":
        if ($("#filter").value) {
          $("#filter").value = "";
          $("#filter").dispatchEvent(new Event("search", { bubbles: true }));
        }
        break;
      case "a": case "o": case "Insert": case "n":
        if (row) htmx.ajax("GET", `/entries/new?date=${row.dataset.date}`, { target: "#modal" });
        else clickKey(null, "a");
        break;
      case "Enter": handled = clickKey(row, "e"); break;
      case "d": if (row) sequence("d", () => clickKey(row, "d")); break;
      case "Delete": handled = clickKey(row, "d"); break;
      case "y": if (row) sequence("y", () => yank(row.dataset.id)); break;
      case "p": paste(); break;
      case " ": handled = clickKey(row, "s"); break;
      case "x":
        if (pending === "g" && row) { pending = ""; clickKey(row, "i"); } else handled = false;
        break;
      default:
        handled = clickKey(row, e.key);
    }
    if (handled) e.preventDefault();
  });

  document.addEventListener("DOMContentLoaded", () => {
    restoreSelection();
    restoreCard();
    scheduleToastHide();
  });
})();
