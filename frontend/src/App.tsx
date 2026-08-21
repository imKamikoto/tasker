import { useCallback, useEffect, useMemo, useRef, useState } from "react";

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
import { applyClick } from "./selection";
import { defaultSettings } from "./settings";
import { statusForKey } from "./statuses";
import { MovePicker } from "./components/MovePicker";
import { Splitter } from "./components/Splitter";

/** Сколько заметок просим за раз. Виртуализация списка — задача фазы 4. */
const pageSize = 200;

export default function App() {
  // Настройки интерфейса читаются один раз при запуске и с тех пор живут здесь.
  const [settings, setSettings] = useState(defaultSettings);

  const [notebooks, setNotebooks] = useState<Notebook[]>([]);
  const [tags, setTags] = useState<Tag[]>([]);
  const [notes, setNotes] = useState<Note[]>([]);
  const [note, setNote] = useState<Note | null>(null);

  // «Активные» по умолчанию: с этого экрана начинается рабочий день (SPEC §8.3).
  const [filter, setFilter] = useState<Filter>({ kind: "active" });
  const [query, setQuery] = useState("");
  // Выделение — набор, а не одна заметка: Cmd и Shift выделяют несколько
  // (SPEC §8.4). Якорь нужен, чтобы Shift тянул диапазон от одной точки.
  const [selected, setSelected] = useState<string[]>([]);
  const [anchor, setAnchor] = useState<string | null>(null);

  // Редактор показывает заметку, только когда выбрана ровно одна: показывать
  // «одну из пяти» — значит врать про то, что правится.
  const single = selected.length === 1 ? selected[0] : null;
  const [listError, setListError] = useState<string | null>(null);
  const [noteError, setNoteError] = useState<string | null>(null);

  // revision растёт на каждое изменение снаружи и перезапускает запросы.
  // Отдельное число, а не флаг: два изменения подряд должны дать два обновления.
  const [revision, setRevision] = useState(0);

  // Есть ли в редакторе несохранённое. Держим в ref: значение нужно
  // обработчику события, а не разметке.
  const dirty = useRef(false);

  // Файл изменился на диске, пока в буфере лежит несохранённое. Молча взять
  // любую из сторон нельзя: обе — чья-то работа.
  const [conflict, setConflict] = useState(false);

  // Выбранные вручную цвета тегов приходят из индекса вместе со счётчиками:
  // «default» означает, что цвет выведется из имени.
  const chosenColors = useMemo(() => {
    const map: Record<string, number> = {};
    for (const tag of tags) {
      const parsed = Number(tag.Color);
      if (Number.isInteger(parsed)) map[tag.Name] = parsed;
    }
    return map;
  }, [tags]);

  // Просьба передать фокус в редактор. Число, а не флаг: Enter, нажатый дважды,
  // должен сработать оба раза.
  const [focusToken, setFocusToken] = useState(0);

  // Открыт ли выбор ноутбука для переноса (клавиша m).
  const [moving, setMoving] = useState(false);

  // Запрос собирается здесь, а не в Go, только потому что это склейка строки из
  // того, что человек уже выбрал. Разбирает и исполняет его всё равно Go.
  const search = buildQuery(filter, query);

  useEffect(() => {
    let cancelled = false;
    api
      .loadSettings()
      .then((loaded) => !cancelled && setSettings(loaded))
      .catch((err) => !cancelled && setListError(describeError(err)));
    return () => {
      cancelled = true;
    };
  }, []);

  // Сохраняем с задержкой: ширины меняются на каждый пиксель перетаскивания,
  // и писать файл на каждый из них незачем.
  useEffect(() => {
    if (settings === defaultSettings) return;
    const timer = window.setTimeout(() => {
      void api.saveSettings(settings).catch(() => undefined);
    }, 400);
    return () => window.clearTimeout(timer);
  }, [settings]);

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
    // Завершённое и брошенное прячем, пока человек сам не спросит про статус
    // (SPEC §8.3). «Активные» и так собраны по статусам.
    const request =
      filter.kind === "active"
        ? api.tasks(pageSize)
        : filter.kind === "trash"
          ? api.trashed(pageSize)
          : api.search(search, pageSize, !query.includes("status:"), settings);

    request
      .then((found) => {
        if (cancelled) return;
        setNotes(found);
        setListError(null);
      })
      .catch((err) => !cancelled && setListError(describeError(err)));
    return () => {
      cancelled = true;
    };
  }, [search, revision, filter.kind, query, settings]);

  useEffect(() => {
    if (!single) {
      setNote(null);
      return;
    }
    let cancelled = false;
    api
      .get(single)
      .then((loaded) => {
        if (cancelled) return;
        setNote(loaded);
        setNoteError(null);
      })
      .catch((err) => !cancelled && setNoteError(describeError(err)));
    return () => {
      cancelled = true;
    };
  }, [single, revision]);

  // Заметка, заведённая агентом через tasker-mcp, должна появиться в списке
  // сама — ради этого шага сценария приёмки (MCP.md §6) всё и затевалось.
  useEffect(() => {
    const off = [
      subscribe(events.notesChanged, () => setRevision((n) => n + 1)),
      subscribe<NoteChanged>(events.noteChanged, (changed) => {
        if (changed.id !== single) return;
        // Чистый буфер просто перечитываем. С несохранённым решает человек:
        // молча затереть его правку содержимым с диска нельзя (SPEC §5.3).
        if (dirty.current) setConflict(true);
        else setRevision((n) => n + 1);
      }),
    ];
    return () => off.forEach((unsubscribe) => unsubscribe());
  }, [single]);

  // Обе операции корзины меняют список целиком: заметка либо уезжает обратно,
  // либо исчезает навсегда.
  const act = useCallback((operation: Promise<unknown>, keepSelection = false) => {
    operation
      .then(() => {
        if (!keepSelection) setSelected([]);
        setRevision((n) => n + 1);
      })
      .catch((err) => setNoteError(describeError(err)));
  }, []);

  // Создание заметки: в текущем ноутбуке, если он выбран, иначе в корне.
  const createNote = useCallback(() => {
    const notebook = filter.kind === "notebook" ? filter.path : "";
    api
      .create("Новая заметка", notebook)
      .then((created) => {
        setSelected([created.ID]);
        setAnchor(created.ID);
        setRevision((n) => n + 1);
      })
      .catch((err) => setListError(describeError(err)));
  }, [filter]);

  const onFilter = useCallback((next: Filter) => {
    setFilter(next);
    setSelected([]);
    setAnchor(null);
    setConflict(false);
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

  // Клавиатура списка. Слушаем окно, а не список: фокус может быть где угодно,
  // а шоткаты статусов должны работать и из редактора (SPEC §8.4, §8.3).
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      // В поле ввода буквы — это буквы, а не команды.
      const target = event.target as HTMLElement | null;
      const typing =
        target instanceof HTMLInputElement ||
        target instanceof HTMLTextAreaElement ||
        target?.isContentEditable === true;

      if (event.metaKey && event.ctrlKey) {
        const status = statusForKey(event.key);
        if (status && selected.length > 0) {
          event.preventDefault();
          // Заметки могли перестать подходить под фильтр — перечитываем список.
          act(api.setStatusMany(selected, status), true);
        }
        return;
      }

      // Дальше — команды списка, и в тексте им делать нечего: Cmd+D там
      // мультикурсор CodeMirror (SPEC §8.6), Cmd+Backspace — удаление до
      // начала строки, а буквы — буквы.
      if (typing) return;

      // Cmd+Backspace — в корзину, Cmd+D — дублировать (SPEC §8.4).
      if (event.metaKey && !event.ctrlKey && !event.altKey && event.key === "n") {
        event.preventDefault();
        createNote();
        return;
      }

      if (event.metaKey && !event.ctrlKey && !event.altKey && selected.length > 0) {
        if (event.key === "Backspace") {
          event.preventDefault();
          act(api.trashMany(selected));
          return;
        }
        if (event.key === "d" && single) {
          // Дублирование остаётся штучным: копия пачки почти всегда промах.
          event.preventDefault();
          act(api.duplicate(single));
          return;
        }
      }

      if (event.metaKey || event.ctrlKey || event.altKey) return;

      switch (event.key) {
        case "j":
        case "k": {
          event.preventDefault();
          // Стрелками ходим по одной: расширять выделение с клавиатуры — это
          // Shift+клик, а не j и k.
          const next = step(notes, single, event.key === "j" ? 1 : -1);
          setSelected(next ? [next] : []);
          setAnchor(next);
          return;
        }
        case "Enter":
          // Из списка — в текст. Обратно уводит вим по :q или мышь.
          if (single) {
            event.preventDefault();
            setFocusToken((n) => n + 1);
          }
          return;
        case "m":
          // Перенос в другой ноутбук: выбор открывается поверх списка.
          if (selected.length > 0) {
            event.preventDefault();
            setMoving(true);
          }
          return;
        case "p": {
          if (selected.length === 0) return;
          event.preventDefault();
          // Пачку закрепляем целиком, ориентируясь на первую заметку: половину
          // закрепить, половину открепить — не то, чего ждут от одной клавиши.
          const first = notes.find((item) => item.ID === selected[0]);
          act(api.setPinnedMany(selected, !(first?.Pinned ?? false)), true);
          return;
        }
      }
    };

    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [notes, selected, single, act, createNote]);

  return (
    <div
      className="layout"
      style={{
        gridTemplateColumns: `${settings.sidebarWidth}px 1px ${settings.listWidth}px 1px 1fr`,
      }}
    >
      {moving && selected.length > 0 && (
        <MovePicker
          notebooks={notebooks.map((notebook) => notebook.Path)}
          onCancel={() => setMoving(false)}
          onPick={(notebook) => {
            setMoving(false);
            act(api.moveMany(selected, notebook), true);
          }}
        />
      )}

      <Sidebar
        notebooks={notebooks}
        tags={tags}
        filter={filter}
        onFilter={onFilter}
        collapsed={settings.collapsed}
        onCreateNotebook={(path) => act(api.createNotebook(path), true)}
        onRenameNotebook={(from, to) => act(api.renameNotebook(from, to), true)}
        onDeleteNotebook={(path) => act(api.deleteNotebook(path), true)}
        onRenameTag={(from, to) => act(api.renameTag(from, to), true)}
        onDropNote={(id, notebook) =>
          // Тащат одну строку, но если она внутри выделения — переносится всё
          // выделение: иначе пришлось бы тащить их по одной.
          act(api.moveMany(selected.includes(id) ? selected : [id], notebook), true)
        }
        onToggle={(path) =>
          setSettings((current) => ({
            ...current,
            collapsed: current.collapsed.includes(path)
              ? current.collapsed.filter((item) => item !== path)
              : [...current.collapsed, path],
          }))
        }
      />
      <Splitter
        width={settings.sidebarWidth}
        min={160}
        max={400}
        onChange={(value) => setSettings((current) => ({ ...current, sidebarWidth: value }))}
      />
      <NoteList
        notes={notes}
        selected={selected}
        query={query}
        error={listError}
        onQuery={setQuery}
        onSelect={(id, modifiers) => {
          const next = applyClick({
            order: notes.map((note) => note.ID),
            selected,
            anchor,
            clicked: id,
            toggle: modifiers.toggle,
            range: modifiers.range,
          });
          setSelected(next.selected);
          setAnchor(next.anchor);
        }}
        sortField={settings.sortField}
        sortReversed={settings.sortReversed}
        onSort={(sortField, sortReversed) =>
          setSettings((current) => ({ ...current, sortField, sortReversed }))
        }
        onCreate={createNote}
      />
      <Splitter
        width={settings.listWidth}
        min={240}
        max={600}
        onChange={(value) => setSettings((current) => ({ ...current, listWidth: value }))}
      />
      {noteError && (
        <div className="pane pane--editor">
          <div className="error">{noteError}</div>
        </div>
      )}
      {!noteError && !note && selected.length <= 1 && (
        <div className="pane pane--editor">
          <div className="empty">Выберите заметку слева</div>
        </div>
      )}
      {selected.length > 1 && (
        <div className="pane pane--editor">
          <div className="empty">Выбрано заметок: {selected.length}</div>
        </div>
      )}
      {!noteError && note && filter.kind === "trash" && (
        <div className="pane pane--editor">
          <input className="editor__title" value={note.Title} readOnly />
          <div className="editor__meta">
            <span className="editor__state">в корзине</span>
          </div>
          <div className="conflict">
            <span>Заметка удалена. Править её нельзя, пока она в корзине.</span>
            <button onClick={() => act(api.restore(note.ID))}>Восстановить</button>
            <button onClick={() => act(api.deleteForever(note.ID))}>Удалить насовсем</button>
          </div>
          <div className="editor__body trashed-body">{note.Body}</div>
        </div>
      )}
      {!noteError && note && filter.kind !== "trash" && (
        // key по id: переключение заметки пересоздаёт редактор целиком, вместо
        // того чтобы подменять документ и сбрасывать состояние вима руками.
        <Editor
          key={`${note.ID}:${revision}`}
          note={note}
          onSaved={onSaved}
          onDirty={(value) => (dirty.current = value)}
          onClose={() => setSelected([])}
          conflict={conflict}
          focusToken={focusToken}
          knownTags={tags.map((tag) => tag.Name)}
          onTags={(next) => act(api.setTags(note.ID, next), true)}
          tagColors={chosenColors}
          onTagColor={(tag, color) => act(api.setTagColor(tag, color), true)}
          onReload={() => {
            // Пересоздание редактора выбрасывает буфер вместе с ним.
            dirty.current = false;
            setConflict(false);
            setRevision((n) => n + 1);
          }}
          onKeepMine={() => setConflict(false)}
        />
      )}
    </div>
  );
}

/**
 * step двигает выделение по списку, не выходя за края: на границе оставаться
 * на месте понятнее, чем прыгать на другой конец.
 */
function step(notes: Note[], selected: string | null, direction: 1 | -1): string | null {
  if (notes.length === 0) return null;
  const current = notes.findIndex((note) => note.ID === selected);
  if (current < 0) return notes[direction === 1 ? 0 : notes.length - 1].ID;

  const next = current + direction;
  if (next < 0 || next >= notes.length) return selected;
  return notes[next].ID;
}

/**
 * buildQuery складывает выбранное в сайдбаре и набранное в поле поиска в один
 * запрос на языке из SPEC §8.5.
 */
function buildQuery(filter: Filter, query: string): string {
  const parts: string[] = [];
  if (filter.kind === "notebook") parts.push(`book:${quote(filter.path)}`);
  // Несколько тегов соединяются через И — это делает сам язык запросов.
  if (filter.kind === "tags") parts.push(...filter.names.map((name) => `tag:${quote(name)}`));
  if (query.trim() !== "") parts.push(query.trim());
  return parts.join(" ");
}

/** quote берёт значение в кавычки, если в нём есть пробелы. */
function quote(value: string): string {
  return /\s/.test(value) ? `"${value}"` : value;
}
