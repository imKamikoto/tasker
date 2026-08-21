import assert from "node:assert/strict";
import { test } from "node:test";

import { findCheckboxes } from "./checkboxes.ts";

/** Удобная запись ожидаемого: что именно попало под подсветку. */
function marks(text: string): { text: string; done: boolean }[] {
  return findCheckboxes(text).map((box) => ({
    text: text.slice(box.from, box.to),
    done: box.done,
  }));
}

test("открытый и закрытый различаются", () => {
  assert.deepEqual(marks("- [ ] сделать\n- [x] сделано"), [
    { text: "- [ ]", done: false },
    { text: "- [x]", done: true },
  ]);
});

test("заглавная X считается закрытым", () => {
  assert.deepEqual(marks("- [X] сделано"), [{ text: "- [X]", done: true }]);
});

test("маркер списка любой из трёх", () => {
  assert.deepEqual(marks("* [ ] раз\n+ [ ] два\n- [ ] три").length, 3);
});

test("вложенные пункты подсвечиваются, отступ не попадает под маркер", () => {
  const text = "- [ ] верх\n    - [x] низ";
  const found = findCheckboxes(text);
  assert.equal(found.length, 2);
  assert.equal(text.slice(found[1].from, found[1].to), "- [x]");
});

test("табуляция в отступе не ломает разбор", () => {
  const text = "\t- [x] с табом";
  const found = findCheckboxes(text);
  assert.equal(found.length, 1);
  assert.equal(text.slice(found[0].from, found[0].to), "- [x]");
});

test("скобки посреди строки — это текст", () => {
  assert.deepEqual(marks("см. ссылку - [x] здесь"), []);
  assert.deepEqual(marks("массив a[x] и b[ ]"), []);
});

test("маркер без пробела после скобок не считается", () => {
  // `- [x]готово` — это не пункт списка, а строка с опечаткой.
  assert.deepEqual(marks("- [x]готово"), []);
});

test("обычный пункт списка не трогаем", () => {
  assert.deepEqual(marks("- просто пункт\n1. нумерованный"), []);
});

test("что-то кроме пробела и x внутри скобок не считается", () => {
  assert.deepEqual(marks("- [-] отменено\n- [?] спорно"), []);
});

test("смещение переводит координаты в документные", () => {
  const found = findCheckboxes("- [ ] раз", 100);
  assert.deepEqual(found, [{ from: 100, to: 105, done: false }]);
});

test("смещение учитывает переводы строк внутри куска", () => {
  const found = findCheckboxes("текст\n- [x] два", 50);
  assert.equal(found.length, 1);
  // «текст» — 5 символов плюс перевод строки, значит маркер начинается с 56-го.
  assert.equal(found[0].from, 56);
});

test("пустой текст ничего не даёт", () => {
  assert.deepEqual(findCheckboxes(""), []);
});
