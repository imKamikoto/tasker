import { rampFor, type Pane } from "../focus";

type Props = {
  kind: Pane;
  /** Колонка держит клавиатурный фокус, а окно активно. */
  active: boolean;
};

/**
 * FocusRamp — индикатор фокуса колонки в титульной строке (FOCUS-STRIP.md).
 *
 * Живёт в верхних 11 пикселях полосы перетаскивания и высоты у содержимого не
 * отнимает: полоса там уже есть, рампа лежит поверх неё абсолютно.
 *
 * Ключ по состоянию — не украшение: анимация заливки запускается при монтаже,
 * и без смены ключа React переиспользовал бы узел, а анимация проигралась бы
 * ровно один раз за всю жизнь окна.
 */
export function FocusRamp({ kind, active }: Props) {
  return (
    <div className="ramp" aria-hidden="true">
      <span key={active ? "on" : "off"} className="ramp__marks" data-active={active}>
        {rampFor(kind, active)}
      </span>
    </div>
  );
}
