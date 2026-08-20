import assert from "node:assert/strict";
import { test } from "node:test";

import { notebookRows, type NotebookNode } from "./tree.ts";

const tree: NotebookNode[] = [
  { path: "", count: 1 },
  { path: "Личное", count: 2 },
  { path: "Работа", count: 3 },
  { path: "Работа/Баги", count: 5 },
  { path: "Работа/Баги/Старые", count: 7 },
  { path: "Работа/Задачи", count: 11 },
];

const paths = (rows: ReturnType<typeof notebookRows>) => rows.map((r) => r.path);

test("развёрнутое дерево показывает всё", () => {
  const rows = notebookRows(tree, []);
  assert.deepEqual(paths(rows), [
    "", "Личное", "Работа", "Работа/Баги", "Работа/Баги/Старые", "Работа/Задачи",
  ]);
});

test("глубина считается по пути", () => {
  const rows = notebookRows(tree, []);
  const depth = Object.fromEntries(rows.map((r) => [r.path, r.depth]));
  assert.equal(depth[""], 0);
  assert.equal(depth["Работа"], 0);
  assert.equal(depth["Работа/Баги"], 1);
  assert.equal(depth["Работа/Баги/Старые"], 2);
});

test("свёрнутый прячет потомков на любую глубину", () => {
  const rows = notebookRows(tree, ["Работа"]);
  assert.deepEqual(paths(rows), ["", "Личное", "Работа"]);
});

test("свёрнутый в середине прячет только своё поддерево", () => {
  const rows = notebookRows(tree, ["Работа/Баги"]);
  assert.deepEqual(paths(rows), ["", "Личное", "Работа", "Работа/Баги", "Работа/Задачи"]);
});

// Правило из SPEC §8.1, ради которого всё это и вынесено в функцию.
test("свёрнутый считает вложенные, развёрнутый — только свои", () => {
  const expanded = notebookRows(tree, []).find((r) => r.path === "Работа");
  assert.equal(expanded?.count, 3, "развёрнутый показывает только свои");

  const collapsed = notebookRows(tree, ["Работа"]).find((r) => r.path === "Работа");
  assert.equal(collapsed?.count, 3 + 5 + 7 + 11, "свёрнутый показывает сумму с вложенными");
});

test("свёрнутый лист считает по-прежнему себя", () => {
  const rows = notebookRows(tree, ["Личное"]);
  assert.equal(rows.find((r) => r.path === "Личное")?.count, 2);
});

test("наличие детей видно по строке", () => {
  const rows = notebookRows(tree, []);
  const has = Object.fromEntries(rows.map((r) => [r.path, r.hasChildren]));
  assert.equal(has["Работа"], true);
  assert.equal(has["Работа/Задачи"], false);
  assert.equal(has[""], false, "корень vault детей не имеет: он обычная строка");
});

// Сворачивание корня не должно прятать весь сайдбар.
test("корень vault не родитель верхнего уровня", () => {
  const rows = notebookRows(tree, [""]);
  assert.deepEqual(paths(rows), [
    "", "Личное", "Работа", "Работа/Баги", "Работа/Баги/Старые", "Работа/Задачи",
  ]);
});

test("пустое дерево и мусор в свёрнутых не ломают", () => {
  assert.deepEqual(notebookRows([], ["чего-то нет"]), []);
  assert.equal(notebookRows(tree, ["Несуществующий"]).length, tree.length);
});

test("порядок по-русски", () => {
  const rows = notebookRows(
    [{ path: "Яблоко", count: 0 }, { path: "Арбуз", count: 0 }, { path: "Ёлка", count: 0 }],
    [],
  );
  assert.deepEqual(paths(rows), ["Арбуз", "Ёлка", "Яблоко"]);
});
