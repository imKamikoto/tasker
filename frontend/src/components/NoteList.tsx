import { useLayoutEffect, useRef, useState } from "react";

import type { Note } from "../api";
import type { SortField } from "../settings";
import { visibleWindow } from "../virtual";

/** Высота строки списка. Совпадает с --row-height в стилях. */
const rowHeight = 104;

/** Сколько строк рисовать про запас, чтобы быстрая прокрутка не мигала. */
const overscan = 6;

type Props = {
  notes: Note[];
  selected: string[];
  query: string;
  error: string | null;
  onQuery: (query: string) => void;
  onSelect: (id: string, modifiers: { toggle: boolean; range: boolean }) => void;
  sortField: SortField;
  sortReversed: boolean;
  onSort: (field: SortField, reversed: boolean) => void;
  onCreate: () => void;
};

/** Подписи сортировок. Порядок — как в SPEC §8.4. */
const sortLabels: Record<SortField, string> = {
  title: "заголовок",
  created: "создано",
  updated: "изменено",
};

/**
 * NoteList — поле запроса и список найденного.
 *
 * Рисуются только видимые строки: десять тысяч заметок должны скроллиться без
 * лагов (SPEC §8.4), а десять тысяч узлов DOM этого не дают.
 */
export function NoteList({
  notes,
  selected,
  query,
  error,
  onQuery,
  onSelect,
  sortField,
  sortReversed,
  onSort,
  onCreate,
}: Props) {
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
      <div className="listbar">
        <input
          className="search"
          placeholder="Поиск: слово, tag:баг, status:active…"
          value={query}
          onChange={(event) => onQuery(event.target.value)}
          spellCheck={false}
          autoCorrect="off"
        />
        <button className="listbar__add" title="Новая заметка (Cmd+N)" onClick={onCreate}>
          +
        </button>
      </div>

      <div className="sortbar">
        {(Object.keys(sortLabels) as SortField[]).map((field) => (
          <button
            key={field}
            className="sortbar__field"
            aria-selected={field === sortField}
            // Повторное нажатие по уже выбранному полю переворачивает порядок:
            // отдельная кнопка направления заняла бы место ради одного щелчка.
            onClick={() => onSort(field, field === sortField ? !sortReversed : false)}
          >
            {sortLabels[field]}
            {field === sortField && <span className="sortbar__arrow">{sortReversed ? "↑" : "↓"}</span>}
          </button>
        ))}
      </div>

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
          aria-selected={selected.includes(note.ID)}
          onClick={(event) =>
            // metaKey — Cmd на macOS; платформа у нас одна (SPEC §9).
            onSelect(note.ID, { toggle: event.metaKey, range: event.shiftKey })
          }
          draggable
          // Тащим идентификатор: ноутбук на той стороне сам решит, что с ним
          // делать, и знать о списке ему не нужно.
          onDragStart={(event) => event.dataTransfer.setData("text/tasker-note", note.ID)}
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
