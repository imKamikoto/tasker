import assert from "node:assert/strict";
import { test } from "node:test";

import { backlinkCount, fileSize, noteCount, noteCountTarget, shortDate } from "./format.ts";

test("склонение заметок по последней цифре", () => {
  assert.equal(noteCount(0), "0 заметок");
  assert.equal(noteCount(1), "1 заметка");
  assert.equal(noteCount(2), "2 заметки");
  assert.equal(noteCount(4), "4 заметки");
  assert.equal(noteCount(5), "5 заметок");
  assert.equal(noteCount(21), "21 заметка");
  assert.equal(noteCount(102), "102 заметки");
});

test("одиннадцать-четырнадцать — исключение", () => {
  // По последней цифре 11 дало бы «11 заметка», а 12 — «12 заметки».
  assert.equal(noteCount(11), "11 заметок");
  assert.equal(noteCount(12), "12 заметок");
  assert.equal(noteCount(14), "14 заметок");
  assert.equal(noteCount(111), "111 заметок");
});

test("винительный падеж отличается только в единственном числе", () => {
  assert.equal(noteCountTarget(1), "1 заметку");
  assert.equal(noteCountTarget(21), "21 заметку");
  // Всё остальное совпадает с именительным.
  for (const n of [0, 2, 4, 5, 11, 12, 14, 102, 111]) {
    assert.equal(noteCountTarget(n), noteCount(n), `на ${n}`);
  }
});

test("склонение бэклинков по тому же правилу", () => {
  assert.equal(backlinkCount(1), "1 бэклинк");
  assert.equal(backlinkCount(2), "2 бэклинка");
  assert.equal(backlinkCount(5), "5 бэклинков");
  assert.equal(backlinkCount(13), "13 бэклинков");
});

test("сегодняшнее показывается временем", () => {
  const now = new Date(2026, 7, 21, 18, 30);
  const today = new Date(2026, 7, 21, 9, 41).toISOString();
  assert.match(shortDate(today, now), /^\d{2}:\d{2}$/);
});

test("вчерашнее показывается датой", () => {
  const now = new Date(2026, 7, 21, 0, 5);
  const yesterday = new Date(2026, 7, 20, 23, 55).toISOString();
  // Разница меньше десяти минут, но день другой — значит дата, а не время.
  assert.doesNotMatch(shortDate(yesterday, now), /^\d{2}:\d{2}$/);
});

test("тот же день год назад — не сегодня", () => {
  const now = new Date(2026, 7, 21, 12, 0);
  const lastYear = new Date(2025, 7, 21, 12, 0).toISOString();
  assert.doesNotMatch(shortDate(lastYear, now), /^\d{2}:\d{2}$/);
});

test("непонятное значение не роняет строку", () => {
  assert.equal(shortDate(""), "");
  assert.equal(shortDate("не дата"), "");
});

test("размер файла печатается человеческими единицами", () => {
  assert.equal(fileSize(0), "—");
  assert.equal(fileSize(-5), "—");
  assert.equal(fileSize(512), "512 Б");
  assert.equal(fileSize(1024), "1.0 КБ");
  assert.equal(fileSize(33_554_432), "32 МБ");
  assert.equal(fileSize(1_610_612_736), "1.5 ГБ");
});

test("размер не срывается в неразличимые единицы", () => {
  // 1.4 и 1.9 МБ отличаются в полтора раза — округлить их в одно «1 МБ» нельзя.
  assert.notEqual(fileSize(1_468_006), fileSize(1_992_294));
});
