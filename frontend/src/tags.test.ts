import assert from "node:assert/strict";
import { test } from "node:test";

import { addTag, suggestTags, tagColor, tagPalette } from "./tags.ts";

const all = ["баг", "работа", "срочно", "черновик", "bug"];

test("подсказки не повторяют уже проставленные", () => {
  assert.deepEqual(suggestTags(all, ["баг", "BUG"], ""), ["работа", "срочно", "черновик"]);
});

test("подсказки ищут подстроку без учёта регистра", () => {
  assert.deepEqual(suggestTags(all, [], "раб"), ["работа"]);
  assert.deepEqual(suggestTags(all, [], "РАБ"), ["работа"]);
  assert.deepEqual(suggestTags(all, [], "чно"), ["срочно"]);
  assert.deepEqual(suggestTags(all, [], "ззз"), []);
});

test("подсказок не больше предела", () => {
  assert.equal(suggestTags(Array.from({ length: 50 }, (_, i) => `тег${i}`), [], "", 8).length, 8);
});

test("добавление отбрасывает пустое и повторы", () => {
  assert.deepEqual(addTag(["баг"], "работа"), ["баг", "работа"]);
  assert.deepEqual(addTag(["баг"], "  "), ["баг"]);
  assert.deepEqual(addTag(["баг"], "БАГ"), ["баг"], "повтор в другом регистре");
  assert.deepEqual(addTag(["баг"], " работа "), ["баг", "работа"], "края обрезаются");
});

test("повтор возвращает тот же массив, чтобы не писать зря", () => {
  const current = ["баг"];
  assert.equal(addTag(current, "баг"), current);
  assert.notEqual(addTag(current, "новый"), current);
});

test("цвет тега постоянен и лежит в палитре", () => {
  for (const tag of all) {
    const color = tagColor(tag);
    assert.ok(color >= 0 && color < tagPalette, `${tag} → ${color}`);
    assert.equal(color, tagColor(tag), "цвет должен быть одинаковым при каждом вызове");
  }
});
