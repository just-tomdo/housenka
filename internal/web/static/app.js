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

  async function copy(text, label) {
    try {
      await navigator.clipboard.writeText(text);
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
      const form = $("form.entry-form", dlg);
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
      const el = $(`[data-modal-key="${CSS.escape(e.key)}"]`, dlg);
      if (el) {
        e.preventDefault();
        el.click();
      } else if (e.key === "q") {
        dlg.close();
      }
      return;
    }

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
    scheduleToastHide();
  });
})();
