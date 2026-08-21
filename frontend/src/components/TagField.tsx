import { useState } from "react";

import { addTag, autoColor, suggestTags, tagColor, tagPalette } from "../tags";

type Props = {
  tags: string[];
  known: string[];
  onChange: (tags: string[]) => void;
  /** Выбранные вручную цвета: имя тега → номер в палитре. */
  colors: Record<string, number>;
  onColor: (tag: string, color: number) => void;
};

/**
 * TagField — теги заметки под заголовком (SPEC §8.2).
 *
 * Набор правится целиком: добавили или убрали — наружу уходит новый список, а
 * не разница. Вычислять разницу должен тот, кто пишет файл, а не поле ввода.
 */
export function TagField({ tags, known, onChange, colors, onColor }: Props) {
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);
  const [painting, setPainting] = useState<string | null>(null);

  const suggestions = suggestTags(known, tags, query);

  const commit = (tag: string) => {
    const next = addTag(tags, tag);
    setQuery("");
    setOpen(false);
    // Повтор возвращает тот же массив — записывать нечего.
    if (next !== tags) onChange(next);
  };

  return (
    <div className="tagfield">
      {tags.map((tag) => (
        <span key={tag} className="chip" data-color={tagColor(tag, colors)}>
          {/* Щелчок по имени открывает палитру: отдельной кнопки «цвет» нет,
              а само имя ничего другого не делает. */}
          <button className="chip__name" onClick={() => setPainting(painting === tag ? null : tag)}>
            #{tag}
          </button>
          <button
            className="chip__remove"
            aria-label={`убрать ${tag}`}
            onClick={() => onChange(tags.filter((other) => other !== tag))}
          >
            ×
          </button>

          {painting === tag && (
            <div className="palette">
              <button
                className="palette__auto"
                onClick={() => {
                  onColor(tag, autoColor);
                  setPainting(null);
                }}
              >
                авто
              </button>
              {Array.from({ length: tagPalette }, (_, color) => (
                <button
                  key={color}
                  className="palette__swatch"
                  data-color={color}
                  aria-label={`цвет ${color}`}
                  onClick={() => {
                    onColor(tag, color);
                    setPainting(null);
                  }}
                />
              ))}
            </div>
          )}
        </span>
      ))}

      <div className="tagfield__entry">
        <input
          className="tagfield__input"
          placeholder="+ тег"
          value={query}
          spellCheck={false}
          onChange={(event) => {
            setQuery(event.target.value);
            setOpen(true);
          }}
          onFocus={() => setOpen(true)}
          // Закрываем с задержкой: щелчок по подсказке снимает фокус раньше,
          // чем успевает сработать, и без этого список исчезает под курсором.
          onBlur={() => window.setTimeout(() => setOpen(false), 120)}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              commit(suggestions.length > 0 && query.trim() === "" ? suggestions[0] : query);
            }
            if (event.key === "Escape") {
              setQuery("");
              setOpen(false);
            }
          }}
        />
        {open && suggestions.length > 0 && (
          <div className="tagfield__suggestions">
            {suggestions.map((tag) => (
              <button key={tag} className="row" onClick={() => commit(tag)}>
                <span className="row__label">#{tag}</span>
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
