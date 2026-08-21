import assert from "node:assert/strict";
import { test } from "node:test";

import { applyClick } from "./selection.ts";

const order = ["a", "b", "c", "d", "e"];
const click = (over: Partial<Parameters<typeof applyClick>[0]>) =>
  applyClick({ order, selected: [], anchor: null, clicked: "a", toggle: false, range: false, ...over });

test("обычный щелчок оставляет одну заметку", () => {
  const got = click({ selected: ["a", "b", "c"], anchor: "a", clicked: "d" });
  assert.deepEqual(got, { selected: ["d"], anchor: "d" });
});

test("Cmd добавляет и убирает по одной", () => {
  const added = click({ selected: ["a"], anchor: "a", clicked: "c", toggle: true });
  assert.deepEqual(added.selected, ["a", "c"]);

  const removed = click({ selected: ["a", "c"], anchor: "a", clicked: "a", toggle: true });
  assert.deepEqual(removed.selected, ["c"]);
});

test("Shift выделяет диапазон в порядке экрана", () => {
  const got = click({ selected: ["b"], anchor: "b", clicked: "d", range: true });
  assert.deepEqual(got.selected, ["b", "c", "d"]);
});

test("Shift работает и вверх", () => {
  const got = click({ selected: ["d"], anchor: "d", clicked: "b", range: true });
  assert.deepEqual(got.selected, ["b", "c", "d"]);
});

// Ловушка: без этого второй Shift+щелчок тянул бы диапазон от предыдущего,
// и выделение «схлопывалось» бы шагами.
test("несколько Shift+щелчков тянут от одного якоря", () => {
  const first = click({ selected: ["b"], anchor: "b", clicked: "d", range: true });
  const second = applyClick({
    order, selected: first.selected, anchor: first.anchor, clicked: "e", toggle: false, range: true,
  });
  assert.deepEqual(second.selected, ["b", "c", "d", "e"]);
  assert.equal(second.anchor, "b");
});

test("Shift без якоря ведёт себя как обычный щелчок", () => {
  const got = click({ selected: [], anchor: null, clicked: "c", range: true });
  assert.deepEqual(got, { selected: ["c"], anchor: "c" });
});

test("Shift на исчезнувший якорь не ломается", () => {
  const got = click({ selected: [], anchor: "которого-нет", clicked: "c", range: true });
  assert.deepEqual(got.selected, ["c"]);
});

test("Cmd переносит якорь на последний тронутый", () => {
  const got = click({ selected: ["a"], anchor: "a", clicked: "d", toggle: true });
  assert.equal(got.anchor, "d");
});
