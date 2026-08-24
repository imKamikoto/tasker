import { useEffect, useMemo, useRef, useState } from "react";

import type { Template } from "../api";
import { filterPaths, movePick } from "../picker";

type Props = {
  templates: Template[];
  onPick: (path: string) => void;
  onCancel: () => void;
};

/**
 * TemplatePicker — выбор шаблона для заметки (SPEC §8.10, ⌘T).
 *
 * Тот же приём, что и у выбора ноутбука: набрал часть имени, стрелками выбрал,
 * Enter. Отбор идёт по имени файла, а не по заголовку из `_template`: имя
 * человек придумывал сам и помнит его, а заголовок может быть заготовкой вроде
 * «Баг: » и в поиске бесполезен.
 */
export function TemplatePicker({ templates, onPick, onCancel }: Props) {
  const [query, setQuery] = useState("");
  const [index, setIndex] = useState(0);
  const input = useRef<HTMLInputElement | null>(null);

  const names = useMemo(() => templates.map((item) => item.Name), [templates]);
  const matches = useMemo(() => filterPaths(names, query), [names, query]);
  const byName = useMemo(
    () => new Map(templates.map((item) => [item.Name, item])),
    [templates],
  );

  useEffect(() => input.current?.focus(), []);
  useEffect(
    () => setIndex((current) => Math.min(current, Math.max(0, matches.length - 1))),
    [matches.length],
  );

  const choose = (position: number) => {
    const found = byName.get(matches[position]);
    if (found) onPick(found.Path);
  };

  return (
    <div className="overlay" onMouseDown={onCancel}>
      <div className="modal picker" onMouseDown={(event) => event.stopPropagation()}>
        <div className="modal__head">
          <span>Шаблон</span>
          <span className="key">⌘T</span>
        </div>
        <input
          ref={input}
          className="picker__input"
          placeholder="Имя шаблона"
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
                  movePick(matches.length, current, event.key === "ArrowDown" ? 1 : -1),
                );
                return;
            }
          }}
        />
        <div className="picker__list">
          {templates.length === 0 && (
            <div className="empty empty--inline">
              Шаблонов нет. Положите любой .md в папку templates внутри хранилища.
            </div>
          )}
          {templates.length > 0 && matches.length === 0 && (
            <div className="empty empty--inline">Такого шаблона нет</div>
          )}
          {matches.map((name, position) => {
            const item = byName.get(name);
            return (
              <div
                key={name}
                className="row row--template"
                aria-selected={position === index}
                onMouseEnter={() => setIndex(position)}
                onClick={() => item && onPick(item.Path)}
              >
                <span className="row__label">{name}</span>
                {/* Превью — чтобы было видно, что именно вставится: имя файла
                    об этом не говорит ничего. */}
                <span className="row__preview">{item?.Preview}</span>
              </div>
            );
          })}
        </div>
        <div className="modal__foot">
          <span className="key">↑↓</span>
          <span>выбрать</span>
          <span className="key">⏎</span>
          <span>применить</span>
          <span className="key">esc</span>
          <span>отменить</span>
        </div>
      </div>
    </div>
  );
}
