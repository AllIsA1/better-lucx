// Run with: node --test page/block.test.mjs
import test from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';

const { buildModel, pickLang, cookieValue, TEXT } = createRequire(import.meta.url)('./block.js');

const nodes = [
  {
    name: 'node one',
    formats: [
      { id: 'vpn', value: 'vpn://AAA' },
      { id: 'conf-link', value: 'amneziawg://BBB#node' },
      { id: 'legacy', value: 'amneziawg://k@h:1?x=1' },
      { id: 'conf', value: '[Interface]\n[Peer]\n' },
    ],
  },
];

test('the forms come in the order given, with the panel-style labels', () => {
  const [node] = buildModel(nodes, 'en');
  assert.deepEqual(
    node.options.map((o) => o.label),
    ['vpn://', 'amneziawg://', 'amneziawg:// (legacy)', '.conf'],
  );
  assert.deepEqual(
    node.options.map((o) => o.value),
    ['vpn://AAA', 'amneziawg://BBB#node', 'amneziawg://k@h:1?x=1', '[Interface]\n[Peer]\n'],
  );
  assert.equal(node.name, 'node one');
});

test('the legacy form is named in the language of the page', () => {
  assert.equal(buildModel(nodes, 'ru')[0].options[2].label, 'amneziawg:// (старый)');
  assert.equal(buildModel(nodes, 'xx')[0].options[2].label, 'amneziawg:// (legacy)');
});

test('what is not a node, or has no value, is left out', () => {
  assert.deepEqual(buildModel(null, 'en'), []);
  assert.deepEqual(buildModel([null, {}, { name: 'x' }, { name: 'y', formats: [] }], 'en'), []);
  const model = buildModel([{ name: 'z', formats: [{ id: 'vpn', value: '' }, { id: 'conf', value: 'c' }, null] }], 'en');
  assert.equal(model.length, 1);
  assert.deepEqual(model[0].options.map((o) => o.id), ['conf']);
  assert.deepEqual(buildModel([{ name: 'z', formats: [{ id: 'vpn', value: '' }] }], 'en'), []);
});

test('a node without a name is left out', () => {
  assert.deepEqual(buildModel([{ formats: [{ id: 'vpn', value: 'v' }] }, { name: 5, formats: [{ id: 'vpn', value: 'v' }] }], 'en'), []);
});

test('an unknown form keeps its id as its label', () => {
  assert.equal(buildModel([{ name: 'n', formats: [{ id: 'future', value: 'v' }] }], 'en')[0].options[0].label, 'future');
});

test('the language comes from the page cookie first, then <html lang>, then the browser', () => {
  assert.equal(pickLang('ru-RU', 'en', 'en-US'), 'ru');
  assert.equal(pickLang('en-US', 'ru', 'ru-RU'), 'en');
  assert.equal(pickLang('', 'ru', 'en-US'), 'ru');
  assert.equal(pickLang('', '', 'ru-RU'), 'ru');
  assert.equal(pickLang('RU-ru', '', ''), 'ru');
  assert.equal(pickLang('de', 'ru', 'ru'), 'en');
  assert.equal(pickLang('', '', ''), 'en');
  assert.equal(pickLang(null, undefined, undefined), 'en');
});

test('the cookie is read by name and decoded', () => {
  assert.equal(cookieValue('a=1; subLang=ru-RU; b=2', 'subLang'), 'ru-RU');
  assert.equal(cookieValue('subLang=en-US', 'subLang'), 'en-US');
  assert.equal(cookieValue('xsubLang=ru; other=1', 'subLang'), '');
  assert.equal(cookieValue('subLang=a%2Db', 'subLang'), 'a-b');
  assert.equal(cookieValue('', 'subLang'), '');
  assert.equal(cookieValue(undefined, 'subLang'), '');
});

test('both languages have every text', () => {
  assert.deepEqual(Object.keys(TEXT.ru).sort(), Object.keys(TEXT.en).sort());
  for (const lang of Object.values(TEXT)) for (const v of Object.values(lang)) assert.ok(v.length > 0);
});
