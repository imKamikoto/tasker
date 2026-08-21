import assert from "node:assert/strict";
import { test } from "node:test";

import { progressBar } from "./progress.ts";

test("прогресс не рисуется, когда задач нет", () => {
  assert.equal(progressBar(0, 0), "");
  assert.equal(progressBar(3, 0), "");
});

test("полоса всегда нужной ширины", () => {
  for (let done = 0; done <= 7; done++) {
    assert.equal(progressBar(done, 7).length, 7, `done=${done}`);
  }
});

test("сделано всё — полоса закрашена целиком", () => {
  assert.equal(progressBar(7, 7), "▓▓▓▓▓▓▓");
  assert.equal(progressBar(6, 6), "▓▓▓▓▓▓▓");
});

test("не сделано ничего — полоса пустая", () => {
  assert.equal(progressBar(0, 7), "░░░░░░░");
});

test("начатое видно даже при округлении вниз", () => {
  // 1 из 100 — это 0.07 клетки, но работа началась, и это должно быть видно.
  assert.equal(progressBar(1, 100), "▓░░░░░░");
});

test("незаконченное не выглядит законченным", () => {
  // 99 из 100 округляются до всех семи клеток — последнюю придерживаем.
  assert.equal(progressBar(99, 100), "▓▓▓▓▓▓░");
});

test("пример из макета", () => {
  assert.equal(progressBar(3, 7), "▓▓▓░░░░");
});

test("значения вне диапазона не ломают полосу", () => {
  assert.equal(progressBar(-5, 7), "░░░░░░░");
  assert.equal(progressBar(50, 7), "▓▓▓▓▓▓▓");
});
