/**
 * Правила выделения в списке (SPEC §8.4).
 *
 * Вынесено отдельно, потому что вариантов немного, но каждый легко перепутать:
 * обычный щелчок, Cmd+щелчок и Shift+щелчок ведут себя по-разному, и на глаз
 * это не проверяется.
 */
export type SelectionInput = {
  /** Порядок строк на экране: от него зависит, что попадёт в диапазон. */
  order: string[];
  selected: string[];
  /** Откуда тянется диапазон при Shift+щелчке. */
  anchor: string | null;
  clicked: string;
  toggle: boolean;
  range: boolean;
};

export type SelectionResult = {
  selected: string[];
  anchor: string;
};

export function applyClick(input: SelectionInput): SelectionResult {
  const { order, selected, anchor, clicked, toggle, range } = input;

  if (range && anchor !== null) {
    const from = order.indexOf(anchor);
    const to = order.indexOf(clicked);
    if (from >= 0 && to >= 0) {
      const [start, end] = from <= to ? [from, to] : [to, from];
      // Якорь не двигается: несколько Shift+щелчков подряд тянут диапазон от
      // одной и той же точки, а не от предыдущего щелчка.
      return { selected: order.slice(start, end + 1), anchor };
    }
  }

  if (toggle) {
    const without = selected.filter((id) => id !== clicked);
    const next = without.length === selected.length ? [...selected, clicked] : without;
    return { selected: next, anchor: clicked };
  }

  return { selected: [clicked], anchor: clicked };
}
