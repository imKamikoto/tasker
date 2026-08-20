import assert from "node:assert/strict";
import { test } from "node:test";

import { filterPaths, movePick } from "./picker.ts";

const paths = ["", "Личное", "Личное/Покупки", "Работа", "Работа/Баги"];

test("пустой запрос показывает всё", () => {
  assert.deepEqual(filterPaths(paths, ""), paths);
  assert.deepEqual(filterPaths(paths, "   "), paths);
});

test("подстрока ищется в любом месте пути и без учёта регистра", () => {
  assert.deepEqual(filterPaths(paths, "раб"), ["Работа", "Работа/Баги"]);
  assert.deepEqual(filterPaths(paths, "РАБ"), ["Работа", "Работа/Баги"]);
  assert.deepEqual(filterPaths(paths, "баги"), ["Работа/Баги"]);
  assert.deepEqual(filterPaths(paths, "покуп"), ["Личное/Покупки"]);
});

test("несовпадение даёт пусто", () => {
  assert.deepEqual(filterPaths(paths, "зззз"), []);
});

test("выбор не выходит за края", () => {
  assert.equal(movePick(3, 0, -1), 0);
  assert.equal(movePick(3, 2, 1), 2);
  assert.equal(movePick(3, 1, 1), 2);
  assert.equal(movePick(3, 1, -1), 0);
  assert.equal(movePick(0, 0, 1), 0);
});
