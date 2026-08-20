/**
 * Какие строки списка сейчас надо нарисовать.
 *
 * Строки одной высоты — тогда окно считается арифметикой, без измерения DOM.
 * Ради этого превью в строке жёстко обрезано двумя строками, а мета уложена в
 * одну: список из десяти тысяч заметок должен скроллиться без лагов (SPEC §8.4),
 * и платить за это измерением каждой строки — не вариант.
 */
export type Window = {
  /** Индекс первой отрисовываемой строки. */
  first: number;
  /** Индекс за последней отрисовываемой строкой. */
  end: number;
  /** Высота распорки сверху и снизу, чтобы полоса прокрутки не врала. */
  padTop: number;
  padBottom: number;
};

export type WindowInput = {
  total: number;
  rowHeight: number;
  viewport: number;
  scrollTop: number;
  /** Сколько строк рисовать про запас за краями окна. */
  overscan: number;
};

export function visibleWindow({
  total,
  rowHeight,
  viewport,
  scrollTop,
  overscan,
}: WindowInput): Window {
  if (total <= 0 || rowHeight <= 0) {
    return { first: 0, end: 0, padTop: 0, padBottom: 0 };
  }

  // Прокрутка может уехать за пределы списка — например, когда список
  // укоротился, а позиция осталась прежней.
  const top = Math.max(0, Math.min(scrollTop, total * rowHeight));

  const first = Math.max(0, Math.floor(top / rowHeight) - overscan);
  const visible = Math.ceil(viewport / rowHeight) + overscan * 2 + 1;
  const end = Math.min(total, first + visible);

  return {
    first,
    end,
    padTop: first * rowHeight,
    padBottom: (total - end) * rowHeight,
  };
}
