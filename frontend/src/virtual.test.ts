import assert from "node:assert/strict";
import { test } from "node:test";

import { visibleWindow } from "./virtual.ts";

const base = { rowHeight: 80, viewport: 800, overscan: 3 };

test("в начале списка окно не уходит в минус", () => {
  const w = visibleWindow({ ...base, total: 1000, scrollTop: 0 });
  assert.equal(w.first, 0);
  assert.equal(w.padTop, 0);
  assert.ok(w.end >= 10, `нарисовано ${w.end} строк на экран в 10`);
});

test("распорки складываются в полную высоту списка", () => {
  const total = 10_000;
  for (const scrollTop of [0, 4000, 123_456, 799_999]) {
    const w = visibleWindow({ ...base, total, scrollTop });
    const drawn = (w.end - w.first) * base.rowHeight;
    assert.equal(w.padTop + drawn + w.padBottom, total * base.rowHeight, `при scrollTop=${scrollTop}`);
  }
});

test("в конце списка окно не вылезает за последнюю строку", () => {
  const total = 100;
  const w = visibleWindow({ ...base, total, scrollTop: total * base.rowHeight });
  assert.equal(w.end, total);
  assert.equal(w.padBottom, 0);
  assert.ok(w.first < total);
});

test("рисуется малая доля большого списка", () => {
  const w = visibleWindow({ ...base, total: 10_000, scrollTop: 40_000 });
  const drawn = w.end - w.first;
  assert.ok(drawn < 30, `нарисовано ${drawn} строк из 10000`);
  assert.ok(drawn > 10, `нарисовано всего ${drawn} — экран не закрыт`);
});

test("список короче экрана рисуется целиком", () => {
  const w = visibleWindow({ ...base, total: 3, scrollTop: 0 });
  assert.equal(w.first, 0);
  assert.equal(w.end, 3);
  assert.equal(w.padTop, 0);
  assert.equal(w.padBottom, 0);
});

test("пустой список ничего не рисует", () => {
  const w = visibleWindow({ ...base, total: 0, scrollTop: 0 });
  assert.deepEqual(w, { first: 0, end: 0, padTop: 0, padBottom: 0 });
});

test("прокрутка за пределы списка не ломает окно", () => {
  // Так бывает, когда список укоротился, а позиция осталась прежней.
  const w = visibleWindow({ ...base, total: 5, scrollTop: 999_999 });
  assert.ok(w.first >= 0 && w.first <= 5);
  assert.equal(w.end, 5);
  assert.equal(w.padBottom, 0);
});

test("нулевая высота строки не делит на ноль", () => {
  const w = visibleWindow({ ...base, rowHeight: 0, total: 100, scrollTop: 10 });
  assert.deepEqual(w, { first: 0, end: 0, padTop: 0, padBottom: 0 });
});
