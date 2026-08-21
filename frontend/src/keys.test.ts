import assert from "node:assert/strict";
import { test } from "node:test";

import { combination, resolveCommand, type KeyEventLike } from "./keys.ts";

const press = (key: string, mods: Partial<KeyEventLike> = {}): KeyEventLike => ({
  key, metaKey: false, ctrlKey: false, altKey: false, shiftKey: false, ...mods,
});

test("простые клавиши приводятся к нижнему регистру", () => {
  assert.equal(combination(press("j")), "j");
  assert.equal(combination(press("J", { shiftKey: true })), "j");
});

test("модификаторы идут в постоянном порядке", () => {
  assert.equal(combination(press("n", { metaKey: true })), "cmd+n");
  assert.equal(combination(press("1", { metaKey: true, ctrlKey: true })), "cmd+ctrl+1");
  // Порядок в событии значения не имеет — только зафиксированный.
  assert.equal(combination(press("d", { ctrlKey: true, metaKey: true, altKey: true })), "cmd+ctrl+alt+d");
});

test("именованные клавиши получают короткие имена", () => {
  assert.equal(combination(press("Backspace", { metaKey: true })), "cmd+backspace");
  assert.equal(combination(press("Enter")), "enter");
  assert.equal(combination(press("ArrowDown")), "down");
  assert.equal(combination(press("Escape")), "escape");
  assert.equal(combination(press(" ")), "space");
});

// Shift у символов не пишем: cmd+shift+/ и cmd+? — одно нажатие.
test("shift пишется только у именованных клавиш", () => {
  assert.equal(combination(press("?", { metaKey: true, shiftKey: true })), "cmd+?");
  assert.equal(combination(press("Enter", { shiftKey: true })), "shift+enter");
});

const keymap = {
  global: { "cmd+n": "note.create" },
  "note-list": { j: "list.down", "cmd+backspace": "note.trash" },
  editor: {},
};

test("команда ищется в первом подходящем контексте", () => {
  assert.equal(resolveCommand(keymap, ["note-list", "global"], press("j")), "list.down");
  assert.equal(resolveCommand(keymap, ["note-list", "global"], press("n", { metaKey: true })), "note.create");
});

// Главное свойство: в тексте клавиши списка не срабатывают.
test("в редакторе клавиши списка не перехватываются", () => {
  assert.equal(resolveCommand(keymap, ["editor", "global"], press("j")), undefined);
  assert.equal(resolveCommand(keymap, ["editor", "global"], press("Backspace", { metaKey: true })), undefined);
  // А глобальные — работают.
  assert.equal(resolveCommand(keymap, ["editor", "global"], press("n", { metaKey: true })), "note.create");
});

test("незнакомое сочетание не даёт команды", () => {
  assert.equal(resolveCommand(keymap, ["note-list", "global"], press("z", { altKey: true })), undefined);
});
