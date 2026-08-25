import assert from "node:assert/strict";
import { test } from "node:test";

import {
  focusCommands,
  focusRamps,
  idleRamp,
  movePane,
  panes,
  rampFor,
  rampState,
  stealsFromEditor,
  type Pane,
} from "./focus.ts";

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

// Выключенный вим приходит сюда пустым режимом: режимов у редактора больше
// нет, и текст набирают всегда.
test("без вима Ctrl-сочетания остаются редактору", () => {
  assert.equal(stealsFromEditor("ctrl+h", ""), false);
  assert.equal(stealsFromEditor("ctrl+w", ""), false);
  // Смена фокуса не на территории редактора и работает по-прежнему.
  assert.equal(stealsFromEditor("ctrl+shift+h", ""), true);
  assert.equal(stealsFromEditor("cmd+n", ""), true);
});

// Длины из FOCUS-STRIP.md: 11 у сайдбара, 17 у списка, 23 у редактора.
test("длина рампы соответствует колонке", () => {
  assert.equal([...focusRamps.sidebar].length, 11);
  assert.equal([...focusRamps.list].length, 17);
  assert.equal([...focusRamps.editor].length, 23);
});

test("рампа симметрична: градиент сходится к центру с обеих сторон", () => {
  for (const kind of panes) {
    const chars = [...focusRamps[kind]];
    assert.deepEqual(chars, [...chars].reverse(), `рампа ${kind} несимметрична`);
  }
});

test("у каждой колонки своя рампа, у неактивной — точки", () => {
  assert.equal(rampFor("list", "active"), focusRamps.list);
  assert.equal(rampFor("list", "idle"), idleRamp);
  assert.equal([...idleRamp].length, 15);
});

test("фокус в колонке — рампа, в соседних — точки", () => {
  assert.equal(rampState("list", "list", true, true), "active");
  assert.equal(rampState("sidebar", "list", true, true), "idle");
});

// Подсвеченная колонка в неактивном окне обещала бы клавиатурный фокус,
// которого у окна сейчас нет.
test("неактивное окно гасит все три колонки", () => {
  for (const kind of panes) {
    assert.equal(rampState(kind, "list", false, true), "idle");
  }
});

// Индикатор существует ради слепого переключения по ⌃⇧H и ⌃⇧L. Движений нет —
// переключения нет, и точки намекали бы на механику, которой в этом режиме
// не существует.
test("без движений вима полосы нет вовсе, а не пустая", () => {
  for (const kind of panes) {
    assert.equal(rampState(kind, kind, true, false), "hidden");
    assert.equal(rampState(kind, "list", false, false), "hidden");
  }
});
