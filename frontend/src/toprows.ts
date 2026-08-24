/**
 * Верхние пункты сайдбара: «Активные», «Все заметки», «От агента».
 *
 * Порядок и видимость правит человек, поэтому и то и другое хранится в
 * настройках. Список того, что вообще бывает, — здесь: он закрытый, новых
 * пунктов в него добавляют кодом, а не данными.
 *
 * «Корзины» здесь нет намеренно: она прижата к низу колонки и в общий порядок
 * не входит — это не пункт того же ряда, а дно.
 */
export const topKinds = ["active", "all", "agent"] as const;
export type TopKind = (typeof topKinds)[number];

/** Подписи и глифы. Совпадают с тем, что было в сайдбаре до настройки. */
export const topLabels: Record<TopKind, string> = {
  active: "Активные",
  all: "Все заметки",
  agent: "От агента",
};

export const topGlyphs: Record<TopKind, string> = {
  active: "▸",
  all: "≡",
  agent: "◆",
};

/** У каких пунктов глиф красится акцентом — так было до настройки. */
export const topAccented: Record<TopKind, boolean> = {
  active: true,
  all: false,
  agent: true,
};

/**
 * topRows решает, что и в каком порядке показать.
 *
 * Порядок из настроек, но список закрытый: незнакомое имя выбрасывается, а
 * недостающее дописывается в конец. Иначе испорченный или устаревший
 * config.json прятал бы пункты навсегда, и вернуть их было бы нечем — сама
 * настройка живёт в том же файле.
 *
 * «От агента» показывается, только когда агент что-то написал: пустой раздел в
 * хранилище, куда агент не ходит, — просто шум. Спрятать его руками тоже
 * можно, но появиться сам он не должен.
 */
export function topRows(order: string[], hidden: string[], hasAgentNotes: boolean): TopKind[] {
  const known = new Set<string>(topKinds);
  const seen = new Set<string>();

  const ordered: TopKind[] = [];
  for (const item of order) {
    if (!known.has(item) || seen.has(item)) continue;
    seen.add(item);
    ordered.push(item as TopKind);
  }
  for (const item of topKinds) {
    if (!seen.has(item)) ordered.push(item);
  }

  const off = new Set(hidden);
  return ordered.filter((item) => {
    if (off.has(item)) return false;
    if (item === "agent") return hasAgentNotes;
    return true;
  });
}

/** move переставляет пункт на одну позицию. Края никуда не двигаются. */
export function move(order: TopKind[], kind: TopKind, direction: 1 | -1): TopKind[] {
  const at = order.indexOf(kind);
  const next = at + direction;
  if (at < 0 || next < 0 || next >= order.length) return order;

  const out = [...order];
  out[at] = out[next];
  out[next] = kind;
  return out;
}
