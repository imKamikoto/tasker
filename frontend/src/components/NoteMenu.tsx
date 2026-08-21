import { useEffect, useRef } from "react";

import { statusGlyphs, statuses, type Status } from "../statuses";

type Props = {
  /** Куда ставить меню: координаты курсора в окне. */
  at: { x: number; y: number };
  /** Статус заметки, по которой щёлкнули — он отмечен галочкой. */
  status: string;
  pinned: boolean;
  onStatus: (status: Status) => void;
  onPin: () => void;
  onMove: () => void;
  onDuplicate: () => void;
  onTrash: () => void;
  onClose: () => void;
};

/** Шоткаты статусов. Индекс совпадает с порядком в statuses. */
const statusKeys = ["⌘⌃1", "⌘⌃2", "⌘⌃3", "⌘⌃4", "⌘⌃5"];

/**
 * NoteMenu — правый клик по строке списка.
 *
 * Всё, что здесь есть, доступно и с клавиатуры. Меню нужно не вместо шоткатов,
 * а чтобы их можно было найти, не заглядывая в keymap.json.
 */
export function NoteMenu({
  at,
  status,
  pinned,
  onStatus,
  onPin,
  onMove,
  onDuplicate,
  onTrash,
  onClose,
}: Props) {
  const menu = useRef<HTMLDivElement | null>(null);

  // Закрывается по щелчку где угодно и по Escape. Слушаем окно в фазе
  // погружения: иначе щелчок по кнопке внутри меню закроет его раньше, чем
  // сработает сама кнопка.
  useEffect(() => {
    const onDown = (event: MouseEvent) => {
      if (!menu.current?.contains(event.target as Node)) onClose();
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("mousedown", onDown);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("mousedown", onDown);
      window.removeEventListener("keydown", onKey);
    };
  }, [onClose]);

  // Меню не должно вылезать за окно: у нижних строк списка места вниз нет.
  const height = 250;
  const top = Math.min(at.y, Math.max(8, window.innerHeight - height));
  const left = Math.min(at.x, Math.max(8, window.innerWidth - 200));

  const pick = (run: () => void) => () => {
    onClose();
    run();
  };

  return (
    <div ref={menu} className="menu menu--context" style={{ top, left }}>
      <button className="menu__item" onClick={pick(onPin)}>
        <span className="menu__glyph">{pinned ? "☆" : "★"}</span>
        <span className="menu__label">{pinned ? "Открепить" : "Закрепить"}</span>
        <span className="menu__key">p</span>
      </button>
      <button className="menu__item" onClick={pick(onMove)}>
        <span className="menu__glyph">→</span>
        <span className="menu__label">Перенести…</span>
        <span className="menu__key">m</span>
      </button>
      <button className="menu__item" onClick={pick(onDuplicate)}>
        <span className="menu__glyph">⧉</span>
        <span className="menu__label">Дублировать</span>
        <span className="menu__key">⌘D</span>
      </button>

      <div className="section" style={{ padding: "8px 8px 4px" }}>
        <span className="section__label">Статус</span>
        <span className="section__rule" />
      </div>
      {statuses.map((value, i) => (
        <button
          key={value}
          className="menu__item"
          aria-selected={value === status}
          onClick={pick(() => onStatus(value))}
        >
          <span className="menu__glyph" data-status={value}>
            {statusGlyphs[value]}
          </span>
          <span className="menu__label">{value}</span>
          {value === status && <span className="menu__check">✓</span>}
          <span className="menu__key">{statusKeys[i]}</span>
        </button>
      ))}

      <button className="menu__item menu__item--danger" onClick={pick(onTrash)}>
        <span className="menu__glyph">▚</span>
        <span className="menu__label">В корзину</span>
        <span className="menu__key">⌘⌫</span>
      </button>
    </div>
  );
}
