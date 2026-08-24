import assert from "node:assert/strict";
import { test } from "node:test";

import {
  combination,
  resolveCommand,
  withoutVimMotions,
  type KeyEventLike,
} from "./keys.ts";

const press = (key: string, mods: Partial<KeyEventLike> = {}): KeyEventLike => ({
  key, metaKey: false, ctrlKey: false, altKey: false, shiftKey: false, ...mods,
});

test("простые клавиши приводятся к нижнему регистру", () => {
  assert.equal(combination(press("j")), "j");
  // Регистр самой буквы не важен, а вот shift теперь записывается: в виме
  // J — это соединить строки, а не то же самое, что j.
  assert.equal(combination(press("J", { shiftKey: true })), "shift+j");
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
test("shift пишется у букв и именованных клавиш, но не у символов", () => {
  assert.equal(combination(press("?", { metaKey: true, shiftKey: true })), "cmd+?");
  assert.equal(combination(press("Enter", { shiftKey: true })), "shift+enter");
  assert.equal(combination(press("H", { ctrlKey: true, shiftKey: true })), "ctrl+shift+h");
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

/** Нажатие с русской раскладкой: event.key приходит кириллицей. */
const ru = press;

test("кириллица приводится к латинице по физической позиции", () => {
  // Без этого при ЙЦУКЕН не работает ни одно буквенное сочетание.
  assert.equal(combination(ru("б", { metaKey: true })), "cmd+,");
  assert.equal(combination(ru("т", { metaKey: true })), "cmd+n");
  assert.equal(combination(ru("в", { metaKey: true })), "cmd+d");
  assert.equal(combination(ru("о")), "j");
  assert.equal(combination(ru("л")), "k");
  assert.equal(combination(ru("з")), "p");
  assert.equal(combination(ru("ь")), "m");
});

test("латиница от перевода не страдает", () => {
  assert.equal(combination(ru(",", { metaKey: true })), "cmd+,");
  assert.equal(combination(ru("n", { metaKey: true })), "cmd+n");
  assert.equal(combination(ru("j")), "j");
});

test("обе раскладки дают одно и то же сочетание", () => {
  // Это и есть смысл перевода: шоткат не должен зависеть от того, на каком
  // языке человек сейчас печатает.
  const pairs: [string, string][] = [
    ["б", ","], ["т", "n"], ["в", "d"], ["о", "j"], ["л", "k"], ["з", "p"], ["ь", "m"],
  ];
  for (const [cyrillic, latin] of pairs) {
    assert.equal(
      combination(ru(cyrillic, { metaKey: true })),
      combination(ru(latin, { metaKey: true })),
      `${cyrillic} и ${latin}`,
    );
  }
});

test("заглавная кириллица переводится по верхнему ряду", () => {
  // Shift+физическая запятая: в русской раскладке это Б, в английской — <.
  assert.equal(combination(ru("Б", { metaKey: true, shiftKey: true })), "cmd+<");
  assert.equal(combination(ru("<", { metaKey: true, shiftKey: true })), "cmd+<");
});

test("именованные клавиши перевод не трогает", () => {
  assert.equal(combination(ru("Enter")), "enter");
  assert.equal(combination(ru("Backspace", { metaKey: true })), "cmd+backspace");
  assert.equal(combination(ru("ArrowDown")), "down");
});

test("незнакомый символ остаётся собой", () => {
  // Таблица покрывает ЙЦУКЕН, а не все раскладки мира.
  assert.equal(combination(ru("ß", { metaKey: true })), "cmd+ß");
  assert.equal(combination(ru("1", { metaKey: true, ctrlKey: true })), "cmd+ctrl+1");
});

test("знаки препинания перевод не трогает", () => {
  // В русском ряду физическая «/» даёт «.», а с шифтом «,» — те же символы,
  // что и в латинском, но на другой позиции. Перевод сломал бы обычную
  // запятую: Cmd+, стал бы Cmd+? на английской раскладке.
  for (const sign of [",", ".", "/", ";", "'", "[", "]", "<", ">", "?"]) {
    assert.equal(combination(press(sign, { metaKey: true })), `cmd+${sign}`, sign);
  }
});

test("shift у букв различает нажатия", () => {
  // Без этого Ctrl+Shift+H неотличим от Ctrl+H: буква всё равно приводится
  // к нижнему регистру, и назначить на них разные команды было бы нельзя.
  assert.equal(combination(press("H", { ctrlKey: true, shiftKey: true })), "ctrl+shift+h");
  assert.equal(combination(press("h", { ctrlKey: true })), "ctrl+h");
  assert.notEqual(
    combination(press("H", { ctrlKey: true, shiftKey: true })),
    combination(press("h", { ctrlKey: true })),
  );
});

test("shift у символов по-прежнему не пишется", () => {
  // Cmd+Shift+/ приходит как «?»: shift уже поменял сам символ, и записывать
  // его вторым способом значит развести одно нажатие на две записи в файле.
  assert.equal(combination(press("?", { metaKey: true, shiftKey: true })), "cmd+?");
  assert.equal(combination(press("!", { metaKey: true, shiftKey: true })), "cmd+!");
  assert.equal(combination(press("<", { ctrlKey: true, shiftKey: true })), "ctrl+<");
});

test("shift у именованных клавиш пишется", () => {
  assert.equal(combination(press("Enter", { shiftKey: true })), "shift+enter");
  assert.equal(combination(press("Tab", { shiftKey: true })), "shift+tab");
});

test("shift и кириллица вместе", () => {
  // Русская «Б» — буква, но стоит на физической запятой и переводится в «<»,
  // где shift уже учтён. Обе раскладки обязаны дать одно и то же.
  assert.equal(combination(press("Б", { ctrlKey: true, shiftKey: true })), "ctrl+<");
  assert.equal(
    combination(press("Р", { ctrlKey: true, shiftKey: true })),
    combination(press("H", { ctrlKey: true, shiftKey: true })),
  );
});

// Выключенная вим-навигация снимает буквы, но не стрелки и не мнемоники.
test("вимовые движения снимаются, остальное остаётся", () => {
  const keymap = {
    global: { "cmd+n": "note.create" },
    sidebar: {
      j: "sidebar.down",
      k: "sidebar.up",
      h: "sidebar.collapse",
      l: "sidebar.expand",
      down: "sidebar.down",
      up: "sidebar.up",
      left: "sidebar.collapse",
      right: "sidebar.expand",
      enter: "sidebar.open",
    },
    "note-list": {
      j: "list.down",
      k: "list.up",
      down: "list.down",
      up: "list.up",
      enter: "list.open",
      p: "note.pin",
      m: "note.move",
      "cmd+d": "note.duplicate",
    },
  };

  const got = withoutVimMotions(keymap);

  assert.deepEqual(got["note-list"], {
    down: "list.down",
    up: "list.up",
    enter: "list.open",
    p: "note.pin",
    m: "note.move",
    "cmd+d": "note.duplicate",
  });
  assert.deepEqual(got.sidebar, {
    down: "sidebar.down",
    up: "sidebar.up",
    left: "sidebar.collapse",
    right: "sidebar.expand",
    enter: "sidebar.open",
  });
  // Глобальный контекст движений не содержит и трогать его незачем.
  assert.deepEqual(got.global, { "cmd+n": "note.create" });
});

// Исходную раскладку показывает экран шоткатов — портить её нельзя.
test("снятие движений не трогает исходную раскладку", () => {
  const keymap = { "note-list": { j: "list.down", down: "list.down" } };
  withoutVimMotions(keymap);
  assert.deepEqual(keymap, { "note-list": { j: "list.down", down: "list.down" } });
});

// Снимается позиция, а не команда: переназначенное на j остаётся снятым,
// а та же команда на стрелке продолжает работать.
test("движение снимается по клавише, а не по команде", () => {
  const got = withoutVimMotions({ "note-list": { j: "note.pin", down: "list.down" } });
  assert.deepEqual(got["note-list"], { down: "list.down" });
});
