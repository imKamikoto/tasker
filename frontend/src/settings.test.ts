import assert from "node:assert/strict";
import { test } from "node:test";

import { defaultSettings, parseSettings } from "./settings.ts";

test("нормальные настройки читаются как есть", () => {
  const got = parseSettings(
    JSON.stringify({ sidebarWidth: 240, listWidth: 400, sortField: "title", sortReversed: true, collapsed: ["Работа"] }),
  );
  assert.deepEqual(got, {
    sidebarWidth: 240, listWidth: 400, sortField: "title", sortReversed: true, collapsed: ["Работа"],
  });
});

test("мусор вместо файла даёт умолчания", () => {
  for (const raw of ["", "не json", "null", "[]", "42", '"строка"']) {
    assert.deepEqual(parseSettings(raw), defaultSettings, `на вводе ${raw}`);
  }
});

test("одно испорченное поле не утаскивает остальные", () => {
  const got = parseSettings(JSON.stringify({ sidebarWidth: "широкий", sortField: "выдуманное", collapsed: "нет" }));
  assert.equal(got.sidebarWidth, defaultSettings.sidebarWidth);
  assert.equal(got.sortField, defaultSettings.sortField);
  assert.deepEqual(got.collapsed, []);
});

test("ширины зажимаются в разумные пределы", () => {
  assert.equal(parseSettings(JSON.stringify({ sidebarWidth: -500 })).sidebarWidth, 160);
  assert.equal(parseSettings(JSON.stringify({ sidebarWidth: 99999 })).sidebarWidth, 400);
  assert.equal(parseSettings(JSON.stringify({ listWidth: 1 })).listWidth, 240);
  assert.equal(parseSettings(JSON.stringify({ sidebarWidth: 240.7 })).sidebarWidth, 241);
});

test("в списке свёрнутых остаются только строки", () => {
  const got = parseSettings(JSON.stringify({ collapsed: ["Работа", 42, null, "Личное"] }));
  assert.deepEqual(got.collapsed, ["Работа", "Личное"]);
});
