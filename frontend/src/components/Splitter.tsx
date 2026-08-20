import { useCallback, useEffect, useRef, useState } from "react";

type Props = {
  width: number;
  min: number;
  max: number;
  onChange: (width: number) => void;
};

/**
 * Splitter — разделитель колонок, который тянется мышью (SPEC §7).
 *
 * Ширина хранится у родителя: колонку рисует он, а разделитель только сообщает
 * новое число.
 */
export function Splitter({ width, min, max, onChange }: Props) {
  const [dragging, setDragging] = useState(false);
  // Стартовые значения держим в ref: они нужны обработчику мыши, но перерисовки
  // от них не зависят.
  const start = useRef({ x: 0, width: 0 });

  const onMouseDown = useCallback(
    (event: React.MouseEvent) => {
      event.preventDefault();
      start.current = { x: event.clientX, width };
      setDragging(true);
    },
    [width],
  );

  useEffect(() => {
    if (!dragging) return;

    const onMove = (event: MouseEvent) => {
      const next = start.current.width + (event.clientX - start.current.x);
      onChange(Math.min(max, Math.max(min, next)));
    };
    const onUp = () => setDragging(false);

    // Слушаем окно, а не сам разделитель: курсор во время перетаскивания
    // уезжает с него, и события пошли бы мимо.
    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
    return () => {
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mouseup", onUp);
    };
  }, [dragging, min, max, onChange]);

  return (
    <div
      className="splitter"
      role="separator"
      aria-orientation="vertical"
      data-dragging={dragging}
      onMouseDown={onMouseDown}
    />
  );
}
