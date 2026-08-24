import assert from "node:assert/strict";
import { test } from "node:test";

import { move, topRows, type TopKind } from "./toprows.ts";

test("порядок по умолчанию — как было до настройки", () => {
  assert.deepEqual(topRows([], [], true), ["active", "all", "agent"]);
});

test("порядок из настроек соблюдается", () => {
  assert.deepEqual(topRows(["all", "active"], [], false), ["all", "active"]);
});

// Испорченный или устаревший config.json не должен прятать пункты навсегда:
// сама настройка лежит в том же файле, и вернуть их было бы нечем.
test("незнакомое имя выбрасывается, недостающее дописывается", () => {
  assert.deepEqual(topRows(["телепорт", "all"], [], true), ["all", "active", "agent"]);
});

test("повтор в настройках не задваивает пункт", () => {
  assert.deepEqual(topRows(["all", "all"], [], false), ["all", "active"]);
});

test("спрятанное не показывается", () => {
  assert.deepEqual(topRows([], ["all"], true), ["active", "agent"]);
});

// «От агента» появляется, только когда агент что-то написал. Спрятать руками
// можно, а появиться сам в чужом хранилище он не должен.
test("пункт агента зависит от того, писал ли агент", () => {
  assert.ok(topRows([], [], true).includes("agent"));
  assert.ok(!topRows([], [], false).includes("agent"));
  assert.ok(!topRows([], ["agent"], true).includes("agent"));
});

test("перестановка сдвигает на одну позицию", () => {
  const order: TopKind[] = ["active", "all", "agent"];
  assert.deepEqual(move(order, "all", -1), ["all", "active", "agent"]);
  assert.deepEqual(move(order, "all", 1), ["active", "agent", "all"]);
});

test("края никуда не двигаются", () => {
  const order: TopKind[] = ["active", "all"];
  assert.deepEqual(move(order, "active", -1), order);
  assert.deepEqual(move(order, "all", 1), order);
});

test("перестановка не трогает исходный список", () => {
  const order: TopKind[] = ["active", "all"];
  move(order, "all", -1);
  assert.deepEqual(order, ["active", "all"]);
});
