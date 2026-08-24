import assert from "node:assert/strict";
import { test } from "node:test";

import { spansLines } from "./indent.ts";

test("выделение внутри одной строки — не отступ", () => {
  assert.equal(spansLines([{ fromLine: 3, toLine: 3 }]), false);
});

test("пустое выделение — не отступ: там Tab печатает", () => {
  assert.equal(spansLines([{ fromLine: 1, toLine: 1 }]), false);
  assert.equal(spansLines([]), false);
});

test("выделение на две строки и больше — отступ", () => {
  assert.equal(spansLines([{ fromLine: 2, toLine: 3 }]), true);
  assert.equal(spansLines([{ fromLine: 1, toLine: 40 }]), true);
});

// Мультикурсор CodeMirror: хватает одного куска через строки, чтобы нажатие
// стало отступом — иначе один курсор из десяти решал бы за всех.
test("достаточно одного многострочного куска", () => {
  assert.equal(
    spansLines([
      { fromLine: 1, toLine: 1 },
      { fromLine: 5, toLine: 7 },
    ]),
    true,
  );
  assert.equal(
    spansLines([
      { fromLine: 1, toLine: 1 },
      { fromLine: 5, toLine: 5 },
    ]),
    false,
  );
});
