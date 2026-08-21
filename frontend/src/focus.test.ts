import assert from "node:assert/strict";
import { test } from "node:test";

import { focusCommands, movePane, panes, stealsFromEditor, type Pane } from "./focus.ts";

test("порядок колонок совпадает с порядком на экране", () => {
  assert.deepEqual([...panes], ["sidebar", "list", "editor"]);
});

test("вперёд идём слева направо", () => {
  assert.equal(movePane("sidebar", 1), "list");
  assert.equal(movePane("list", 1), "editor");
});

test("назад — справа налево", () => {
  assert.equal(movePane("editor", -1), "list");
  assert.equal(movePane("list", -1), "sidebar");
});

test("круг замыкается в обе стороны", () => {
  // Иначе из редактора в сайдбар надо жать дважды в одну сторону и один раз
  // в другую — правило, которое никто не запомнит.
  assert.equal(movePane("editor", 1), "sidebar");
  assert.equal(movePane("sidebar", -1), "editor");
});

test("три шага вперёд возвращают на место", () => {
  for (const start of panes) {
    let pane: Pane = start;
    for (let i = 0; i < panes.length; i++) pane = movePane(pane, 1);
    assert.equal(pane, start, `из ${start}`);
  }
});

test("шаг назад отменяет шаг вперёд", () => {
  for (const start of panes) {
    assert.equal(movePane(movePane(start, 1), -1), start, `из ${start}`);
  }
});

test("команды смены фокуса знают своё направление", () => {
  assert.equal(focusCommands["focus.next"], 1);
  assert.equal(focusCommands["focus.prev"], -1);
  assert.equal(focusCommands["note.create"], undefined);
});

test("на территории редактора при наборе не перехватываем", () => {
  // Одинокий Ctrl с буквой принадлежит тексту: Ctrl+H — backspace,
  // Ctrl+K — до конца строки. Отбирать их во время набора значит молча
  // портить текст.
  assert.equal(stealsFromEditor("ctrl+h", "INSERT"), false);
  assert.equal(stealsFromEditor("ctrl+k", "INSERT"), false);
  assert.equal(stealsFromEditor("ctrl+w", "REPLACE"), false);
  assert.equal(stealsFromEditor("ctrl+alt+h", "INSERT"), false);
});

test("вне набора перехватываем и там", () => {
  // В нормальном и визуальном эти команды редактору не нужны.
  for (const mode of ["NORMAL", "VISUAL", "VISUAL LINE", "VISUAL BLOCK"]) {
    assert.equal(stealsFromEditor("ctrl+h", mode), true, mode);
  }
});

test("shift снимает вопрос совсем", () => {
  // Ctrl+Shift+буква не занимает ни вим, ни CodeMirror — работает и при наборе,
  // то есть без предварительного Esc.
  assert.equal(stealsFromEditor("ctrl+shift+h", "INSERT"), true);
  assert.equal(stealsFromEditor("ctrl+shift+l", "INSERT"), true);
  assert.equal(stealsFromEditor("ctrl+shift+l", "NORMAL"), true);
});

test("cmd — территория приложения, а не текста", () => {
  assert.equal(stealsFromEditor("cmd+k", "INSERT"), true);
  assert.equal(stealsFromEditor("cmd+ctrl+l", "INSERT"), true);
});

test("именованные клавиши с ctrl тоже отдаём тексту при наборе", () => {
  assert.equal(stealsFromEditor("ctrl+enter", "INSERT"), false);
  assert.equal(stealsFromEditor("ctrl+enter", "NORMAL"), true);
});

test("режим в любом регистре понимается одинаково", () => {
  assert.equal(stealsFromEditor("ctrl+h", "insert"), false);
  assert.equal(stealsFromEditor("ctrl+h", "normal"), true);
});
