import { useCallback, useEffect, useRef, useState } from "react";

import { api, describeError, events, subscribe, type Note, type NoteRecord } from "../api";
import { backlinkCount } from "../format";
import { statusGlyphs, statuses, type Status } from "../statuses";
import { CodeMirror, type EditorStatus, type MarkupKind } from "./CodeMirror";
import { MarkupToolbar } from "./MarkupToolbar";
import type { SelectionRect } from "../toolbar";
import { ConflictModal } from "./ConflictModal";
import { TagField } from "./TagField";

/**
 * base64 читает файл в строку для передачи в Go.
 *
 * Через FileReader и data-URI, а не побайтово: btoa над длинной строкой из
 * arrayBuffer на нескольких мегабайтах заметно подвешивает вкладку, а
 * readAsDataURL делает ту же работу в браузере.
 */
function base64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(reader.error ?? new Error("не прочитался файл"));
    reader.onload = () => {
      const result = String(reader.result);
      // data:<тип>;base64,<данные> — нужна только вторая половина.
      const comma = result.indexOf(",");
      resolve(comma < 0 ? "" : result.slice(comma + 1));
    };
    reader.readAsDataURL(file);
  });
}

/** Шоткаты статусов в меню. Индекс совпадает с порядком в statuses. */
const statusKeys = ["⌘⌃1", "⌘⌃2", "⌘⌃3", "⌘⌃4", "⌘⌃5"];

type Props = {
  note: Note;
  onSaved: (record: NoteRecord) => void;
  /** Сообщает наружу, есть ли несохранённое: от этого зависит, можно ли
   *  перечитать заметку, изменившуюся на диске. */
  onDirty: (dirty: boolean) => void;
  onClose: () => void;
  /** Файл изменился на диске, пока в буфере есть несохранённое. */
  conflict: boolean;
  focusToken: number;
  knownTags: string[];
  onTags: (tags: string[]) => void;
  tagColors: Record<string, number>;
  onTagColor: (tag: string, color: number) => void;
  onStatus: (status: Status) => void;
  /** Режим вима наружу: от него зависит, можно ли отобрать сочетание у текста. */
  onMode: (mode: string) => void;
  onReload: () => void;
  onKeepMine: () => void;
  /** Сколько ждать после последней правки перед записью, мс. */
  saveDelay: number;
  vimEnabled: boolean;
  /** Открыть другую заметку: по ссылке из текста или из списка обратных. */
  onOpenNote: (id: string) => void;
  lineNumbers: boolean;
  lineWrap: boolean;
  /** Колонка принимает клавиши: показываем полосу, как у соседей. */
  focused: boolean;
};

type SaveState = "clean" | "dirty" | "saving" | "failed";

/**
 * Editor — заголовок, тело и сохранение.
 *
 * Родитель монтирует его заново на каждую заметку (key по id), поэтому здесь
 * не нужно ни подменять документ, ни сбрасывать состояние вима: этим
 * занимается React.
 */
export function Editor({
  note,
  onSaved,
  onDirty,
  onClose,
  conflict,
  focusToken,
  knownTags,
  onTags,
  tagColors,
  onTagColor,
  onStatus,
  onMode,
  onReload,
  onKeepMine,
  saveDelay,
  vimEnabled,
  onOpenNote,
  lineNumbers,
  lineWrap,
  focused,
}: Props) {
  const [title, setTitle] = useState(note.Title);
  const [state, setState] = useState<SaveState>("clean");
  const [error, setError] = useState<string | null>(null);
  const [savedAt, setSavedAt] = useState<string | null>(null);
  const [statusMenu, setStatusMenu] = useState(false);
  const [backlinksOpen, setBacklinksOpen] = useState(false);
  // Плавающий тулбар: где стоит выделение и какую разметку просили применить.
  const pane = useRef<HTMLDivElement | null>(null);

  // Файл едет в Go строкой base64: массив байтов через биндинги превращается
  // в числа по одному. Разметку собирает тоже Go — он же решает, картинка это
  // или обычное вложение.
  const attach = async (file: File): Promise<string> => {
    try {
      const saved = await api.addAttachment(file.name, await base64(file));
      return saved.Markdown;
    } catch (err) {
      setError(describeError(err));
      throw err;
    }
  };
  const [selection, setSelection] = useState<SelectionRect | null>(null);
  const [markup, setMarkup] = useState<{ kind: MarkupKind | ""; token: number }>({
    kind: "",
    token: 0,
  });
  // Пустой режим при выключенном виме: режимов нет, и врать «NORMAL» до
  // первого нажатия нельзя — в строке статуса это единственное место, где
  // видно, включён вим или нет.
  const [status, setStatus] = useState<EditorStatus>({
    mode: vimEnabled ? "NORMAL" : "",
    line: 1,
    column: 1,
  });

  // Текущее содержимое держим в ref, а не в state: перерисовывать панель на
  // каждое нажатие незачем, а сохранению нужны свежие значения — в том числе
  // при размонтировании, куда состояние уже не доедет.
  const latest = useRef({ title: note.Title, body: note.Body, dirty: false });
  const timer = useRef<number | null>(null);

  const save = useCallback(async () => {
    if (timer.current !== null) {
      clearTimeout(timer.current);
      timer.current = null;
    }
    if (!latest.current.dirty) return;

    const { title: nextTitle, body } = latest.current;
    latest.current.dirty = false;
    onDirty(false);
    setState("saving");
    try {
      const saved = await api.save(note.ID, nextTitle, body);
      setState("clean");
      setSavedAt(new Date().toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit" }));
      setError(null);
      onSaved(saved);
    } catch (err) {
      // Правку не теряем: она осталась в буфере, и следующая попытка её заберёт.
      latest.current.dirty = true;
      onDirty(true);
      setState("failed");
      setError(describeError(err));
    }
  }, [note.ID, onSaved, onDirty]);

  const schedule = useCallback(() => {
    latest.current.dirty = true;
    onDirty(true);
    setState("dirty");
    if (timer.current !== null) clearTimeout(timer.current);
    timer.current = window.setTimeout(save, saveDelay);
  }, [save, onDirty, saveDelay]);

  // Несохранённое при закрытии заметки обязано уехать на диск: переключение
  // заметки — это размонтирование, и другого шанса не будет.
  const saveRef = useRef(save);
  saveRef.current = save;
  useEffect(() => {
    return () => {
      void saveRef.current();
    };
  }, []);

  // Окно закрывается: Go ждёт, пока буфер уедет на диск, и только потом
  // отпускает закрытие (SPEC §6).
  useEffect(() => {
    return subscribe(events.beforeClose, () => {
      void saveRef
        .current()
        .catch(() => undefined)
        .finally(() => void api.readyToClose());
    });
  }, []);

  const onTitle = useCallback(
    (value: string) => {
      setTitle(value);
      latest.current.title = value;
      schedule();
    },
    [schedule],
  );

  const onBody = useCallback(
    (value: string) => {
      latest.current.body = value;
      schedule();
    },
    [schedule],
  );

  const backlinks = note.Backlinks?.length ?? 0;

  return (
    <div className="pane pane--editor" data-focused={focused} ref={pane}>
      <div className="drag-strip" />

      <input
        className="editor__title"
        value={title}
        onChange={(event) => onTitle(event.target.value)}
        spellCheck={false}
        autoCorrect="off"
        placeholder="Без заголовка"
      />

      <div className="editor__meta">
        <span className="editor__book">{(note.Notebook || "Все заметки").replace(/\//g, " / ")}</span>
        <span className="meta__sep">│</span>
        {/* Плашка статуса — она же кнопка: место, где статус показан, и место,
            где он меняется, должны совпадать. */}
        <span style={{ position: "relative" }}>
          <button
            className="status-pill"
            data-status={note.Status}
            onClick={() => setStatusMenu((open) => !open)}
          >
            {statusGlyphs[note.Status as Status] ?? "·"} {note.Status}
          </button>
          {statusMenu && (
            <div className="menu menu--sort" onMouseLeave={() => setStatusMenu(false)}>
              {statuses.map((value, i) => (
                <button
                  key={value}
                  className="menu__item"
                  aria-selected={value === note.Status}
                  onClick={() => {
                    setStatusMenu(false);
                    onStatus(value);
                  }}
                >
                  <span className="menu__glyph" data-status={value}>
                    {statusGlyphs[value]}
                  </span>
                  <span className="menu__label">{value}</span>
                  {value === note.Status && <span className="menu__check">✓</span>}
                  <span className="menu__key">{statusKeys[i]}</span>
                </button>
              ))}
            </div>
          )}
        </span>
        {backlinks > 0 && (
          <>
            <span className="meta__sep">│</span>
            {/* Счётчик сам раскрывает список: знать, что на заметку кто-то
                ссылается, и не иметь способа посмотреть кто — половина
                ответа. Панелью под редактором это станет в фазе 7, пока
                хватает всплывающего списка на том же месте. */}
            <span className="backlinks">
              <button
                className="backlinks__count"
                aria-expanded={backlinksOpen}
                onClick={() => setBacklinksOpen((open) => !open)}
              >
                {backlinkCount(backlinks)}
              </button>
              {backlinksOpen && (
                <div className="menu menu--backlinks" onMouseLeave={() => setBacklinksOpen(false)}>
                  {(note.Backlinks ?? []).map((item) => (
                    <button
                      key={item.ID}
                      className="menu__item"
                      onClick={() => {
                        setBacklinksOpen(false);
                        onOpenNote(item.ID);
                      }}
                    >
                      <span className="menu__label">{item.Title}</span>
                    </button>
                  ))}
                </div>
              )}
            </span>
          </>
        )}
        <span className="editor__hints">
          <span className="hint">⌘N новая</span>
          <span className="editor__state" data-state={state}>
            {stateLabel(state, savedAt)}
          </span>
        </span>
      </div>

      <TagField
        tags={note.Tags ?? []}
        known={knownTags}
        onChange={onTags}
        colors={tagColors}
        onColor={onTagColor}
      />

      <div className="editor__rule" />

      {error && <div className="error">{error}</div>}

      <div className="editor__body">
        <CodeMirror
          initialDoc={note.Body}
          onChange={onBody}
          onWrite={save}
          onQuit={onClose}
          focusToken={focusToken}
          onStatus={(next) => {
            setStatus(next);
            onMode(next.mode);
          }}
          vimEnabled={vimEnabled}
          onOpenNote={onOpenNote}
          onAttach={attach}
          onSelection={setSelection}
          markup={markup}
          lineNumbers={lineNumbers}
          lineWrap={lineWrap}
        />
        <MarkupToolbar
          selection={selection}
          container={pane.current?.getBoundingClientRect() ?? null}
          onApply={(kind) => setMarkup((current) => ({ kind, token: current.token + 1 }))}
        />
      </div>

      <div className="editor__status">
        {status.mode !== "" && <span className="editor__mode">{status.mode}</span>}
        <span>{note.Path}</span>
        <span className="editor__status-right">
          <span>markdown</span>
          <span>utf-8</span>
          <span>
            {status.line}:{status.column}
          </span>
        </span>
      </div>

      {conflict && (
        <ConflictModal
          path={note.Path}
          fromAgent={note.Origin === "agent"}
          onReload={onReload}
          onKeepMine={onKeepMine}
        />
      )}
    </div>
  );
}

function stateLabel(state: SaveState, at: string | null): string {
  switch (state) {
    case "saving":
      return "● сохраняю…";
    case "dirty":
      return "● не сохранено";
    case "failed":
      return "● ошибка записи";
    default:
      return at ? `● сохранено ${at}` : "● сохранено";
  }
}
