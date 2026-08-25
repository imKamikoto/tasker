import { rampFor, type Pane, type RampState } from "../focus";

type Props = {
  kind: Pane;
  state: RampState;
};

/**
 * FocusRamp — индикатор фокуса колонки в титульной строке (FOCUS-STRIP.md).
 *
 * Живёт на линии системных кнопок окна и высоты у содержимого не отнимает:
 * полоса перетаскивания там уже есть, рампа лежит поверх неё абсолютно.
 *
 * Состояние hidden убирает индикатор целиком, а не гасит его: он существует
 * ради слепого переключения колонок по ⌃⇧H и ⌃⇧L, и с выключенными
 * движениями вима переключать вслепую нечем.
 *
 * Ключ по состоянию — не украшение: анимация заливки запускается при монтаже,
 * и без смены ключа React переиспользовал бы узел, а анимация проигралась бы
 * ровно один раз за всю жизнь окна.
 */
export function FocusRamp({ kind, state }: Props) {
  if (state === "hidden") return null;

  const active = state === "active";
  return (
    <div className="ramp" aria-hidden="true">
      <span key={active ? "on" : "off"} className="ramp__marks" data-active={active}>
        {rampFor(kind, state)}
      </span>
    </div>
  );
}
