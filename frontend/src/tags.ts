/**
 * Подсказки для поля тегов.
 *
 * Уже проставленные не предлагаются, совпадение ищется подстрокой без учёта
 * регистра — как и в выборе ноутбука: набор небольшой, и нечёткий поиск здесь
 * мешает больше, чем помогает.
 */
export function suggestTags(all: string[], current: string[], query: string, limit = 8): string[] {
  const needle = query.trim().toLowerCase();
  const taken = new Set(current.map((tag) => tag.toLowerCase()));

  return all
    .filter((tag) => !taken.has(tag.toLowerCase()))
    .filter((tag) => needle === "" || tag.toLowerCase().includes(needle))
    .slice(0, limit);
}

/**
 * addTag добавляет тег к набору, если его там ещё нет.
 *
 * Возвращает тот же массив, когда добавлять нечего: так вызывающий может
 * отличить настоящее изменение от повтора и не дёргать запись зря.
 */
export function addTag(current: string[], tag: string): string[] {
  const clean = tag.trim();
  if (clean === "") return current;
  if (current.some((other) => other.toLowerCase() === clean.toLowerCase())) return current;
  return [...current, clean];
}

/** Сколько цветов в палитре (SPEC §8.2). Совпадает с TagPalette в Go. */
export const tagPalette = 14;

/** «Цвет не выбран»: тогда он выводится из имени. Совпадает с AutoColor в Go. */
export const autoColor = -1;

/**
 * tagColor возвращает цвет тега.
 *
 * Выбранный вручную побеждает; если его нет, цвет выводится из имени — так у
 * тега всегда есть постоянный оттенок, даже когда его никто не выбирал.
 */
export function tagColor(tag: string, chosen?: Record<string, number>): number {
  const picked = chosen?.[tag];
  if (picked !== undefined && picked >= 0 && picked < tagPalette) return picked;

  let hash = 0;
  for (const char of tag) hash = (hash * 31 + char.codePointAt(0)!) % 100003;
  return hash % tagPalette;
}
