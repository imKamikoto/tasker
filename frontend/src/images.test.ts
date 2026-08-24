import assert from "node:assert/strict";
import { test } from "node:test";

import { findImages, imageURL } from "./images.ts";

test("картинка находится в тексте", () => {
  const got = findImages("текст ![схема](attachments/2026/08/ABCDEFGH.png) дальше");
  assert.equal(got.length, 1);
  assert.equal(got[0].alt, "схема");
  assert.equal(got[0].src, "attachments/2026/08/ABCDEFGH.png");
});

test("подпись может быть пустой", () => {
  const got = findImages("![](attachments/a.png)");
  assert.equal(got.length, 1);
  assert.equal(got[0].alt, "");
});

test("две картинки в строке не склеиваются", () => {
  const got = findImages("![a](one.png) и ![b](two.png)");
  assert.deepEqual(
    got.map((item) => item.src),
    ["one.png", "two.png"],
  );
});

// Ссылка без восклицательного знака — это ссылка, а не картинка.
test("обычная ссылка картинкой не считается", () => {
  assert.deepEqual(findImages("[текст](file.png)"), []);
});

test("смещение куска добавляется к координатам", () => {
  const got = findImages("![a](one.png)", 50);
  assert.equal(got[0].from, 50);
});

test("путь от корня хранилища превращается в адрес раздатчика", () => {
  assert.equal(imageURL("attachments/2026/08/ABCDEFGH.png"), "/vault/attachments/2026/08/ABCDEFGH.png");
});

// Кириллица и пробелы в пути обязаны пережить кодирование, а слэши — остаться
// слэшами, иначе путь превратится в одно длинное имя файла.
test("кириллица и пробелы кодируются посегментно", () => {
  assert.equal(imageURL("Работа/моя схема.png"), "/vault/%D0%A0%D0%B0%D0%B1%D0%BE%D1%82%D0%B0/%D0%BC%D0%BE%D1%8F%20%D1%81%D1%85%D0%B5%D0%BC%D0%B0.png");
});

test("внешние адреса не трогаются", () => {
  assert.equal(imageURL("https://example.com/a.png"), "https://example.com/a.png");
  assert.equal(imageURL("data:image/png;base64,AAAA"), "data:image/png;base64,AAAA");
});
