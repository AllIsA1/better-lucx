// Copyright (c) 2026 AllIsA1. SPDX-License-Identifier: MIT
(function () {
  'use strict';

  var TEXT = {
    en: {
      title: 'Own links',
      intro:
        'Give a client any link, of any scheme (vless://, yptun://, qwdtt://, openflux:// …). It is added to the end of the client’s subscription. Changes show up almost at once; a switched-off, expired or out-of-traffic client gets nothing, as in the panel. Only the usual subscription format is changed.',
      pick: 'Client',
      find: 'Find',
      searchPlaceholder: 'Email or part of it (empty: clients that have links)',
      withLinks: 'Clients that have own links',
      noneWithLinks: 'No client has own links yet. Find a client to add some.',
      noClients: 'No such client.',
      colOn: 'On',
      colName: 'Name',
      colLink: 'Link',
      empty: 'This client has no own links yet.',
      addTitle: 'Add links (one per line)',
      textPlaceholder: 'yptun://…\nqwdtt://…',
      namePlaceholder: 'Name for these links (optional)',
      add: 'Add',
      remove: 'Delete',
      confirmRemove: 'Delete this link?',
      links: function (n) { return n + (n === 1 ? ' link' : ' links'); },
      subline: function (sub, enable) { return 'Subscription: ' + sub + (enable ? '' : ' · the client is switched off'); },
      editorTitle: function (email) { return 'Links of ' + email; },
      added: function (n) { return 'Added: ' + n + '.'; },
      nothingAdded: 'Nothing was added.',
      saved: 'Saved.',
      failed: function (e) { return 'Failed: ' + e; },
      rejectedLine: function (line, why) { return line + ' — ' + why; },
    },
    ru: {
      title: 'Свои ссылки',
      intro:
        'Добавьте клиенту любую ссылку любой схемы (vless://, yptun://, qwdtt://, openflux:// …): она будет дописана в конец подписки клиента. Изменения видны почти сразу; у выключенного, просроченного клиента или без трафика ссылок нет, как и в панели. Меняется только обычный формат подписки.',
      pick: 'Клиент',
      find: 'Найти',
      searchPlaceholder: 'Email или его часть (пусто: клиенты со ссылками)',
      withLinks: 'Клиенты со своими ссылками',
      noneWithLinks: 'Ни у одного клиента пока нет своих ссылок. Найдите клиента, чтобы добавить.',
      noClients: 'Такого клиента нет.',
      colOn: 'Вкл',
      colName: 'Название',
      colLink: 'Ссылка',
      empty: 'У этого клиента пока нет своих ссылок.',
      addTitle: 'Добавить ссылки (по одной в строке)',
      textPlaceholder: 'yptun://…\nqwdtt://…',
      namePlaceholder: 'Название для этих ссылок (необязательно)',
      add: 'Добавить',
      remove: 'Удалить',
      confirmRemove: 'Удалить эту ссылку?',
      links: function (n) {
        var m = n % 10, h = n % 100;
        var w = m === 1 && h !== 11 ? 'ссылка' : m >= 2 && m <= 4 && (h < 12 || h > 14) ? 'ссылки' : 'ссылок';
        return n + ' ' + w;
      },
      subline: function (sub, enable) { return 'Подписка: ' + sub + (enable ? '' : ' · клиент выключен'); },
      editorTitle: function (email) { return 'Ссылки клиента ' + email; },
      added: function (n) { return 'Добавлено: ' + n + '.'; },
      nothingAdded: 'Ничего не добавлено.',
      saved: 'Сохранено.',
      failed: function (e) { return 'Не удалось: ' + e; },
      rejectedLine: function (line, why) { return line + ' — ' + why; },
    },
  };
  var REASONS = {
    ru: {
      'already added': 'уже добавлена',
      empty: 'пусто',
    },
  };

  var lang = 'en';
  var current = null; // {email, subId, enable}

  function T() { return TEXT[lang] || TEXT.en; }
  function $(id) { return document.getElementById(id); }

  function el(tag, props, children) {
    var node = document.createElement(tag);
    Object.keys(props || {}).forEach(function (k) { node[k] = props[k]; });
    (children || []).forEach(function (c) { node.appendChild(typeof c === 'string' ? document.createTextNode(c) : c); });
    return node;
  }

  function message(text, kind) {
    var m = $('message');
    m.textContent = text || '';
    m.className = 'message' + (kind ? ' ' + kind : '');
  }

  function why(reason) {
    var map = REASONS[lang];
    return (map && map[reason]) || reason;
  }

  function applyText() {
    var t = T();
    Array.prototype.forEach.call(document.querySelectorAll('[data-i18n]'), function (n) {
      var v = t[n.getAttribute('data-i18n')];
      if (typeof v === 'string') n.textContent = v;
    });
    document.documentElement.lang = lang;
    $('search').placeholder = t.searchPlaceholder;
    $('text').placeholder = t.textPlaceholder;
    $('name').placeholder = t.namePlaceholder;
  }

  function fail(res) {
    message(T().failed((res.body && res.body.error) || res.error || res.status), 'bad');
  }

  function renderClients(rows, hintText) {
    var list = $('clients');
    list.textContent = '';
    $('clientsHint').textContent = hintText || '';
    rows.forEach(function (c) {
      var li = el('li', { title: c.subId ? c.subId : '' }, [
        el('strong', { textContent: c.email }),
        el('span', { className: 'count', textContent: T().links(c.links) }),
      ]);
      if (current && current.email === c.email) li.className = 'current';
      li.onclick = function () { choose(c); };
      list.appendChild(li);
    });
  }

  function loadClients() {
    var q = $('search').value.trim();
    var path = q ? '/api/clients?search=' + encodeURIComponent(q) : '/api/overview';
    lucx.api(path).then(function (res) {
      if (!res.ok) return fail(res);
      var rows = res.body.clients || [];
      if (!q) {
        renderClients(rows.map(function (r) { return { email: r.email, links: r.links }; }), rows.length ? T().withLinks : T().noneWithLinks);
      } else {
        renderClients(rows, rows.length ? '' : T().noClients);
      }
    });
  }

  function choose(c) {
    current = c;
    $('editor').hidden = false;
    $('rejected').textContent = '';
    $('editorTitle').textContent = T().editorTitle(c.email);
    $('subline').textContent = c.subId ? T().subline(c.subId, c.enable !== false) : '';
    loadLinks();
    loadClients();
  }

  function loadLinks() {
    if (!current) return;
    lucx.api('/api/links?email=' + encodeURIComponent(current.email)).then(function (res) {
      if (!res.ok) return fail(res);
      renderLinks(res.body.links || []);
    });
  }

  function post(path, body) { return lucx.api(path, { method: 'POST', body: body }); }

  function renderLinks(links) {
    var tbody = $('links').querySelector('tbody');
    tbody.textContent = '';
    $('empty').hidden = links.length > 0;
    $('links').hidden = links.length === 0;
    links.forEach(function (l) {
      var on = el('input', { type: 'checkbox', checked: l.enabled });
      on.onchange = function () {
        post('/api/links/update', { email: current.email, id: l.id, enabled: on.checked }).then(function (res) {
          if (!res.ok) { on.checked = !on.checked; return fail(res); }
          row.className = on.checked ? '' : 'off';
          message(T().saved, 'good');
        });
      };
      var name = el('input', { type: 'text', value: l.name || '', maxLength: 100 });
      name.onchange = function () {
        post('/api/links/update', { email: current.email, id: l.id, name: name.value }).then(function (res) {
          if (!res.ok) return fail(res);
          message(T().saved, 'good');
        });
      };
      var del = el('button', { textContent: T().remove });
      del.onclick = function () {
        if (!window.confirm(T().confirmRemove)) return;
        post('/api/links/delete', { email: current.email, id: l.id }).then(function (res) {
          if (!res.ok) return fail(res);
          loadLinks();
          loadClients();
        });
      };
      var row = el('tr', { className: l.enabled ? '' : 'off' }, [
        el('td', {}, [on]),
        el('td', {}, [name]),
        el('td', { className: 'url', title: l.url, textContent: l.url }),
        el('td', {}, [del]),
      ]);
      tbody.appendChild(row);
    });
  }

  function addLinks() {
    if (!current) return;
    var text = $('text').value;
    if (!text.trim()) return;
    var btn = $('add');
    btn.disabled = true;
    post('/api/links', { email: current.email, text: text, name: $('name').value }).then(function (res) {
      btn.disabled = false;
      if (!res.ok) return fail(res);
      var added = res.body.added || [];
      var rejected = res.body.rejected || [];
      var box = $('rejected');
      box.textContent = '';
      rejected.forEach(function (r) {
        box.appendChild(el('li', { textContent: T().rejectedLine(r.line, why(r.reason)) }));
      });
      if (added.length) {
        $('text').value = '';
        message(T().added(added.length), 'good');
      } else {
        message(T().nothingAdded, 'bad');
      }
      loadLinks();
      loadClients();
    });
  }

  function start(ctx) {
    lang = ctx && ctx.lang && String(ctx.lang).slice(0, 2) === 'ru' ? 'ru' : 'en';
    lucx.applyTheme();
    applyText();
    if (current) $('editorTitle').textContent = T().editorTitle(current.email);
    loadClients();
  }

  $('searchBtn').onclick = loadClients;
  $('search').onkeydown = function (e) { if (e.key === 'Enter') loadClients(); };
  $('add').onclick = addLinks;

  lucx.ready().then(start);
  lucx.onContext(function (ctx) {
    lang = ctx && ctx.lang && String(ctx.lang).slice(0, 2) === 'ru' ? 'ru' : 'en';
    lucx.applyTheme();
    applyText();
  });
})();
