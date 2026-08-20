import { useEffect, useMemo, useRef, useState } from "react";

import { filterPaths, movePick } from "../picker";

type Props = {
  notebooks: string[];
  onPick: (notebook: string) => void;
  onCancel: () => void;
};

/**
 * MovePicker — выбор ноутбука для перемещения заметки (SPEC §8.4, клавиша m).
 *
 * Работает целиком с клавиатуры: набрал часть имени, стрелками выбрал, Enter.
 * Мышь тут не нужна — за неё отвечает перетаскивание.
 */
export function MovePicker({ notebooks, onPick, onCancel }: Props) {
  const [query, setQuery] = useState("");
  const [index, setIndex] = useState(0);
  const input = useRef<HTMLInputElement | null>(null);

  const matches = useMemo(() => filterPaths(notebooks, query), [notebooks, query]);

  useEffect(() => input.current?.focus(), []);
  // Список сузился под курсором — возвращаем выбор в границы.
  useEffect(() => setIndex((current) => Math.min(current, Math.max(0, matches.length - 1))), [matches.length]);

  return (
    <div className="overlay" onMouseDown={onCancel}>
      <div className="picker" onMouseDown={(event) => event.stopPropagation()}>
        <input
          ref={input}
          className="picker__input"
          placeholder="Куда перенести…"
          value={query}
          spellCheck={false}
          onChange={(event) => setQuery(event.target.value)}
          onKeyDown={(event) => {
            switch (event.key) {
              case "Escape":
                event.preventDefault();
                onCancel();
                return;
              case "Enter":
                event.preventDefault();
                if (matches.length > 0) onPick(matches[index]);
                return;
              case "ArrowDown":
              case "ArrowUp":
                event.preventDefault();
                setIndex((current) => movePick(matches.length, current, event.key === "ArrowDown" ? 1 : -1));
                return;
            }
          }}
        />
        <div className="picker__list">
          {matches.length === 0 && <div className="empty">Такого ноутбука нет</div>}
          {matches.map((path, position) => (
            <button
              key={path}
              className="row"
              aria-selected={position === index}
              onMouseEnter={() => setIndex(position)}
              onClick={() => onPick(path)}
            >
              <span className="row__label">{path === "" ? "Корень" : path}</span>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
