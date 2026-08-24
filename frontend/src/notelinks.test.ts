import assert from "node:assert/strict";
import { test } from "node:test";

import { findNoteLinks, linkAt } from "./notelinks.ts";

const id = "01M0K9V866QABD4VZ8PKH9WPJM";
const other = "01M0K9QWTZDCGECTRMB26QZ7CE";

test("ссылка находится в тексте markdown", () => {
  const text = `см. [Функционал](tasker://note/${id}) дальше`;
  const found = findNoteLinks(text);
  assert.equal(found.length, 1);
  assert.equal(found[0].id, id);
  // Подчёркивается сам адрес, а не подпись: подпись — обычный текст, и
  // трогать её нельзя, иначе щелчок по слову начнёт куда-то уводить.
  assert.equal(text.slice(found[0].from, found[0].to), `tasker://note/${id}`);
});

test("несколько ссылок подряд не склеиваются", () => {
  const found = findNoteLinks(`tasker://note/${id} tasker://note/${other}`);
  assert.deepEqual(
    found.map((item) => item.id),
    [id, other],
  );
});

test("смещение куска добавляется к координатам", () => {
  const found = findNoteLinks(`tasker://note/${id}`, 100);
  assert.equal(found[0].from, 100);
});

test("обрезанный идентификатор ссылкой не считается", () => {
  assert.deepEqual(findNoteLinks(`tasker://note/${id.slice(0, 20)}`), []);
  assert.deepEqual(findNoteLinks("tasker://note/"), []);
  // Строчные буквы в ULID не встречаются: их кладёт Go в верхнем регистре.
  assert.deepEqual(findNoteLinks(`tasker://note/${id.toLowerCase()}`), []);
});

test("текст без ссылок не даёт ничего", () => {
  assert.deepEqual(findNoteLinks("просто заметка про tasker и note"), []);
});

test("щелчок попадает в ссылку по позиции", () => {
  const links = findNoteLinks(`а tasker://note/${id} б`);
  const { from, to } = links[0];
  assert.equal(linkAt(links, from), id);
  assert.equal(linkAt(links, to), id);
  assert.equal(linkAt(links, from + 5), id);
  // Снаружи — мимо, и это должно читаться как «ссылки здесь нет».
  assert.equal(linkAt(links, from - 1), "");
  assert.equal(linkAt(links, to + 1), "");
  assert.equal(linkAt([], 0), "");
});
