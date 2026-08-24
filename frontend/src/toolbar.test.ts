import assert from "node:assert/strict";
import { test } from "node:test";

import { toolbarPosition } from "./toolbar.ts";

// Коробка — колонка редактора, поэтому и клампинг по её ширине.
const container = { width: 1200, height: 800 };
const toolbar = { width: 300, height: 30 };

test("тулбар встаёт над выделением, когда сверху есть место", () => {
  const got = toolbarPosition({ top: 400, bottom: 420, left: 600 }, container, toolbar);
  assert.equal(got.top, 400 - 30 - 8);
  // По горизонтали центрируется на начале выделения.
  assert.equal(got.left, 600 - 150);
});

// Выделение под самой верхней кромкой: над ним тулбар ушёл бы под полосу
// перетаскивания окна, где его не видно и не нажать.
test("места сверху нет — уходит под выделение", () => {
  const got = toolbarPosition({ top: 50, bottom: 70, left: 600 }, container, toolbar);
  assert.equal(got.top, 70 + 8);
});

test("тулбар не вылезает за края колонки", () => {
  const left = toolbarPosition({ top: 400, bottom: 420, left: 5 }, container, toolbar);
  assert.equal(left.left, 8);

  const right = toolbarPosition({ top: 400, bottom: 420, left: 1190 }, container, toolbar);
  assert.equal(right.left, 1200 - 300 - 8);
});

// Окно уже самого тулбара — не повод уехать в отрицательные координаты.
test("узкая колонка прижимает тулбар к левому краю", () => {
  const got = toolbarPosition({ top: 400, bottom: 420, left: 100 }, { width: 200, height: 800 }, toolbar);
  assert.equal(got.left, 8);
});
