import { useCallback, useEffect, useRef, useState } from "react";

import { api, describeError, events, subscribe, type Note, type NoteRecord } from "../api";
import { CodeMirror } from "./CodeMirror";
import { TagField } from "./TagField";

/** Сколько ждать после последней правки перед записью (SPEC §, фаза 3). */
const saveDelay = 400;

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
  onReload: () => void;
  onKeepMine: () => void;
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
  onReload,
  onKeepMine,
}: Props) {
  const [title, setTitle] = useState(note.Title);
  const [state, setState] = useState<SaveState>("clean");
  const [error, setError] = useState<string | null>(null);

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
  }, [save, onDirty]);

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

  return (
    <div className="pane pane--editor">
      <input
        className="editor__title"
        value={title}
        onChange={(event) => onTitle(event.target.value)}
        spellCheck={false}
        autoCorrect="off"
        placeholder="Без заголовка"
      />
      <div className="editor__meta">
        <span>{note.Notebook || "Корень"}</span>
        {note.Status !== "none" && <span className="status">{note.Status}</span>}
        {(note.Backlinks?.length ?? 0) > 0 && <span>ссылаются: {note.Backlinks?.length}</span>}
        <span className="editor__state" data-state={state}>
          {stateLabel(state)}
        </span>
      </div>

      <TagField tags={note.Tags ?? []} known={knownTags} onChange={onTags} />

      {conflict && (
        <div className="conflict">
          <span>Файл изменён снаружи, а здесь есть несохранённое.</span>
          <button onClick={onReload}>Взять с диска</button>
          <button onClick={onKeepMine}>Оставить моё</button>
        </div>
      )}

      {error && <div className="error">{error}</div>}

      <div className="editor__body">
        <CodeMirror
          initialDoc={note.Body}
          onChange={onBody}
          onWrite={save}
          onQuit={onClose}
          focusToken={focusToken}
        />
      </div>
    </div>
  );
}

function stateLabel(state: SaveState): string {
  switch (state) {
    case "saving":
      return "сохраняю…";
    case "dirty":
      return "не сохранено";
    case "failed":
      return "ошибка записи";
    default:
      return "сохранено";
  }
}
