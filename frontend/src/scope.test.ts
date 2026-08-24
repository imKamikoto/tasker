import assert from "node:assert/strict";
import { test } from "node:test";

import { scopeLabel } from "./scope.ts";

test("у каждой выборки есть человеческое имя", () => {
  assert.equal(scopeLabel({ kind: "active" }), "Активные");
  assert.equal(scopeLabel({ kind: "all" }), "Все заметки");
  assert.equal(scopeLabel({ kind: "agent" }), "От агента");
  assert.equal(scopeLabel({ kind: "trash" }), "Корзина");
});

test("ноутбук показывается полным путём", () => {
  assert.equal(scopeLabel({ kind: "notebook", path: "Работа" }), "Работа");
  // Целиком, а не последним сегментом: «Баги» не отвечает, какие это баги.
  assert.equal(scopeLabel({ kind: "notebook", path: "Работа/Баги" }), "Работа/Баги");
});

test("у корня хранилища имя то же, что в дереве", () => {
  assert.equal(scopeLabel({ kind: "notebook", path: "" }), "Корень");
});

test("теги читаются как И, а не перечислением", () => {
  assert.equal(scopeLabel({ kind: "tags", names: ["баг"] }), "#баг");
  assert.equal(scopeLabel({ kind: "tags", names: ["баг", "срочно"] }), "#баг + #срочно");
});
