/** Ширина полосы прогресса в символах. Из макета: `▓▓▓░░░░ 3/7`. */
export const progressWidth = 7;

/**
 * progressBar рисует прогресс чеклиста символами, а не элементами.
 *
 * Символами — чтобы полоса стояла в одной моноширинной строке с датой и
 * статусом и не требовала ни своей высоты, ни выравнивания по базовой линии.
 *
 * Округление вниз, но с двумя оговорками: пока сделано не всё, последняя
 * клетка не закрашивается (иначе 6 из 7 выглядят как готово), а как только
 * сделано хоть что-то — закрашивается первая (иначе начатая работа выглядит
 * как неначатая).
 */
export function progressBar(done: number, total: number, width = progressWidth): string {
  if (total <= 0 || width <= 0) return "";

  const clamped = Math.min(Math.max(done, 0), total);
  let filled = Math.floor((clamped / total) * width);
  if (clamped > 0 && filled === 0) filled = 1;
  if (clamped < total && filled === width) filled = width - 1;

  return "▓".repeat(filled) + "░".repeat(width - filled);
}
