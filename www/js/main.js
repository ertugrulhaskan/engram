// engram.im — landing page behavior.
// Plain classic script, no modules and no dependencies (loaded with `defer`).
// The pre-paint theme guard stays inline in <head> so the correct theme paints
// on the first frame; everything below runs after the DOM is parsed.

// --- copy install command (guarded + clears base classes so the green state shows) ---
(function () {
  function copyCmd(btn) {
    var code = btn.parentElement.querySelector('code');
    var base = ['text-neutral-600', 'dark:text-neutral-400', 'border-neutral-300', 'dark:border-neutral-700'];
    var ok = ['text-green-700', 'dark:text-green-400', 'border-green-600', 'dark:border-green-500'];
    function done() {
      btn.textContent = 'copied ✓';
      base.forEach(function (c) { btn.classList.remove(c); });
      ok.forEach(function (c) { btn.classList.add(c); });
      setTimeout(function () {
        btn.textContent = 'copy';
        ok.forEach(function (c) { btn.classList.remove(c); });
        base.forEach(function (c) { btn.classList.add(c); });
      }, 1600);
    }
    var text = code.textContent;
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(done).catch(function () {});
    } else {
      try {
        var r = document.createRange(); r.selectNode(code);
        var s = window.getSelection(); s.removeAllRanges(); s.addRange(r);
        document.execCommand('copy'); s.removeAllRanges(); done();
      } catch (e) { /* clipboard unavailable */ }
    }
  }
  document.querySelectorAll('[data-copy]').forEach(function (btn) {
    btn.addEventListener('click', function () { copyCmd(btn); });
  });
})();

// --- light / dark / system theme (button + menu; cross-browser, no <details>) ---
(function () {
  var media = window.matchMedia('(prefers-color-scheme: dark)');
  var root = document.getElementById('thm');
  var btn = document.getElementById('thm-btn');
  var menu = document.getElementById('thm-menu');
  var items = Array.prototype.slice.call(menu.querySelectorAll('[data-theme-set]'));
  function lsGet() { try { return localStorage.theme; } catch (e) { return undefined; } }
  function lsSet(v) { try { if (v) { localStorage.theme = v; } else { localStorage.removeItem('theme'); } } catch (e) {} }
  function lsHas() { try { return 'theme' in localStorage; } catch (e) { return false; } }
  function cur() { return lsGet() || 'system'; }
  function apply() {
    var t = lsGet(); // 'light' | 'dark' | undefined(system)
    document.documentElement.classList.toggle('dark', t === 'dark' || (!t && media.matches));
    var c = cur();
    document.querySelectorAll('[data-sico]').forEach(function (s) {
      s.classList.toggle('hidden', s.getAttribute('data-sico') !== c);
    });
    items.forEach(function (b) {
      var on = b.getAttribute('data-theme-set') === c;
      b.setAttribute('aria-checked', on ? 'true' : 'false');
      b.querySelector('[data-check]').textContent = on ? '✓' : '';
    });
  }
  var open = false;
  function setOpen(v) {
    open = v;
    menu.classList.toggle('hidden', !v);
    btn.setAttribute('aria-expanded', v ? 'true' : 'false');
  }
  btn.addEventListener('click', function () { setOpen(!open); if (open) items[0].focus(); });
  btn.addEventListener('keydown', function (e) {
    if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') { e.preventDefault(); setOpen(true); items[0].focus(); }
  });
  items.forEach(function (b, i) {
    b.addEventListener('click', function () {
      var v = b.getAttribute('data-theme-set');
      lsSet(v === 'system' ? null : v);
      apply(); setOpen(false); btn.focus();
    });
    b.addEventListener('keydown', function (e) {
      if (e.key === 'ArrowDown') { e.preventDefault(); items[(i + 1) % items.length].focus(); }
      else if (e.key === 'ArrowUp') { e.preventDefault(); items[(i - 1 + items.length) % items.length].focus(); }
      else if (e.key === 'Home') { e.preventDefault(); items[0].focus(); }
      else if (e.key === 'End') { e.preventDefault(); items[items.length - 1].focus(); }
      else if (e.key === 'Escape') { e.preventDefault(); setOpen(false); btn.focus(); }
    });
  });
  document.addEventListener('click', function (e) { if (open && !root.contains(e.target)) setOpen(false); });
  document.addEventListener('keydown', function (e) { if (e.key === 'Escape' && open) { setOpen(false); btn.focus(); } });
  media.addEventListener('change', function () { if (!lsHas()) apply(); });
  apply();
})();

// --- nav scroll-spy: active = the last section whose top has passed below the header ---
(function () {
  var links = {};
  document.querySelectorAll('nav a[data-nav]').forEach(function (a) { links[a.getAttribute('href').slice(1)] = a; });
  var ids = Object.keys(links);
  var sections = ids.map(function (id) { return document.getElementById(id); });
  var on = ['font-bold', 'text-neutral-900', 'dark:text-white'];
  var offset = 90; // just below the sticky header
  function update() {
    var active = null;
    sections.forEach(function (s, i) { if (s && s.getBoundingClientRect().top <= offset) active = ids[i]; });
    // at the bottom of the page the last section can't reach the header — pin it active
    if (window.innerHeight + window.scrollY >= document.documentElement.scrollHeight - 2) active = ids[ids.length - 1];
    ids.forEach(function (id) { on.forEach(function (c) { links[id].classList.toggle(c, id === active); }); });
  }
  window.addEventListener('scroll', update, { passive: true });
  window.addEventListener('resize', update, { passive: true });
  update();
})();

// --- demo view tabs (under the terminal): auto-advance until the user picks another tab ---
(function () {
  var panels = Array.prototype.slice.call(document.querySelectorAll('[data-tabview]'));
  var tabs = Array.prototype.slice.call(document.querySelectorAll('[role="tab"]'));
  var cur = 0, timer = null, taken = false;
  var reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  var onCls = ['bg-neutral-200', 'dark:bg-neutral-800', 'text-neutral-800', 'dark:text-neutral-200'];
  var offCls = ['text-neutral-600', 'dark:text-neutral-400'];
  function render(n, focusTab) {
    cur = n;
    panels.forEach(function (p, i) { p.classList.toggle('hidden', i !== n); });
    tabs.forEach(function (t, i) {
      var on = i === n;
      t.setAttribute('aria-selected', on ? 'true' : 'false');
      t.tabIndex = on ? 0 : -1;
      onCls.forEach(function (c) { t.classList.toggle(c, on); });
      offCls.forEach(function (c) { t.classList.toggle(c, !on); });
      var c = t.querySelector('[data-num]');
      c.classList.toggle('bg-blue-600', on); c.classList.toggle('text-white', on);
      c.classList.toggle('dark:bg-blue-400', on); c.classList.toggle('dark:text-neutral-950', on);
      c.classList.toggle('bg-neutral-200', !on); c.classList.toggle('text-neutral-700', !on);
      c.classList.toggle('dark:bg-neutral-700', !on); c.classList.toggle('dark:text-neutral-300', !on);
    });
    if (focusTab) tabs[n].focus();
  }
  function advance() { render((cur + 1) % panels.length, false); }
  function start() { if (!reduce && !taken && !timer) timer = setInterval(advance, 3500); }
  function stop() { if (timer) { clearInterval(timer); timer = null; } }
  function activate(n) { taken = true; stop(); render(n, true); } // any user pick pins the demo
  tabs.forEach(function (t, i) {
    t.addEventListener('click', function () { activate(i); });
    t.addEventListener('keydown', function (e) {
      var k = e.key;
      if (k === 'ArrowRight' || k === 'ArrowDown') { e.preventDefault(); activate((cur + 1) % tabs.length); }
      else if (k === 'ArrowLeft' || k === 'ArrowUp') { e.preventDefault(); activate((cur - 1 + tabs.length) % tabs.length); }
      else if (k === 'Home') { e.preventDefault(); activate(0); }
      else if (k === 'End') { e.preventDefault(); activate(tabs.length - 1); }
    });
  });
  // Pause auto-advance while a keyboard user is focused inside the demo (a11y); resume on blur unless pinned.
  var demo = document.getElementById('demo');
  demo.addEventListener('focusin', stop);
  demo.addEventListener('focusout', function (e) { if (!demo.contains(e.relatedTarget)) start(); });
  render(0, false); start();
})();

// --- mobile drawer (slides in from the right; full height) ---
(function () {
  var ham = document.getElementById('ham');
  var drawer = document.getElementById('drawer');
  var bg = document.getElementById('drawer-bg');
  var panel = document.getElementById('drawer-panel');
  var closeBtn = document.getElementById('drawer-close');
  if (!ham || !drawer) return;
  function open() {
    drawer.classList.remove('pointer-events-none');
    bg.classList.remove('opacity-0');
    panel.classList.remove('translate-x-full');
    ham.setAttribute('aria-expanded', 'true');
    document.body.classList.add('overflow-hidden');
    closeBtn.focus();
  }
  function close() {
    drawer.classList.add('pointer-events-none');
    bg.classList.add('opacity-0');
    panel.classList.add('translate-x-full');
    ham.setAttribute('aria-expanded', 'false');
    document.body.classList.remove('overflow-hidden');
  }
  ham.addEventListener('click', open);
  closeBtn.addEventListener('click', function () { close(); ham.focus(); });
  bg.addEventListener('click', close);
  document.querySelectorAll('[data-drawer-link]').forEach(function (a) { a.addEventListener('click', close); });
  document.addEventListener('keydown', function (e) { if (e.key === 'Escape' && ham.getAttribute('aria-expanded') === 'true') { close(); ham.focus(); } });
  // crossing to desktop hides the drawer via CSS — also release scroll-lock / reset state
  window.matchMedia('(min-width: 768px)').addEventListener('change', function (e) { if (e.matches) close(); });
  // focus trap while open (honors aria-modal)
  panel.addEventListener('keydown', function (e) {
    if (e.key !== 'Tab') return;
    var f = panel.querySelectorAll('a[href], button');
    if (!f.length) return;
    var first = f[0], last = f[f.length - 1];
    if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
    else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
  });
})();

// --- cookie consent + Google Analytics (loads ONLY after opt-in) ---
// Analytics cookies are non-essential, so gtag.js is not requested until the
// visitor clicks Accept (and on later visits if they accepted before). Declining
// never loads it. Choice is remembered in localStorage.
(function () {
  var GA_ID = 'G-V97M01VZNY';
  var banner = document.getElementById('cookie-banner');
  function get() { try { return localStorage.getItem('cookie-consent'); } catch (e) { return null; } }
  function set(v) { try { localStorage.setItem('cookie-consent', v); } catch (e) {} }
  // The banner is fixed to the bottom; reserve equal space on the body so it never
  // overlaps the footer (re-measured on resize since it wraps on small screens).
  function pad() { document.body.style.paddingBottom = (banner && !banner.classList.contains('hidden')) ? banner.offsetHeight + 'px' : ''; }
  function show() { if (banner) { banner.classList.remove('hidden'); pad(); } }
  function hide() { if (banner) banner.classList.add('hidden'); document.body.style.paddingBottom = ''; }
  function loadGA() {
    if (window.__gaLoaded) return;
    window.__gaLoaded = true;
    var s = document.createElement('script');
    s.async = true;
    s.src = 'https://www.googletagmanager.com/gtag/js?id=' + GA_ID;
    document.head.appendChild(s);
    window.dataLayer = window.dataLayer || [];
    window.gtag = function () { dataLayer.push(arguments); };
    gtag('js', new Date());
    gtag('config', GA_ID);
  }
  var choice = get();
  if (choice === 'accepted') { loadGA(); }
  else if (choice !== 'declined' && banner) { show(); }
  var accept = document.getElementById('cookie-accept');
  var decline = document.getElementById('cookie-decline');
  if (accept) accept.addEventListener('click', function () { set('accepted'); hide(); loadGA(); });
  if (decline) decline.addEventListener('click', function () { set('declined'); hide(); });
  window.addEventListener('resize', pad);
})();

// --- install method switcher (Homebrew / Go tabs over one command box) ---
(function () {
  var onCls = ['bg-neutral-200', 'dark:bg-neutral-800', 'text-neutral-800', 'dark:text-neutral-200'];
  var offCls = ['text-neutral-500', 'dark:text-neutral-400', 'hover:text-neutral-800', 'dark:hover:text-neutral-200'];
  document.querySelectorAll('[data-install]').forEach(function (box) {
    var tabs = Array.prototype.slice.call(box.querySelectorAll('[data-install-tab]'));
    var code = box.querySelector('[data-install-code]');
    if (!tabs.length || !code) return;
    function select(tab) {
      code.textContent = tab.getAttribute('data-cmd');
      tabs.forEach(function (t) {
        var on = t === tab;
        t.setAttribute('aria-pressed', on ? 'true' : 'false');
        onCls.forEach(function (c) { t.classList.toggle(c, on); });
        offCls.forEach(function (c) { t.classList.toggle(c, !on); });
      });
    }
    tabs.forEach(function (t) { t.addEventListener('click', function () { select(t); }); });
  });
})();
