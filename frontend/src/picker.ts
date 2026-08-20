/**
 * Отбор строк для выбора ноутбука по набранному.
 *
 * Не нечёткий поиск: подстрока без учёта регистра. Ноутбуков десятки, а не
 * тысячи, и нечёткое совпадение здесь чаще мешает — «Раб» должно давать
 * «Работа», а не «Разбор/Библиотека».
 */
export function filterPaths(paths: string[], query: string): string[] {
  const needle = query.trim().toLowerCase();
  if (needle === "") return paths;
  return paths.filter((path) => path.toLowerCase().includes(needle));
}

/** Двигает выбор по списку, останавливаясь на краях. */
export function movePick(count: number, current: number, direction: 1 | -1): number {
  if (count === 0) return 0;
  return Math.min(count - 1, Math.max(0, current + direction));
}
