import { useCallback, useEffect, useRef, useState } from "react";

import {
  api,
  describeError,
  events,
  subscribe,
  type Note,
  type NoteChanged,
  type NoteRecord,
  type Notebook,
  type Tag,
} from "./api";
import { Editor } from "./components/Editor";
import { NoteList } from "./components/NoteList";
import { Sidebar, type Filter } from "./components/Sidebar";
import { Splitter } from "./components/Splitter";

/** Сколько заметок просим за раз. Виртуализация списка — задача фазы 4. */
const pageSize = 200;

export default function App() {
  const [sidebarWidth, setSidebarWidth] = useState(216);
  const [listWidth, setListWidth] = useState(320);

  const [notebooks, setNotebooks] = useState<Notebook[]>([]);
  const [tags, setTags] = useState<Tag[]>([]);
  const [notes, setNotes] = useState<Note[]>([]);
  const [note, setNote] = useState<Note | null>(null);

  const [filter, setFilter] = useState<Filter>({ kind: "all" });
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const [listError, setListError] = useState<string | null>(null);
  const [noteError, setNoteError] = useState<string | null>(null);

  // revision растёт на каждое изменение снаружи и перезапускает запросы.
  // Отдельное число, а не флаг: два изменения подряд должны дать два обновления.
  const [revision, setRevision] = useState(0);

  // Есть ли в редакторе несохранённое. Держим в ref: значение нужно
  // обработчику события, а не разметке.
  const dirty = useRef(false);

  // Запрос собирается здесь, а не в Go, только потому что это склейка строки из
  // того, что человек уже выбрал. Разбирает и исполняет его всё равно Go.
  const search = buildQuery(filter, query);

  useEffect(() => {
    let cancelled = false;
    Promise.all([api.notebooks(), api.tags()])
      .then(([books, tagList]) => {
        if (cancelled) return;
        setNotebooks(books);
        setTags(tagList);
      })
      .catch((err) => !cancelled && setListError(describeError(err)));
    return () => {
      cancelled = true;
    };
  }, [revision]);

  useEffect(() => {
    let cancelled = false;
    api
      .search(search, pageSize)
      .then((found) => {
        if (cancelled) return;
        setNotes(found);
        setListError(null);
      })
      .catch((err) => !cancelled && setListError(describeError(err)));
    return () => {
      cancelled = true;
    };
  }, [search, revision]);

  useEffect(() => {
    if (!selected) {
      setNote(null);
      return;
    }
    let cancelled = false;
    api
      .get(selected)
      .then((loaded) => {
        if (cancelled) return;
        setNote(loaded);
        setNoteError(null);
      })
      .catch((err) => !cancelled && setNoteError(describeError(err)));
    return () => {
      cancelled = true;
    };
  }, [selected, revision]);

  // Заметка, заведённая агентом через tasker-mcp, должна появиться в списке
  // сама — ради этого шага сценария приёмки (MCP.md §6) всё и затевалось.
  useEffect(() => {
    const off = [
      subscribe(events.notesChanged, () => setRevision((n) => n + 1)),
      subscribe<NoteChanged>(events.noteChanged, (changed) => {
        // Открытую заметку перечитываем, только если в ней нет несохранённого:
        // иначе правка пользователя молча заменилась бы содержимым с диска.
        // Случай с несохранённым — это плашка «файл изменён снаружи», её ещё нет.
        if (changed.id === selected && !dirty.current) setRevision((n) => n + 1);
      }),
    ];
    return () => off.forEach((unsubscribe) => unsubscribe());
  }, [selected]);

  const onFilter = useCallback((next: Filter) => {
    setFilter(next);
    setSelected(null);
  }, []);

  // После записи обновляем строку в списке на месте: перезапрашивать весь
  // список на каждое сохранение — значит дёргать его под курсором.
  const onSaved = useCallback((saved: NoteRecord) => {
    setNotes((current) =>
      current.map((item) =>
        item.ID === saved.ID
          ? {
              // Поимённо, а не спредом: у Note и Record поле Links разного
              // типа, и спред молча подменил бы одно другим.
              ...item,
              Title: saved.Title,
              Excerpt: saved.Excerpt,
              Updated: saved.Updated,
              Status: saved.Status,
              Pinned: saved.Pinned,
              Tags: saved.Tags,
              NumTasks: saved.NumTasks,
              NumDone: saved.NumDone,
            }
          : item,
      ),
    );
  }, []);

  return (
    <div
      className="layout"
      style={{ gridTemplateColumns: `${sidebarWidth}px 1px ${listWidth}px 1px 1fr` }}
    >
      <Sidebar notebooks={notebooks} tags={tags} filter={filter} onFilter={onFilter} />
      <Splitter width={sidebarWidth} min={160} max={400} onChange={setSidebarWidth} />
      <NoteList
        notes={notes}
        selected={selected}
        query={query}
        error={listError}
        onQuery={setQuery}
        onSelect={setSelected}
      />
      <Splitter width={listWidth} min={240} max={600} onChange={setListWidth} />
      {noteError && (
        <div className="pane pane--editor">
          <div className="error">{noteError}</div>
        </div>
      )}
      {!noteError && !note && (
        <div className="pane pane--editor">
          <div className="empty">Выберите заметку слева</div>
        </div>
      )}
      {!noteError && note && (
        // key по id: переключение заметки пересоздаёт редактор целиком, вместо
        // того чтобы подменять документ и сбрасывать состояние вима руками.
        <Editor
          key={`${note.ID}:${revision}`}
          note={note}
          onSaved={onSaved}
          onDirty={(value) => (dirty.current = value)}
          onClose={() => setSelected(null)}
        />
      )}
    </div>
  );
}

/**
 * buildQuery складывает выбранное в сайдбаре и набранное в поле поиска в один
 * запрос на языке из SPEC §8.5.
 */
function buildQuery(filter: Filter, query: string): string {
  const parts: string[] = [];
  if (filter.kind === "notebook") parts.push(`book:${quote(filter.path)}`);
  if (filter.kind === "tag") parts.push(`tag:${quote(filter.name)}`);
  if (query.trim() !== "") parts.push(query.trim());
  return parts.join(" ");
}

/** quote берёт значение в кавычки, если в нём есть пробелы. */
function quote(value: string): string {
  return /\s/.test(value) ? `"${value}"` : value;
}
