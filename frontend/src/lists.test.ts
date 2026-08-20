import assert from "node:assert/strict";
import { test } from "node:test";

import { continueList } from "./lists.ts";

test("маркеры продолжаются", () => {
  assert.deepEqual(continueList("- пункт"), { kind: "continue", insert: "\n- " });
  assert.deepEqual(continueList("* пункт"), { kind: "continue", insert: "\n* " });
  assert.deepEqual(continueList("+ пункт"), { kind: "continue", insert: "\n+ " });
});

test("нумерация возрастает", () => {
  assert.deepEqual(continueList("1. раз"), { kind: "continue", insert: "\n2. " });
  assert.deepEqual(continueList("9. девять"), { kind: "continue", insert: "\n10. " });
  assert.deepEqual(continueList("3) три"), { kind: "continue", insert: "\n4) " });
});

test("чекбокс продолжается пустым", () => {
  assert.deepEqual(continueList("- [ ] не сделано"), { kind: "continue", insert: "\n- [ ] " });
  assert.deepEqual(continueList("- [x] сделано"), { kind: "continue", insert: "\n- [ ] " });
  assert.deepEqual(continueList("- [X] сделано"), { kind: "continue", insert: "\n- [ ] " });
});

test("отступ сохраняется", () => {
  assert.deepEqual(continueList("    - вложенный"), { kind: "continue", insert: "\n    - " });
  assert.deepEqual(continueList("\t- таб"), { kind: "continue", insert: "\n\t- " });
  assert.deepEqual(continueList("  1. вложенная"), { kind: "continue", insert: "\n  2. " });
});

test("расстояние после маркера сохраняется", () => {
  assert.deepEqual(continueList("-   широко"), { kind: "continue", insert: "\n-   " });
});

test("пустой пункт заканчивает список", () => {
  assert.deepEqual(continueList("- "), { kind: "clear" });
  assert.deepEqual(continueList("1. "), { kind: "clear" });
  assert.deepEqual(continueList("- [ ] "), { kind: "clear" });
  assert.deepEqual(continueList("    - "), { kind: "clear" });
});

test("обычный текст не трогаем", () => {
  assert.equal(continueList("просто строка"), null);
  assert.equal(continueList(""), null);
  assert.equal(continueList("# заголовок"), null);
  assert.equal(continueList("> цитата"), null);
  // Дефис без пробела — это не список, а слово.
  assert.equal(continueList("-слово"), null);
  // Горизонтальная линия тоже не список.
  assert.equal(continueList("---"), null);
});
