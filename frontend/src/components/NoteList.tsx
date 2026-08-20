import { useLayoutEffect, useRef, useState } from "react";

import type { Note } from "../api";
import { visibleWindow } from "../virtual";

/** Высота строки списка. Совпадает с --row-height в стилях. */
const rowHeight = 104;

/** Сколько строк рисовать про запас, чтобы быстрая прокрутка не мигала. */
const overscan = 6;

type Props = {
  notes: Note[];
  selected: string | null;
  query: string;
  error: string | null;
  onQuery: (query: string) => void;
  onSelect: (id: string) => void;
};

/**
 * NoteList — поле запроса и список найденного.
 *
 * Рисуются только видимые строки: десять тысяч заметок должны скроллиться без
 * лагов (SPEC §8.4), а десять тысяч узлов DOM этого не дают.
 */
export function NoteList({ notes, selected, query, error, onQuery, onSelect }: Props) {
  const scroller = useRef<HTMLDivElement | null>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [viewport, setViewport] = useState(0);

  // Высоту окна меряем после отрисовки и следим за изменением: разделители
  // тянутся мышью, и колонка меняет размер без перезагрузки.
  useLayoutEffect(() => {
    const element = scroller.current;
    if (!element) return;

    const measure = () => setViewport(element.clientHeight);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  // Не window: имя затеняло бы глобальный объект.
  const visible = visibleWindow({
    total: notes.length,
    rowHeight,
    viewport,
    scrollTop,
    overscan,
  });

  return (
    <div className="pane pane--list">
      <input
        className="search"
        placeholder="Поиск: слово, tag:баг, status:active…"
        value={query}
        onChange={(event) => onQuery(event.target.value)}
        spellCheck={false}
        autoCorrect="off"
      />

      {error && <div className="error">{error}</div>}
      {!error && notes.length === 0 && <div className="empty">Ничего не найдено</div>}

      <div
        className="notes"
        ref={scroller}
        onScroll={(event) => setScrollTop(event.currentTarget.scrollTop)}
      >
        <div style={{ height: visible.padTop }} />
        {notes.slice(visible.first, visible.end).map((note) => (
        <button
          key={note.ID}
          className="note"
          aria-selected={note.ID === selected}
          onClick={() => onSelect(note.ID)}
        >
          <div className="note__title">
            {note.Pinned && <span className="pin">★</span>}
            <span>{note.Title}</span>
          </div>
          {note.Excerpt && <div className="note__excerpt">{note.Excerpt}</div>}
          <div className="note__meta">
            <span>{formatDate(note.Updated)}</span>
            {note.Status !== "none" && <span className="status">{note.Status}</span>}
            {note.NumTasks > 0 && (
              <span>
                {note.NumDone}/{note.NumTasks}
              </span>
            )}
            {(note.Tags ?? []).map((tag) => (
              <span key={tag} className="tag">
                #{tag}
              </span>
            ))}
          </div>
        </button>
        ))}
        <div style={{ height: visible.padBottom }} />
      </div>
    </div>
  );
}

/**
 * formatDate печатает дату так, как её показал бы Finder: сегодняшнее время,
 * всё остальное — датой.
 */
function formatDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";

  const now = new Date();
  const sameDay =
    date.getFullYear() === now.getFullYear() &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate();

  return sameDay
    ? date.toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit" })
    : date.toLocaleDateString("ru-RU", { day: "2-digit", month: "2-digit", year: "numeric" });
}
