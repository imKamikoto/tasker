import assert from "node:assert/strict";
import { test } from "node:test";

import { allowedValues, badTokens, classify, hasBadToken, splitQuery } from "./querytokens.ts";

test("разбор сохраняет строку символ в символ", () => {
  const cases = ["", "  ", "tag:баг status:active", '  book:"Работа/Баги"  -черновик ', "а  б"];
  for (const input of cases) {
    assert.equal(
      splitQuery(input)
        .map((token) => token.text)
        .join(""),
      input,
      input,
    );
  }
});

test("фильтры отбора красятся акцентом", () => {
  assert.equal(classify("tag:баг"), "accent");
  assert.equal(classify("book:Работа"), "accent");
  assert.equal(classify("-tag:черновик"), "accent");
});

test("остальные известные фильтры остаются обычным текстом", () => {
  assert.equal(classify("status:active"), "plain");
  assert.equal(classify("is:pinned"), "plain");
  assert.equal(classify("is:agent"), "plain");
  assert.equal(classify("has:task"), "plain");
  assert.equal(classify("title:отчёт"), "plain");
});

test("значение вне перечисления помечается как ошибка", () => {
  assert.equal(classify("status:активный"), "bad");
  assert.equal(classify("is:закреплено"), "bad");
  assert.equal(classify("has:чеклист"), "bad");
});

test("регистр значения не важен", () => {
  assert.equal(classify("status:onHold"), "plain");
  assert.equal(classify("STATUS:ACTIVE"), "plain");
});

test("недопечатанный фильтр ошибкой не считается", () => {
  // Иначе поле краснеет на каждом втором нажатии, пока набирают значение.
  assert.equal(classify("status:"), "plain");
  assert.equal(classify("tag:"), "accent");
});

test("незнакомый префикс — обычный текст", () => {
  // Двоеточие в коде и ссылках встречается чаще, чем опечатка в имени фильтра.
  assert.equal(classify("http://example.com"), "plain");
  assert.equal(classify("Map:строка"), "plain");
});

test("пробелы внутри кавычек не режут токен", () => {
  const tokens = splitQuery('book:"Работа и отдых" tag:баг');
  const words = tokens.filter((token) => token.text.trim() !== "");
  assert.deepEqual(
    words.map((token) => token.text),
    ['book:"Работа и отдых"', "tag:баг"],
  );
  assert.equal(words[0].kind, "accent");
});

test("испорченные куски достаются целиком", () => {
  assert.equal(hasBadToken("перерасчёт status:активный"), true);
  assert.deepEqual(badTokens("перерасчёт status:активный tag:баг"), ["status:активный"]);
  assert.equal(hasBadToken("перерасчёт tag:баг"), false);
});

test("подсказка перечисляет допустимые значения", () => {
  assert.deepEqual(allowedValues("status"), ["none", "active", "onhold", "completed", "dropped"]);
  assert.deepEqual(allowedValues("is"), ["pinned", "agent"]);
  assert.deepEqual(allowedValues("нет-такого"), []);
});
