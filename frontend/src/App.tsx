import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import {
  api,
  describeError,
  events,
  subscribe,
  type Counts,
  type Note,
  type NoteChanged,
  type NoteRecord,
  type Notebook,
  type Tag,
} from "./api";
import { BulkPane } from "./components/BulkPane";
import { Editor } from "./components/Editor";
import { EmptySearch, EmptyVault } from "./components/EmptyState";
import { MovePicker } from "./components/MovePicker";
import { NoteList } from "./components/NoteList";
import { NoteMenu } from "./components/NoteMenu";
import { Settings } from "./components/Settings";
import { Sidebar, type Filter } from "./components/Sidebar";
import { Splitter } from "./components/Splitter";
import { focusCommands, movePane, stealsFromEditor, type Pane } from "./focus";
import { combination, resolveCommand, type Keymap } from "./keys";
import { applyClick } from "./selection";
import { defaultSettings, nextZoom } from "./settings";
import { statusForCommand, type Status } from "./statuses";

/** Сколько заметок просим за раз. */
const pageSize = 200;

/** Высота строки списка при масштабе 1. Совпадает с --row-height в стилях. */
const baseRowHeight = 104;

/** Пустые счётчики до первого ответа Go: нули лучше, чем скачущие цифры. */
const noCounts: Counts = { Active: 0, All: 0, Agent: 0, Trashed: 0 };

export default function App() {
  // Настройки интерфейса читаются один раз при запуске и с тех пор живут здесь.
  const [settings, setSettings] = useState(defaultSettings);

  const [notebooks, setNotebooks] = useState<Notebook[]>([]);
  const [tags, setTags] = useState<Tag[]>([]);
  const [counts, setCounts] = useState<Counts>(noCounts);
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

  // Что сейчас тащат мышью: сайдбар показывает «+N» на цели.
  const [dragging, setDragging] = useState(0);
  // Контекстное меню строки списка.
  const [menu, setMenu] = useState<{ id: string; x: number; y: number } | null>(null);

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

  // Счётчики ноутбуков по пути — их показывает и выбор ноутбука при переносе.
  const notebookCounts = useMemo(() => {
    const map: Record<string, number> = {};
    for (const notebook of notebooks) map[notebook.Path] = notebook.Count;
    return map;
  }, [notebooks]);

  // Просьба передать фокус в редактор. Число, а не флаг: Enter, нажатый дважды,
  // должен сработать оба раза.
  const [focusToken, setFocusToken] = useState(0);

  // Какая колонка принимает клавиши. Раньше это выводилось из того, стоит ли
  // каретка в тексте, и сайдбар с клавиатуры был недоступен вовсе.
  const [pane, setPane] = useState<Pane>("list");
  // Режим вима из редактора: в режиме вставки Ctrl+K принадлежит тексту.
  const vimMode = useRef("NORMAL");
  // Команда, отданная сайдбару. Токен растёт на каждое нажатие: две «вниз»
  // подряд должны сработать дважды, а одинаковое имя эффект бы не перезапустило.
  const [sidebarCommand, setSidebarCommand] = useState({ name: "", token: 0 });

  // Раскладка клавиш из ~/.tasker/keymap.json (SPEC §8.11). Пустая до загрузки:
  // до неё ни одна команда не сработает, и это правильнее, чем срабатывание по
  // зашитым умолчаниям, которые человек мог переназначить.
  const [keymap, setKeymap] = useState<Keymap>({});

  // Открыт ли выбор ноутбука для переноса (клавиша m).
  const [moving, setMoving] = useState(false);
  // Открыт ли экран настроек (Cmd+,).
  const [prefs, setPrefs] = useState(false);
  // Машина на батарее: от этого зависит, действует ли прозрачность.
  const [onBattery, setOnBattery] = useState(false);

  const rowHeight = Math.round(baseRowHeight * settings.textScale);

  // Запрос собирается здесь, а не в Go, только потому что это склейка строки из
  // того, что человек уже выбрал. Разбирает и исполняет его всё равно Go.
  const search = buildQuery(filter, query);

  useEffect(() => {
    let cancelled = false;
    api
      .loadKeymap()
      .then((loaded) => !cancelled && setKeymap(loaded as Keymap))
      .catch((err) => !cancelled && setListError(describeError(err)));

    api
      .loadSettings()
      .then((loaded) => !cancelled && setSettings(loaded))
      .catch((err) => !cancelled && setListError(describeError(err)));
    return () => {
      cancelled = true;
    };
  }, []);

  // Тема и акцент живут атрибутами на <html>: их читает palette.css, и весь
  // интерфейс перекрашивается без единой строки в компонентах.
  useEffect(() => {
    const root = document.documentElement;
    root.dataset.theme = settings.theme;
    root.dataset.accent = settings.accent;
    // Свой акцент — единственное, что нельзя описать заранее: оттенок его,
    // а насыщенность и светлота берутся из правила темы.
    if (settings.accent === "custom") {
      root.style.setProperty("--hsl-accent-custom", `${settings.accentHue}`);
    } else {
      root.style.removeProperty("--hsl-accent-custom");
    }
  }, [settings.theme, settings.accent, settings.accentHue]);

  // Прозрачность и размытие — тоже переменные на корне: окно уже создано
  // полупрозрачным, а насколько это видно, решает CSS.
  useEffect(() => {
    const root = document.documentElement;
    const off = settings.opaqueOnBattery && onBattery;
    const alpha = off ? 0 : settings.transparency / 100;
    root.style.setProperty("--panel-alpha", String(1 - alpha));
    // Формула из хендоффа: 100% размытия — это 60 пикселей.
    root.style.setProperty("--panel-blur", `${alpha > 0 ? (settings.blur * 0.6).toFixed(0) : 0}px`);
    root.dataset.dither = String(settings.dither);
  }, [settings.transparency, settings.blur, settings.opaqueOnBattery, settings.dither, onBattery]);

  // Состояние питания спрашиваем при запуске и раз в минуту: чаще незачем,
  // а совсем не спрашивать значит держать прозрачность включённой в дороге.
  useEffect(() => {
    let cancelled = false;
    const check = () =>
      api
        .onBattery()
        .then((value) => !cancelled && setOnBattery(value))
        .catch(() => undefined);
    void check();
    const timer = window.setInterval(check, 60_000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, []);

  // Окно автокоммита живёт в настройках интерфейса, а исполняет его Go —
  // поэтому значение туда надо отправить, а не ждать, что он его прочитает.
  useEffect(() => {
    void api.configureGit(settings.commitWindow).catch(() => undefined);
  }, [settings.commitWindow]);

  // Масштаб текста живёт целиком в CSS: растёт кегль, ширины колонок остаются.
  // Высота строки списка считается здесь и здесь же уезжает в стили — на ней
  // стоит арифметика виртуализации, и второго источника у неё быть не должно.
  useEffect(() => {
    const root = document.documentElement;
    root.style.setProperty("--ui-scale", String(settings.textScale));
    root.style.setProperty("--row-height", `${rowHeight}px`);
    root.style.setProperty(
      "--editor-font-size",
      `${Math.round(settings.fontSize * settings.textScale)}px`,
    );
    root.style.setProperty("--editor-line-height", String(settings.lineHeight));
  }, [settings.textScale, settings.fontSize, settings.lineHeight, rowHeight]);

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
    Promise.all([api.notebooks(), api.tags(), api.counts()])
      .then(([books, tagList, totals]) => {
        if (cancelled) return;
        setNotebooks(books);
        setTags(tagList);
        setCounts(totals);
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

  // Закрепление считается от первой заметки выделения: пачка ведёт себя как
  // одно целое, а не как набор независимых переключателей.
  const firstSelected = notes.find((item) => item.ID === selected[0]);
  const togglePinned = useCallback(
    (ids: string[]) => {
      const first = notes.find((item) => item.ID === ids[0]);
      act(api.setPinnedMany(ids, !(first?.Pinned ?? false)), true);
    },
    [notes, act],
  );

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

  // Клавиатура. Слушаем окно, а не список: сочетание может прийти откуда
  // угодно, а разводит их контекст, а не место обработчика (SPEC §8.11).
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      // Каретка в поле ввода — клавиши принадлежат ему, что бы ни говорил
      // фокус колонки: иначе «j» в поиске двигало бы список вместо набора.
      const typing =
        target instanceof HTMLInputElement ||
        target instanceof HTMLTextAreaElement ||
        target?.isContentEditable === true;

      // От частного к общему: сначала контекст колонки, потом глобальный.
      const contexts = typing ? ["editor", "global"] : [pane === "sidebar" ? "sidebar" : "note-list", "global"];
      const command = resolveCommand(keymap, contexts, event);
      if (!command) return;

      if (command.startsWith("sidebar.")) {
        event.preventDefault();
        setSidebarCommand((current) => ({ name: command, token: current.token + 1 }));
        return;
      }

      const status = statusForCommand(command);
      if (status) {
        if (selected.length === 0) return;
        event.preventDefault();
        act(api.setStatusMany(selected, status), true);
        return;
      }

      switch (command) {
        case "note.create":
          event.preventDefault();
          createNote();
          return;
        case "note.trash":
          if (selected.length === 0) return;
          event.preventDefault();
          act(api.trashMany(selected));
          return;
        case "note.duplicate":
          if (!single) return;
          event.preventDefault();
          act(api.duplicate(single));
          return;
        case "note.move":
          if (selected.length === 0) return;
          event.preventDefault();
          setMoving(true);
          return;
        case "note.settings":
          event.preventDefault();
          setPrefs(true);
          return;
        case "view.sidebar":
          event.preventDefault();
          setSettings((current) => ({ ...current, sidebarHidden: !current.sidebarHidden }));
          return;
        case "view.zoom.in":
        case "view.zoom.out":
        case "view.zoom.reset":
          event.preventDefault();
          setSettings((current) => ({
            ...current,
            textScale: nextZoom(current.textScale, command),
          }));
          return;
        case "note.pin": {
          if (selected.length === 0) return;
          event.preventDefault();
          togglePinned(selected);
          return;
        }
        case "list.down":
        case "list.up": {
          event.preventDefault();
          const next = step(notes, single, command === "list.down" ? 1 : -1);
          setSelected(next ? [next] : []);
          setAnchor(next);
          return;
        }
        case "list.open":
          if (!single) return;
          event.preventDefault();
          setPane("editor");
          return;
      }
    };

    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [keymap, notes, selected, single, act, createNote, togglePinned, pane]);

  // Смена фокуса ловится в фазе погружения: иначе сочетание сначала достанется
  // CodeMirror, и выйти из текста клавиатурой будет нельзя. Ctrl+K там —
  // «удалить до конца строки», поэтому в режиме вставки не перехватываем.
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const command = resolveCommand(keymap, ["global"], event);
      const direction = command ? focusCommands[command] : undefined;
      if (direction === undefined) return;
      if (pane === "editor" && !stealsFromEditor(combination(event), vimMode.current)) return;

      event.preventDefault();
      event.stopPropagation();
      setPane((current) => {
        let next = movePane(current, direction);
        // Свёрнутый сайдбар пропускаем: держать фокус на том, чего не видно,
        // значит терять клавиши в никуда.
        if (next === "sidebar" && settings.sidebarHidden) next = movePane(next, direction);
        return next;
      });
    };

    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [keymap, pane, settings.sidebarHidden]);

  // Фокус на редакторе — значит каретка в тексте. Токен растёт, потому что
  // вернуться в текст можно много раз подряд.
  useEffect(() => {
    if (pane === "editor") {
      setFocusToken((n) => n + 1);
      return;
    }
    // Снимаем каретку только с редактора. Слепой blur активного элемента
    // выбрасывал бы человека из поля поиска при каждой смене заметки.
    const active = document.activeElement;
    if (active instanceof HTMLElement && active.closest(".pane--editor")) active.blur();
  }, [pane, single]);

  // Щелчок или Tab внутрь колонки — тоже способ передать ей фокус. Через
  // focusin, а не обработчики на каждом компоненте: так каретка в тексте и
  // состояние фокуса не могут разойтись, кто бы её туда ни поставил.
  useEffect(() => {
    const onFocusIn = (event: FocusEvent) => {
      const target = event.target as HTMLElement | null;
      if (!target?.closest) return;
      if (target.closest(".pane--editor")) setPane("editor");
      else if (target.closest(".pane--sidebar")) setPane("sidebar");
      else if (target.closest(".pane--list")) setPane("list");
    };
    window.addEventListener("focusin", onFocusIn);
    return () => window.removeEventListener("focusin", onFocusIn);
  }, []);

  // Escape снимает выделение пачки: она перехватывает шоткаты, и выйти из неё
  // надо уметь, не трогая мышь.
  useEffect(() => {
    if (selected.length < 2) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") setSelected([]);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [selected.length]);

  const menuNote = menu ? notes.find((item) => item.ID === menu.id) : undefined;

  return (
    <div
      className="layout"
      // Свёрнутый сайдбар — нулевая колонка вместе со своим разделителем:
      // так ширина остаётся записанной и возвращается той же, какой была.
      style={{
        gridTemplateColumns: settings.sidebarHidden
          ? `0 0 ${settings.listWidth}px 1px 1fr`
          : `${settings.sidebarWidth}px 1px ${settings.listWidth}px 1px 1fr`,
      }}
    >
      {prefs && (
        <Settings
          settings={settings}
          onChange={(patch) => setSettings((current) => ({ ...current, ...patch }))}
          keymap={keymap}
          onKeymap={(next) => {
            // Сохраняем и перечитываем: Go сливает файл с умолчаниями, и
            // «что получилось» знает только он.
            setKeymap(next);
            api
              .saveKeymap(next)
              .then(() => api.loadKeymap())
              .then((loaded) => setKeymap(loaded as Keymap))
              .catch((err) => setListError(describeError(err)));
          }}
          onResetKeymap={() => {
            api
              .resetKeymap()
              .then(() => api.loadKeymap())
              .then((loaded) => setKeymap(loaded as Keymap))
              .catch((err) => setListError(describeError(err)));
          }}
          onClose={() => setPrefs(false)}
        />
      )}

      {moving && selected.length > 0 && (
        <MovePicker
          notebooks={notebooks.map((notebook) => notebook.Path)}
          counts={notebookCounts}
          count={selected.length}
          onCancel={() => setMoving(false)}
          onPick={(notebook) => {
            setMoving(false);
            act(api.moveMany(selected, notebook), true);
          }}
          onCreate={(notebook) => {
            setMoving(false);
            // Создаём и сразу переносим: разделять эти два шага незачем,
            // человек уже сказал, чего хочет.
            act(
              api.createNotebook(notebook).then(() => api.moveMany(selected, notebook)),
              true,
            );
          }}
        />
      )}

      {menu && menuNote && (
        <NoteMenu
          at={{ x: menu.x, y: menu.y }}
          status={menuNote.Status}
          pinned={menuNote.Pinned}
          onClose={() => setMenu(null)}
          onStatus={(status) => act(api.setStatusMany(targets(selected, menu.id), status), true)}
          onPin={() => togglePinned(targets(selected, menu.id))}
          onMove={() => {
            setSelected(targets(selected, menu.id));
            setMoving(true);
          }}
          onDuplicate={() => act(api.duplicate(menu.id))}
          onTrash={() => act(api.trashMany(targets(selected, menu.id)))}
        />
      )}

      {settings.sidebarHidden ? (
        <nav className="pane pane--sidebar" aria-hidden="true" />
      ) : (
      <Sidebar
        notebooks={notebooks}
        tags={tags}
        counts={counts}
        tagColors={chosenColors}
        filter={filter}
        onFilter={onFilter}
        collapsed={settings.collapsed}
        dragging={dragging}
        focused={pane === "sidebar"}
        command={sidebarCommand}
        settingsOpen={prefs}
        onSettings={() => setPrefs(true)}
        onCreateNotebook={(path) => act(api.createNotebook(path), true)}
        onRenameNotebook={(from, to) => act(api.renameNotebook(from, to), true)}
        onDeleteNotebook={(path) => act(api.deleteNotebook(path), true)}
        onRenameTag={(from, to) => act(api.renameTag(from, to), true)}
        onDropNote={(id, notebook) => {
          setDragging(0);
          // Тащат одну строку, но если она внутри выделения — переносится всё
          // выделение: иначе пришлось бы тащить их по одной.
          act(api.moveMany(targets(selected, id), notebook), true);
        }}
        onToggle={(path) =>
          setSettings((current) => ({
            ...current,
            collapsed: current.collapsed.includes(path)
              ? current.collapsed.filter((item) => item !== path)
              : [...current.collapsed, path],
          }))
        }
      />
      )}
      <Splitter
        width={settings.sidebarWidth}
        min={180}
        max={320}
        onChange={(value) => setSettings((current) => ({ ...current, sidebarWidth: value }))}
      />

      <NoteList
        notes={notes}
        selected={selected}
        query={query}
        error={listError}
        tagColors={chosenColors}
        onQuery={setQuery}
        onSelect={(id, modifiers) => {
          const next = applyClick({
            order: notes.map((item) => item.ID),
            selected,
            anchor,
            clicked: id,
            toggle: modifiers.toggle,
            range: modifiers.range,
          });
          setSelected(next.selected);
          setAnchor(next.anchor);
        }}
        onContext={(id, at) => {
          // Щелчок вне выделения сначала выбирает строку: меню должно
          // относиться к тому, по чему щёлкнули.
          if (!selected.includes(id)) {
            setSelected([id]);
            setAnchor(id);
          }
          setMenu({ id, ...at });
        }}
        onDragNote={(id) => setDragging(targets(selected, id).length)}
        onDragEnd={() => setDragging(0)}
        sortField={settings.sortField}
        sortReversed={settings.sortReversed}
        onSort={(sortField, sortReversed) =>
          setSettings((current) => ({ ...current, sortField, sortReversed }))
        }
        onCreate={createNote}
        agentBadge={settings.agentBadge}
        rowHeight={rowHeight}
        focused={pane === "list"}
        sidebarHidden={settings.sidebarHidden}
        onToggleSidebar={() =>
          setSettings((current) => ({ ...current, sidebarHidden: !current.sidebarHidden }))
        }
        settingsOpen={prefs}
        onSettings={() => setPrefs(true)}
        empty={
          // Пустой vault и пустая выдача — разные состояния: в первом надо
          // предложить завести первую заметку, во втором — объяснить запрос.
          counts.All === 0 && filter.kind !== "trash" ? (
            <EmptyVault onCreate={createNote} />
          ) : (
            <EmptySearch
              query={query}
              onDropToken={(token) =>
                setQuery(
                  query
                    .split(/\s+/)
                    .filter((part) => part !== token)
                    .join(" "),
                )
              }
              onSearchAll={() => {
                setQuery("");
                onFilter({ kind: "all" });
              }}
            />
          )
        }
        footer={
          selected.length > 1 ? (
            <BulkPane
              count={selected.length}
              pinned={firstSelected?.Pinned ?? false}
              onStatus={(status: Status) => act(api.setStatusMany(selected, status), true)}
              onPin={() => togglePinned(selected)}
              onMove={() => setMoving(true)}
              onTrash={() => act(api.trashMany(selected))}
              onClear={() => setSelected([])}
            />
          ) : null
        }
      />

      <Splitter
        width={settings.listWidth}
        min={280}
        max={480}
        onChange={(value) => setSettings((current) => ({ ...current, listWidth: value }))}
      />

      {noteError && (
        <div className="pane pane--editor" data-focused={pane === "editor"}>
          <div className="drag-strip" />
          <div className="error">{noteError}</div>
        </div>
      )}
      {!noteError && !note && selected.length <= 1 && (
        <div className="pane pane--editor" data-focused={pane === "editor"}>
          <div className="drag-strip" />
          <div className="empty">
            <div className="empty__text">Выберите заметку слева</div>
            <div className="empty__keys">
              <span className="key">j / k</span>
              <span className="key">⏎</span>
              <span className="key">⌘N</span>
            </div>
          </div>
        </div>
      )}
      {selected.length > 1 && (
        <div className="pane pane--editor" data-focused={pane === "editor"}>
          <div className="drag-strip" />
          <div className="empty">
            <div className="empty__title">Выбрано заметок: {selected.length}</div>
            <div className="empty__text">
              Что с ними сделать — в панели под списком. Escape снимает выделение.
            </div>
          </div>
        </div>
      )}
      {!noteError && note && filter.kind === "trash" && (
        <div className="pane pane--editor" data-focused={pane === "editor"}>
          <div className="drag-strip" />
          <input className="editor__title" value={note.Title} readOnly />
          <div className="banner">
            <span className="banner__glyph">▚</span>
            <span className="banner__text">
              <span className="banner__title">Заметка в корзине — редактирование выключено</span>
              <span className="banner__note">{note.Path}</span>
            </span>
            <button className="button button--accent" onClick={() => act(api.restore(note.ID))}>
              Восстановить
            </button>
            <button
              className="button button--danger"
              onClick={() => act(api.deleteForever(note.ID))}
            >
              Удалить навсегда
            </button>
          </div>
          <div className="trashed-body">{note.Body}</div>
        </div>
      )}
      {!noteError && note && filter.kind !== "trash" && (
        // key по id: переключение заметки пересоздаёт редактор целиком, вместо
        // того чтобы подменять документ и сбрасывать состояние вима руками.
        <Editor
          // Настройки редактора входят в ключ: расширения CodeMirror
          // задаются при создании, и подменять их на лету значит собирать
          // компартменты ради галочки, которую трогают раз в год.
          key={`${note.ID}:${revision}:${settings.lineNumbers}:${settings.lineWrap}`}
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
          onStatus={(status) => act(api.setStatus(note.ID, status), true)}
          onMode={(mode) => (vimMode.current = mode)}
          saveDelay={settings.saveDelay}
          lineNumbers={settings.lineNumbers}
          lineWrap={settings.lineWrap}
          focused={pane === "editor"}
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
 * targets решает, к чему относится действие, начатое на одной строке.
 *
 * Строка внутри выделения — значит действие относится ко всей пачке; строка
 * снаружи — только к ней самой. Иначе правый клик по заметке за пределами
 * выделения молча утащил бы в корзину двадцать чужих.
 */
function targets(selected: string[], clicked: string): string[] {
  return selected.includes(clicked) ? selected : [clicked];
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
  if (filter.kind === "agent") parts.push("is:agent");
  // Несколько тегов соединяются через И — это делает сам язык запросов.
  if (filter.kind === "tags") parts.push(...filter.names.map((name) => `tag:${quote(name)}`));
  if (query.trim() !== "") parts.push(query.trim());
  return parts.join(" ");
}

/** quote берёт значение в кавычки, если в нём есть пробелы. */
function quote(value: string): string {
  return /\s/.test(value) ? `"${value}"` : value;
}
