import assert from "node:assert/strict";
import { test } from "node:test";

import { applyAlert, lineRange, toggleFence, toggleInline } from "./markup.ts";

/** Применяет замену и возвращает получившийся текст с отмеченным выделением. */
function apply(doc: string, r: { from: number; to: number; insert: string; select: { from: number; to: number } }) {
  const next = doc.slice(0, r.from) + r.insert + doc.slice(r.to);
  return {
    text: next,
    selected: next.slice(r.select.from, r.select.to),
  };
}

test("выделение оборачивается маркерами", () => {
  const doc = "слово в тексте";
  const got = apply(doc, toggleInline(doc, 0, 5, "bold"));
  assert.equal(got.text, "**слово** в тексте");
  // Выделено остаётся само слово, а не звёздочки: следующее нажатие снимет.
  assert.equal(got.selected, "слово");
});

test("повторное нажатие снимает маркеры изнутри выделения", () => {
  const doc = "**слово** в тексте";
  const got = apply(doc, toggleInline(doc, 0, 9, "bold"));
  assert.equal(got.text, "слово в тексте");
  assert.equal(got.selected, "слово");
});

// Двойной щелчок выделяет слово, а не звёздочки вокруг него — без этого
// снять болд с уже размеченного слова было бы нельзя.
test("маркеры снаружи выделения тоже снимаются", () => {
  const doc = "**слово** в тексте";
  const got = apply(doc, toggleInline(doc, 2, 7, "bold"));
  assert.equal(got.text, "слово в тексте");
  assert.equal(got.selected, "слово");
});

test("пустое выделение ставит каретку между маркерами", () => {
  const doc = "";
  const r = toggleInline(doc, 0, 0, "italic");
  assert.equal(apply(doc, r).text, "**");
  assert.equal(r.select.from, 1);
  assert.equal(r.select.to, 1);
});

test("курсив не путается с болдом на границах", () => {
  const doc = "слово";
  assert.equal(apply(doc, toggleInline(doc, 0, 5, "italic")).text, "*слово*");
  // Одна звёздочка снаружи — курсив, снимается именно он.
  const italic = "*слово*";
  assert.equal(apply(italic, toggleInline(italic, 1, 6, "italic")).text, "слово");
});

test("остальные маркеры работают тем же правилом", () => {
  const doc = "текст";
  assert.equal(apply(doc, toggleInline(doc, 0, 5, "strike")).text, "~~текст~~");
  assert.equal(apply(doc, toggleInline(doc, 0, 5, "mark")).text, "==текст==");
  assert.equal(apply(doc, toggleInline(doc, 0, 5, "code")).text, "`текст`");
});

test("границы строк расширяются до целых", () => {
  const doc = "первая\nвторая\nтретья";
  assert.deepEqual(lineRange(doc, 8, 10), { from: 7, to: 13 });
  assert.deepEqual(lineRange(doc, 0, 0), { from: 0, to: 6 });
  // Выделение через строки берёт обе целиком.
  assert.deepEqual(lineRange(doc, 3, 9), { from: 0, to: 13 });
});

test("блок кода заворачивает целые строки", () => {
  const doc = "код тут";
  const got = apply(doc, toggleFence(doc, 2, 4));
  assert.equal(got.text, "```\nкод тут\n```");
  // Выделен сам текст, а не ограда: следующее нажатие снимает блок.
  assert.equal(got.selected, "код тут");
});

test("повторное нажатие снимает ограду", () => {
  const doc = "```\nкод тут\n```";
  const got = apply(doc, toggleFence(doc, 4, 11));
  assert.equal(got.text, "код тут");
});

test("алерт размечает выделенные строки", () => {
  const doc = "первая\nвторая";
  const got = apply(doc, applyAlert(doc, 0, doc.length, "WARNING"));
  assert.equal(got.text, "> [!WARNING]\n> первая\n> вторая");
});

// Нажать «Важно» на «Совете» — это «пусть будет важно», а не «процитируй».
test("алерт меняет вид, а не вкладывается", () => {
  const doc = "> [!TIP]\n> текст";
  const got = apply(doc, applyAlert(doc, 0, doc.length, "IMPORTANT"));
  assert.equal(got.text, "> [!IMPORTANT]\n> текст");
});

test("пустая строка внутри алерта не тащит лишний пробел", () => {
  const doc = "первая\n\nвторая";
  const got = apply(doc, applyAlert(doc, 0, doc.length, "NOTE"));
  assert.equal(got.text, "> [!NOTE]\n> первая\n>\n> вторая");
});
