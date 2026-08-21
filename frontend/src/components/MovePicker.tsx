import { useEffect, useMemo, useRef, useState } from "react";

import { noteCountTarget } from "../format";
import { filterPaths, movePick } from "../picker";

type Props = {
  notebooks: string[];
  /** Счётчики по путям — их же показывает сайдбар. */
  counts: Record<string, number>;
  /** Сколько заметок переносим: об этом говорит заголовок модалки. */
  count: number;
  onPick: (notebook: string) => void;
  /** Создать ноутбук с таким именем и перенести туда же. */
  onCreate: (notebook: string) => void;
  onCancel: () => void;
};

/**
 * MovePicker — выбор ноутбука для перемещения заметки (SPEC §8.4, клавиша m).
 *
 * Работает целиком с клавиатуры: набрал часть имени, стрелками выбрал, Enter.
 * Мышь тут не нужна — за неё отвечает перетаскивание.
 */
export function MovePicker({ notebooks, counts, count, onPick, onCreate, onCancel }: Props) {
  const [query, setQuery] = useState("");
  const [index, setIndex] = useState(0);
  const input = useRef<HTMLInputElement | null>(null);

  const matches = useMemo(() => filterPaths(notebooks, query), [notebooks, query]);
  const typed = query.trim();
  // Последним пунктом — создание, если набранное не совпало ни с чем точно.
  // Так «перенести в новый ноутбук» не требует сначала выйти и создать его.
  const creatable = typed !== "" && !notebooks.includes(typed);
  const total = matches.length + (creatable ? 1 : 0);

  useEffect(() => input.current?.focus(), []);
  // Список сузился под курсором — возвращаем выбор в границы.
  useEffect(
    () => setIndex((current) => Math.min(current, Math.max(0, total - 1))),
    [total],
  );

  const choose = (position: number) => {
    if (creatable && position === matches.length) onCreate(typed);
    else if (matches.length > 0) onPick(matches[position]);
  };

  return (
    <div className="overlay" onMouseDown={onCancel}>
      <div className="modal picker" onMouseDown={(event) => event.stopPropagation()}>
        <div className="modal__head">
          <span>Перенести {noteCountTarget(count)} в…</span>
          <span className="key">m</span>
        </div>
        <input
          ref={input}
          className="picker__input"
          placeholder="Имя ноутбука"
          value={query}
          spellCheck={false}
          autoCorrect="off"
          onChange={(event) => setQuery(event.target.value)}
          onKeyDown={(event) => {
            switch (event.key) {
              case "Escape":
                event.preventDefault();
                onCancel();
                return;
              case "Enter":
                event.preventDefault();
                choose(index);
                return;
              case "ArrowDown":
              case "ArrowUp":
                event.preventDefault();
                setIndex((current) =>
                  movePick(total, current, event.key === "ArrowDown" ? 1 : -1),
                );
                return;
            }
          }}
        />
        <div className="picker__list">
          {total === 0 && <div className="empty empty--inline">Такого ноутбука нет</div>}
          {matches.map((path, position) => (
            <div
              key={path}
              className="row"
              aria-selected={position === index}
              onMouseEnter={() => setIndex(position)}
              onClick={() => onPick(path)}
            >
              <span className="row__glyph">{path === "" ? "≡" : "└"}</span>
              <span className="row__label">{path === "" ? "Все заметки" : path}</span>
              <span className="row__count">{counts[path] || ""}</span>
            </div>
          ))}
          {creatable && (
            <div
              className="row"
              aria-selected={index === matches.length}
              style={{ color: "var(--color-accent)" }}
              onMouseEnter={() => setIndex(matches.length)}
              onClick={() => onCreate(typed)}
            >
              <span className="row__label">+ Создать ноутбук «{typed}»</span>
            </div>
          )}
        </div>
        <div className="modal__foot">
          <span className="key">↑↓</span>
          <span>выбрать</span>
          <span className="key">⏎</span>
          <span>перенести</span>
          <span className="key">esc</span>
          <span>отменить</span>
        </div>
      </div>
    </div>
  );
}
