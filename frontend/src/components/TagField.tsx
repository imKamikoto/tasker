import { useState } from "react";

import { addTag, suggestTags, tagColor } from "../tags";

type Props = {
  tags: string[];
  known: string[];
  onChange: (tags: string[]) => void;
};

/**
 * TagField — теги заметки под заголовком (SPEC §8.2).
 *
 * Набор правится целиком: добавили или убрали — наружу уходит новый список, а
 * не разница. Вычислять разницу должен тот, кто пишет файл, а не поле ввода.
 */
export function TagField({ tags, known, onChange }: Props) {
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);

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
        <span key={tag} className="chip" data-color={tagColor(tag)}>
          #{tag}
          <button
            className="chip__remove"
            aria-label={`убрать ${tag}`}
            onClick={() => onChange(tags.filter((other) => other !== tag))}
          >
            ×
          </button>
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
