import assert from "node:assert/strict";
import { test } from "node:test";

import { statusForCommand, statuses } from "./statuses.ts";

test("у каждого статуса есть команда", () => {
  for (const status of statuses) {
    assert.equal(statusForCommand(`note.status.${status.toLowerCase()}`), status);
  }
  assert.equal(statuses.length, 5);
});

// Имена в keymap.json пишутся руками, поэтому нижний регистр обязан работать.
test("onhold находит onHold", () => {
  assert.equal(statusForCommand("note.status.onhold"), "onHold");
});

test("чужие команды статуса не дают", () => {
  for (const command of ["note.create", "list.down", "note.status.", "note.status.выдуманный", ""]) {
    assert.equal(statusForCommand(command), undefined, `команда ${command}`);
  }
});
