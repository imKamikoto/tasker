import { Notes } from "../bindings/tasker/internal/app";
import type { Record as NoteRecord, Notebook, Tag } from "../bindings/tasker/internal/index/models";
import type { Note } from "../bindings/tasker/internal/notes/models";

export type { NoteRecord, Notebook, Tag, Note };

/**
 * Единственная точка, через которую фронтенд говорит с Go.
 *
 * Компоненты не зовут биндинги напрямую: так видно всю границу разом, и в
 * одном месте лежит разбор ошибок. Своей логики здесь нет и быть не должно —
 * она вся в internal/notes (CLAUDE.md, инвариант 3).
 */
export const api = {
  search: (query: string, limit: number) => Notes.Search(query, limit),
  get: (id: string) => Notes.Get(id),
  notebooks: () => Notes.Notebooks(),
  tags: () => Notes.Tags(),
  save: (id: string, title: string, body: string) => Notes.Save(id, title, body),
  setStatus: (id: string, status: string) => Notes.SetStatus(id, status),
  trash: (id: string) => Notes.Trash(id),
  create: (title: string, notebook: string) => Notes.Create(title, notebook),
};

/**
 * describeError превращает что угодно в строку для показа человеку.
 *
 * Через биндинг прилетает и ошибка Go, и отказ самого моста — например, когда
 * страница открыта в обычном браузере, а рантайма Wails рядом нет. Показать
 * это надо в любом случае: пустой экран без объяснения хуже любой ошибки.
 */
export function describeError(err: unknown): string {
  if (err instanceof Error) return err.message;
  if (typeof err === "string") return err;
  return JSON.stringify(err);
}
