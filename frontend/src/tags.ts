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

/** Цвет тега по имени: пока постоянный, из палитры на 14 значений (SPEC §8.2). */
export const tagPalette = 14;

export function tagColor(tag: string): number {
  let hash = 0;
  for (const char of tag) hash = (hash * 31 + char.codePointAt(0)!) % 100003;
  return hash % tagPalette;
}
