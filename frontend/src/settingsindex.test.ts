import assert from "node:assert/strict";
import { test } from "node:test";

import { searchSettings, settingsIndex } from "./settingsindex.ts";

test("настройка находится по подписи", () => {
  const got = searchSettings("интерлиньяж");
  assert.equal(got.length, 1);
  assert.equal(got[0].section, "editor");
});

// Слово может честно относиться к двум настройкам: «кегль» — это и кегль
// текста заметки, и масштаб интерфейса. Находиться должны обе.
test("одно слово может вести в несколько настроек", () => {
  const got = searchSettings("кегль");
  assert.deepEqual(
    got.map((item) => item.label).sort(),
    ["Кегль", "Масштаб текста"],
  );
});

// Человек помнит «тёмная», а пункт называется «Оформление».
test("настройка находится по слову, которого нет в подписи", () => {
  const got = searchSettings("тёмная");
  assert.deepEqual(
    got.map((item) => item.label),
    ["Оформление"],
  );
});

test("регистр и обрывок слова не мешают", () => {
  assert.ok(searchSettings("ПРОЗ").length > 0);
  assert.ok(searchSettings("вим").length > 0);
});

// Пустой запрос — это «ещё не искали», а не «показать всё».
test("пустой запрос не находит ничего", () => {
  assert.deepEqual(searchSettings(""), []);
  assert.deepEqual(searchSettings("   "), []);
});

test("несуществующее не находится", () => {
  assert.deepEqual(searchSettings("телепортация"), []);
});

// Два списка — этот и разделы в Settings.tsx — обязаны совпадать: раздел без
// единой записи в поиске просто не находится, и понять это можно только руками.
test("каждый раздел настроек представлен в поиске", () => {
  const sections = ["appearance", "storage", "editor", "shortcuts", "agent", "about"];
  for (const section of sections) {
    const found = settingsIndex.some((entry) => entry.section === section);
    assert.ok(found, `раздел ${section} не представлен в поиске`);
  }
});

test("в поиске нет разделов, которых нет в настройках", () => {
  const sections = new Set(["appearance", "storage", "editor", "shortcuts", "agent", "about"]);
  for (const entry of settingsIndex) {
    assert.ok(sections.has(entry.section), `неизвестный раздел ${entry.section}`);
  }
});
