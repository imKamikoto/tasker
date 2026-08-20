import { useCallback, useEffect, useState } from "react";

import { api, describeError, type Note, type Notebook, type Tag } from "./api";
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
  }, []);

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
  }, [search]);

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
  }, [selected]);

  const onFilter = useCallback((next: Filter) => {
    setFilter(next);
    setSelected(null);
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
      <Editor note={note} error={noteError} />
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
