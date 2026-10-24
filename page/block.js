// better-lucx: the AmneziaWG link format card on the subscription page.
// The data (the nodes and their forms) is in <script id="better-lucx-awg-data">; this script draws a card
// under the panel's own card and lets the visitor pick a form and copy it. MIT licensed.
(function () {
  'use strict';

  var TEXT = {
    en: {
      title: 'AmneziaWG link format',
      hint: 'Pick the form of the link, then copy it.',
      copy: 'Copy',
      copied: 'Copied',
      legacy: 'amneziawg:// (legacy)',
      group: 'AmneziaWG link format',
    },
    ru: {
      title: 'Формат ссылки AmneziaWG',
      hint: 'Выберите вид ссылки и скопируйте её.',
      copy: 'Копировать',
      copied: 'Скопировано',
      legacy: 'amneziawg:// (старый)',
      group: 'Формат ссылки AmneziaWG',
    },
  };
  var LABELS = { vpn: 'vpn://', 'conf-link': 'amneziawg://', conf: '.conf' };

  // The page keeps the visitor's language in the subLang cookie (it is not on <html lang>); the browser's own
  // language is the last resort.
  function pickLang(cookieLang, htmlLang, navLang) {
    var l = String(cookieLang || htmlLang || navLang || 'en').slice(0, 2).toLowerCase();
    return l === 'ru' ? 'ru' : 'en';
  }

  function cookieValue(cookies, name) {
    var m = new RegExp('(?:^|;\\s*)' + name + '=([^;]*)').exec(String(cookies || ''));
    return m ? decodeURIComponent(m[1]) : '';
  }

  // The nodes as the card shows them: a name and the forms in the order they are offered, each with its label.
  function buildModel(nodes, lang) {
    var t = TEXT[lang] || TEXT.en;
    return (Array.isArray(nodes) ? nodes : [])
      .filter(function (n) {
        return n && typeof n.name === 'string' && Array.isArray(n.formats) && n.formats.length > 0;
      })
      .map(function (n) {
        return {
          name: n.name,
          options: n.formats
            .filter(function (f) {
              return f && typeof f.value === 'string' && f.value !== '';
            })
            .map(function (f) {
              return { id: f.id, label: f.id === 'legacy' ? t.legacy : LABELS[f.id] || f.id, value: f.value };
            }),
        };
      })
      .filter(function (n) {
        return n.options.length > 0;
      });
  }

  if (typeof module !== 'undefined' && module.exports) {
    module.exports = { buildModel: buildModel, pickLang: pickLang, cookieValue: cookieValue, TEXT: TEXT };
    return;
  }

  function el(tag, className, text) {
    var node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined) node.textContent = text;
    return node;
  }

  function copyText(text) {
    if (navigator.clipboard && window.isSecureContext) {
      return navigator.clipboard.writeText(text).catch(function () {
        return legacyCopy(text);
      });
    }
    return Promise.resolve(legacyCopy(text));
  }

  function legacyCopy(text) {
    var area = document.createElement('textarea');
    area.value = text;
    area.setAttribute('readonly', '');
    area.style.cssText = 'position:fixed;top:-1000px;opacity:0';
    document.body.appendChild(area);
    area.select();
    var ok = false;
    try {
      ok = document.execCommand('copy');
    } catch (e) {
      ok = false;
    }
    document.body.removeChild(area);
    return ok;
  }

  function currentLang() {
    return pickLang(cookieValue(document.cookie, 'subLang'), document.documentElement.getAttribute('lang'), navigator.language);
  }

  var uid = 0;

  function renderNode(node, t) {
    var chosen = node.options[0];
    var row = el('div', 'bl-node');
    row.appendChild(el('span', 'bl-name', node.name)).title = node.name;

    var seg = el('div', 'bl-formats');
    seg.setAttribute('role', 'radiogroup');
    seg.setAttribute('aria-label', t.group);
    var groupName = 'bl-awg-' + ++uid;
    var items = node.options.map(function (opt) {
      var label = el('label', 'bl-opt' + (opt === chosen ? ' bl-opt-on' : ''));
      var input = el('input');
      input.type = 'radio';
      input.name = groupName;
      input.checked = opt === chosen;
      input.addEventListener('change', function () {
        chosen = opt;
        items.forEach(function (it) {
          it.label.className = 'bl-opt' + (it.opt === chosen ? ' bl-opt-on' : '');
        });
        button.textContent = t.copy;
      });
      label.appendChild(input);
      label.appendChild(el('span', '', opt.label));
      seg.appendChild(label);
      return { opt: opt, label: label };
    });
    row.appendChild(seg);

    var button = el('button', 'bl-copy', t.copy);
    button.type = 'button';
    button.addEventListener('click', function () {
      Promise.resolve(copyText(chosen.value)).then(function (ok) {
        if (ok === false) return;
        button.textContent = t.copied;
        setTimeout(function () {
          button.textContent = t.copy;
        }, 1500);
      });
    });
    row.appendChild(button);
    return row;
  }

  function render(nodes) {
    var lang = currentLang();
    var t = TEXT[lang];
    var model = buildModel(nodes, lang);
    var card = el('div', 'ant-card ant-card-bordered sub-card bl-awg');
    var body = el('div', 'ant-card-body');
    body.appendChild(el('div', 'bl-head', t.title));
    body.appendChild(el('div', 'bl-hint', t.hint));
    model.forEach(function (n) {
      body.appendChild(renderNode(n, t));
    });
    card.appendChild(body);
    card.setAttribute('data-lang', lang);
    return card;
  }

  var dataNode = document.getElementById('better-lucx-awg-data');
  if (!dataNode) return;
  var nodes;
  try {
    nodes = JSON.parse(dataNode.textContent);
  } catch (e) {
    return;
  }

  function mount() {
    var anchor = null;
    var cards = document.querySelectorAll('.sub-card:not(.bl-awg)');
    if (cards.length) anchor = cards[cards.length - 1];
    if (!anchor || !anchor.parentNode) return;
    var current = document.querySelector('.bl-awg');
    var lang = currentLang();
    if (current && current.getAttribute('data-lang') === lang && current.previousElementSibling === anchor) return;
    var card = render(nodes);
    if (current) current.parentNode.replaceChild(card, current);
    else anchor.parentNode.insertBefore(card, anchor.nextSibling);
  }

  var scheduled = false;
  function schedule() {
    if (scheduled) return;
    scheduled = true;
    setTimeout(function () {
      scheduled = false;
      mount();
    }, 50);
  }

  new MutationObserver(schedule).observe(document.documentElement, {
    childList: true,
    subtree: true,
    attributes: true,
    attributeFilter: ['lang'],
  });
  schedule();
})();
