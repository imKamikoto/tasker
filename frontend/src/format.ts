/** Мелкие преобразования для показа. Чистые, чтобы их можно было прогнать тестом. */

/**
 * noteCount склоняет «заметку» по числу.
 *
 * По-русски «12 заметка» читается как сбой программы, а не как число, поэтому
 * склонение здесь не украшательство.
 */
export function noteCount(n: number): string {
  const tens = n % 100;
  const ones = n % 10;
  if (tens >= 11 && tens <= 14) return `${n} заметок`;
  if (ones === 1) return `${n} заметка`;
  if (ones >= 2 && ones <= 4) return `${n} заметки`;
  return `${n} заметок`;
}

/**
 * shortDate печатает дату так, как её показал бы Finder: сегодняшнее — временем,
 * всё остальное — днём и месяцем.
 *
 * now передаётся снаружи, чтобы «сегодня» можно было проверить тестом, а не
 * ждать полуночи.
 */
export function shortDate(value: string, now = new Date()): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";

  const sameDay =
    date.getFullYear() === now.getFullYear() &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate();

  return sameDay
    ? date.toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit" })
    : date.toLocaleDateString("ru-RU", { day: "numeric", month: "short" });
}

/**
 * noteCountTarget — то же число в винительном падеже: «перенести 1 заметку».
 *
 * Отдельная функция, а не флаг: у именительного и винительного совпадают все
 * формы, кроме единственного числа, и склеивать их в одну ветвистую функцию
 * ради одного различия — верный способ однажды перепутать.
 */
export function noteCountTarget(n: number): string {
  const tens = n % 100;
  const ones = n % 10;
  if (tens >= 11 && tens <= 14) return `${n} заметок`;
  if (ones === 1) return `${n} заметку`;
  if (ones >= 2 && ones <= 4) return `${n} заметки`;
  return `${n} заметок`;
}

/**
 * fileSize печатает байты так, как их читают глазами.
 *
 * Одна цифра после запятой у мелких значений в крупных единицах и ни одной у
 * остальных: «32 МБ» — это ответ на вопрос «много ли», а «32.4718 МБ» — уже нет.
 */
export function fileSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "—";

  const units = ["Б", "КБ", "МБ", "ГБ", "ТБ"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  // Байты дробными не бывают, а мелкие значения в крупных единицах без
  // десятой доли схлопываются в неразличимые «1 МБ».
  const rounded = unit > 0 && value < 10 ? value.toFixed(1) : String(Math.round(value));
  return `${rounded} ${units[unit]}`;
}

/** backlinkCount склоняет «бэклинк» — та же причина, что и у noteCount. */
export function backlinkCount(n: number): string {
  const tens = n % 100;
  const ones = n % 10;
  if (tens >= 11 && tens <= 14) return `${n} бэклинков`;
  if (ones === 1) return `${n} бэклинк`;
  if (ones >= 2 && ones <= 4) return `${n} бэклинка`;
  return `${n} бэклинков`;
}
