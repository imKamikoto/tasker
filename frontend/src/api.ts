import { Events } from "@wailsio/runtime";

import { defaultSettings, parseSettings, type UISettings } from "./settings";

import { Closing, Notes, Settings } from "../bindings/tasker/internal/app";
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
  // Пустой срез в Go — это null в JSON, и биндинги честно объявляют его в типе.
  // Разворачиваем здесь, на границе, чтобы дальше по коду никто про это не помнил.
  search: (query: string, limit: number, hideCompleted: boolean, sort: UISettings) =>
    Notes.Search(query, limit, hideCompleted, sort.sortField, sort.sortReversed).then(nonNull),
  tasks: (limit: number) => Notes.Tasks(limit).then(nonNull),
  trashed: (limit: number) => Notes.Trashed(limit).then(nonNull),
  restore: (id: string) => Notes.Restore(id),
  setPinned: (id: string, pinned: boolean) => Notes.SetPinned(id, pinned),
  duplicate: (id: string) => Notes.Duplicate(id),
  deleteForever: (id: string) => Notes.Delete(id),
  get: (id: string) => Notes.Get(id),
  notebooks: () => Notes.Notebooks().then(nonNull),
  tags: () => Notes.Tags().then(nonNull),
  save: (id: string, title: string, body: string) => Notes.Save(id, title, body),
  setStatus: (id: string, status: string) => Notes.SetStatus(id, status),
  trash: (id: string) => Notes.Trash(id),
  create: (title: string, notebook: string) => Notes.Create(title, notebook),
  /** Ответ Go: буфер записан, окно можно закрывать. */
  readyToClose: () => Closing.Ready(),

  loadSettings: async (): Promise<UISettings> => {
    const raw = await Settings.Load();
    return raw ? parseSettings(raw) : { ...defaultSettings };
  },
  saveSettings: (value: UISettings) => Settings.Save(JSON.stringify(value)),
};

/** Имена событий из SPEC §6. Совпадают с константами в internal/app. */
export const events = {
  notesChanged: "tasker:notes-changed",
  noteChanged: "tasker:note-changed",
  beforeClose: "tasker:before-close",
} as const;

/** Заметка изменилась на диске. */
export type NoteChanged = { id: string; path: string };

/**
 * subscribe подписывается на событие из Go и возвращает функцию отписки.
 *
 * Обёртка нужна ради одного: Wails отдаёт полезную нагрузку внутри объекта
 * события, и разворачивать её в каждом обработчике — лишний повод ошибиться.
 */
export function subscribe<T>(name: string, handler: (data: T) => void): () => void {
  return Events.On(name, (event: { data: unknown }) => {
    const payload = Array.isArray(event.data) ? event.data[0] : event.data;
    handler(payload as T);
  });
}

/** nonNull заменяет пришедший из Go null пустым списком. */
function nonNull<T>(value: T[] | null): T[] {
  return value ?? [];
}

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
