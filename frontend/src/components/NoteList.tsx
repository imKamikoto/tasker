import { useLayoutEffect, useRef, useState } from "react";

import type { Note } from "../api";
import { noteCount, shortDate } from "../format";
import { progressBar } from "../progress";
import { splitQuery } from "../querytokens";
import type { SortField } from "../settings";
import { statusGlyphs, type Status } from "../statuses";
import { tagColor, tagStyle } from "../tags";
import { visibleWindow } from "../virtual";

/** Сколько строк рисовать про запас, чтобы быстрая прокрутка не мигала. */
const overscan = 6;

/** Сколько тегов помещается в метаполосу, не выдавливая остальное. */
const tagsInRow = 2;

type Props = {
  notes: Note[];
  selected: string[];
  query: string;
  error: string | null;
  /** Выбранные вручную цвета тегов: имя → номер в палитре. */
  tagColors: Record<string, number>;
  onQuery: (query: string) => void;
  onSelect: (id: string, modifiers: { toggle: boolean; range: boolean }) => void;
  onContext: (id: string, at: { x: number; y: number }) => void;
  onDragNote: (id: string) => void;
  onDragEnd: () => void;
  sortField: SortField;
  sortReversed: boolean;
  onSort: (field: SortField, reversed: boolean) => void;
  onCreate: () => void;
  /** Что показать вместо строк, когда их нет: пустой vault или пустая выдача. */
  empty: React.ReactNode;
  /** Прижатое к низу колонки: панель массовых операций, когда она нужна. */
  footer?: React.ReactNode;
  /** Отмечать заметки агента. Выключается в настройках. */
  agentBadge: boolean;
  /** Высота строки в пикселях. Растёт с масштабом текста, поэтому приходит
   *  снаружи: на ней стоит арифметика виртуализации. */
  rowHeight: number;
  /** Колонка принимает клавиши: показываем рамку. */
  focused: boolean;
  /** Сайдбар свёрнут: кнопка меняет вид, а полоса берёт на себя светофор. */
  sidebarHidden: boolean;
  onToggleSidebar: () => void;
  /** Шестерёнка живёт здесь, только пока сайдбара нет: со своей полосой он
   *  уносит её с собой, а настройки должны оставаться под рукой. */
  settingsOpen: boolean;
  onSettings: () => void;
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
  tagColors,
  onQuery,
  onSelect,
  onContext,
  onDragNote,
  onDragEnd,
  sortField,
  sortReversed,
  onSort,
  onCreate,
  empty,
  footer,
  agentBadge,
  rowHeight,
  focused,
  sidebarHidden,
  onToggleSidebar,
  settingsOpen,
  onSettings,
}: Props) {
  const scroller = useRef<HTMLDivElement | null>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [viewport, setViewport] = useState(0);
  const [sortMenu, setSortMenu] = useState(false);

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

  // Галочки появляются, только когда выбрано больше одной: на одной заметке
  // это лишний глиф в строке, где каждый символ на счету.
  const multi = selected.length > 1;

  return (
    <div className="pane pane--list" data-focused={focused}>
      {/* Со свёрнутым сайдбаром эта полоса становится самой левой, и место
          под системный светофор надо оставлять уже ей. */}
      <div className="drag-strip drag-strip--tools" data-lights={sidebarHidden}>
        <button
          className="stripbutton"
          aria-label={sidebarHidden ? "Показать сайдбар" : "Скрыть сайдбар"}
          aria-pressed={sidebarHidden}
          title={`${sidebarHidden ? "Показать" : "Скрыть"} сайдбар (⌘/)`}
          onClick={onToggleSidebar}
        >
          {sidebarHidden ? "⇥" : "⇤"}
        </button>
        {sidebarHidden && (
          <button
            className="gear"
            aria-label="Настройки"
            aria-expanded={settingsOpen}
            title="Настройки (⌘,)"
            onClick={onSettings}
          >
            {"\u2699\uFE0E"}
          </button>
        )}
      </div>

      <div className="listbar">
        {/* Кнопка рядом с полем, а не внутри: она не имеет к поиску никакого
            отношения, а внутри читалась бы как «искать» или «очистить». */}
        <div className="listbar__row">
        <div className="search">
          <span className="search__glyph">⌕</span>
          <div className="search__field">
            {/* Зеркало под полем красит запрос: сам input покрасить свои куски
                не умеет, а видеть, что распозналось фильтром, надо до отправки. */}
            <div className="search__ghost" aria-hidden="true">
              {splitQuery(query).map((token, i) => (
                <span key={i} data-kind={token.kind}>
                  {token.text}
                </span>
              ))}
            </div>
            <input
              className="search__input"
              placeholder="слово, tag:баг, status:active…"
              value={query}
              onChange={(event) => onQuery(event.target.value)}
              spellCheck={false}
              autoCorrect="off"
            />
          </div>
        </div>
          <button className="listbar__new" title="Новая заметка (⌘N)" onClick={onCreate}>
            +
          </button>
        </div>

        <div className="listbar__meta">
          <span className="listbar__count">{noteCount(notes.length)}</span>
          <span className="listbar__rule" />
          {/* Одна кнопка вместо трёх: сортировку меняют редко, а место в этой
              строке нужнее под счётчик. */}
          <button className="listbar__sort" onClick={() => setSortMenu((open) => !open)}>
            {sortLabels[sortField]} {sortReversed ? "↑" : "↓"}
            {sortMenu && (
              <div className="menu menu--sort" onMouseLeave={() => setSortMenu(false)}>
                {(Object.keys(sortLabels) as SortField[]).map((field) => (
                  <button
                    key={field}
                    className="menu__item"
                    aria-selected={field === sortField}
                    // Повторное нажатие по уже выбранному полю переворачивает
                    // порядок: отдельная кнопка направления заняла бы место
                    // ради одного щелчка.
                    onClick={(event) => {
                      event.stopPropagation();
                      onSort(field, field === sortField ? !sortReversed : false);
                      setSortMenu(false);
                    }}
                  >
                    <span className="menu__label">{sortLabels[field]}</span>
                    {field === sortField && (
                      <span className="menu__check">{sortReversed ? "↑" : "↓"}</span>
                    )}
                  </button>
                ))}
              </div>
            )}
          </button>
        </div>
      </div>

      {error && <div className="error">{error}</div>}
      {!error && notes.length === 0 && empty}

      <div
        className="notes"
        ref={scroller}
        onScroll={(event) => setScrollTop(event.currentTarget.scrollTop)}
      >
        <div style={{ height: visible.padTop }} />
        {notes.slice(visible.first, visible.end).map((note) => {
          const agent = agentBadge && note.Origin === "agent";
          const tags = note.Tags ?? [];
          return (
            <button
              key={note.ID}
              className="note"
              aria-selected={selected.includes(note.ID)}
              data-status={note.Status}
              data-origin={agent ? "agent" : "user"}
              onClick={(event) =>
                // metaKey — Cmd на macOS; платформа у нас одна (SPEC §9).
                onSelect(note.ID, { toggle: event.metaKey, range: event.shiftKey })
              }
              onContextMenu={(event) => {
                event.preventDefault();
                onContext(note.ID, { x: event.clientX, y: event.clientY });
              }}
              draggable
              // Тащим идентификатор: ноутбук на той стороне сам решит, что с ним
              // делать, и знать о списке ему не нужно.
              onDragStart={(event) => {
                event.dataTransfer.setData("text/tasker-note", note.ID);
                onDragNote(note.ID);
              }}
              onDragEnd={onDragEnd}
            >
              <div className="note__head">
                {multi ? (
                  <span className="note__check">
                    {selected.includes(note.ID) ? "▣" : "□"}
                  </span>
                ) : (
                  <span className="note__mark" data-pinned={note.Pinned}>
                    {agent ? "◆" : note.Pinned ? "★" : "☆"}
                  </span>
                )}
                <span className="note__title">{note.Title}</span>
              </div>

              <div className="note__excerpt">{note.Excerpt}</div>

              <div className="note__meta">
                {agent && <span className="note__agent">AGENT</span>}
                {/* Только глиф, без слова: форма и цвет различают все пять
                    статусов, а слово занимало сорок пикселей строки, которых
                    не хватало тегам. Полное название — в подсказке и в
                    плашке редактора. */}
                {note.Status !== "none" && (
                  <span className="note__status" data-status={note.Status} title={note.Status}>
                    {statusGlyphs[note.Status as Status] ?? ""}
                  </span>
                )}
                {note.Status !== "none" && <span className="note__sep">│</span>}
                <span>{shortDate(note.Updated)}</span>
                {note.NumTasks > 0 && (
                  <>
                    <span className="note__sep">│</span>
                    <span className="note__progress">
                      {progressBar(note.NumDone, note.NumTasks)} {note.NumDone}/{note.NumTasks}
                    </span>
                  </>
                )}
                {tags.length > 0 && (
                  <>
                    <span className="note__sep">│</span>
                    <span className="note__tags">
                      {/* Имена в отдельной обёртке: сжимается и обрезается
                          только она, а счётчик остальных виден всегда. */}
                      <span className="note__tags-names">
                        {tags.slice(0, tagsInRow).map((tag) => (
                          <span
                            key={tag}
                            className="note__tag"
                            style={tagStyle(tagColor(tag, tagColors))}
                          >
                            #{tag}
                          </span>
                        ))}
                      </span>
                      {tags.length > tagsInRow && (
                        <span className="note__tags-more">+{tags.length - tagsInRow}</span>
                      )}
                    </span>
                  </>
                )}
              </div>
            </button>
          );
        })}
        <div style={{ height: visible.padBottom }} />
      </div>

      {footer}
    </div>
  );
}
