import assert from "node:assert/strict";
import { test } from "node:test";

import { isDone, statusForKey, statuses } from "./statuses.ts";

test("цифры сопоставлены статусам по порядку из SPEC §8.3", () => {
  assert.equal(statusForKey("1"), "none");
  assert.equal(statusForKey("2"), "active");
  assert.equal(statusForKey("3"), "onHold");
  assert.equal(statusForKey("4"), "completed");
  assert.equal(statusForKey("5"), "dropped");
  assert.equal(statuses.length, 5);
});

test("посторонние клавиши не дают статуса", () => {
  for (const key of ["0", "6", "9", "a", "", "Enter", "-1"]) {
    assert.equal(statusForKey(key), undefined, `клавиша ${key}`);
  }
});

test("скрытые по умолчанию — только завершённое и брошенное", () => {
  assert.equal(isDone("completed"), true);
  assert.equal(isDone("dropped"), true);
  assert.equal(isDone("active"), false);
  assert.equal(isDone("onHold"), false);
  assert.equal(isDone("none"), false);
});
