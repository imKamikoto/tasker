import assert from "node:assert/strict";
import { test } from "node:test";

import { bindingsFor, checkBinding, commands, prettyCombo } from "./commands.ts";

test("в каталоге нет повторов и пустых имён", () => {
  const seen = new Set<string>();
  for (const command of commands) {
    assert.ok(command.id !== "", "пустой id");
    assert.ok(command.label !== "", `у ${command.id} нет подписи`);
    assert.ok(!seen.has(command.id), `команда ${command.id} перечислена дважды`);
    seen.add(command.id);
  }
});

test("свободное сочетание конфликтов не даёт", () => {
  assert.deepEqual(checkBinding({}, "note-list", "cmd+shift+k", "note.pin"), { kind: "none" });
});

test("занятое сочетание называет, кем занято", () => {
  const bindings = { "cmd+d": "note.duplicate" };
  assert.deepEqual(checkBinding(bindings, "note-list", "cmd+d", "note.pin"), {
    kind: "taken",
    command: "note.duplicate",
  });
});

test("переназначение на себя же конфликтом не считается", () => {
  const bindings = { "cmd+d": "note.duplicate" };
  assert.deepEqual(checkBinding(bindings, "note-list", "cmd+d", "note.duplicate"), {
    kind: "none",
  });
});

test("однобуквенная привязка в тексте отнимает клавишу у вима", () => {
  const got = checkBinding({}, "editor", "j", "list.down");
  assert.deepEqual(got, { kind: "vim", key: "j" });
});

test("в списке те же буквы вима не трогают", () => {
  // Ради этого контексты и заведены: j в списке — движение, в тексте — вим.
  assert.deepEqual(checkBinding({}, "note-list", "j", "list.down"), { kind: "none" });
  assert.deepEqual(checkBinding({}, "global", "p", "note.pin"), { kind: "none" });
});

test("модификатор снимает спор с вимом", () => {
  assert.deepEqual(checkBinding({}, "editor", "cmd+j", "list.down"), { kind: "none" });
  assert.deepEqual(checkBinding({}, "editor", "alt+d", "note.duplicate"), { kind: "none" });
});

test("занятость важнее вима", () => {
  // Иначе человек увидит предупреждение про вим и не узнает, что клавиша
  // вдобавок отберётся у другой команды.
  const got = checkBinding({ j: "list.down" }, "editor", "j", "note.pin");
  assert.deepEqual(got, { kind: "taken", command: "list.down" });
});

test("сочетание печатается глифами мака", () => {
  assert.equal(prettyCombo("cmd+n"), "⌘N");
  assert.equal(prettyCombo("cmd+ctrl+1"), "⌘⌃1");
  assert.equal(prettyCombo("cmd+backspace"), "⌘⌫");
  assert.equal(prettyCombo("enter"), "⏎");
  assert.equal(prettyCombo("j"), "J");
  assert.equal(prettyCombo(""), "—");
});

test("все сочетания команды находятся и идут по порядку", () => {
  const keymap = {
    "note-list": { j: "list.down", down: "list.down", k: "list.up" },
  };
  assert.deepEqual(bindingsFor(keymap, "note-list", "list.down"), ["down", "j"]);
  assert.deepEqual(bindingsFor(keymap, "note-list", "list.up"), ["k"]);
  assert.deepEqual(bindingsFor(keymap, "note-list", "note.pin"), []);
  // Несуществующий контекст не роняет разбор.
  assert.deepEqual(bindingsFor(keymap, "editor", "list.down"), []);
});
