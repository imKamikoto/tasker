/**
 * Что делать с Enter внутри списка.
 *
 * Логика вынесена из редактора отдельной чистой функцией: она вся в разборе
 * строки, и проверять её на живом CodeMirror значило бы поднимать браузер ради
 * регулярного выражения.
 */
export type ListAction =
  | { kind: "continue"; insert: string }
  | { kind: "clear" }
  | null;

/**
 * Строка списка: отступ, маркер, пробелы, необязательный чекбокс, содержимое.
 *
 * Нумерация принимает и «1.», и «1)» — GFM допускает оба.
 */
const listLine = /^(\s*)(?:([-*+])|(\d+)([.)]))(\s+)(\[[ xX]\]\s+)?(.*)$/;

/**
 * continueList решает, чем ответить на Enter в конце строки.
 *
 * Пустой пункт означает, что список кончился: строка очищается, и человек
 * продолжает обычным текстом. Так ведут себя все редакторы, и другого
 * способа выйти из списка, не удаляя маркер руками, нет.
 */
export function continueList(line: string): ListAction {
  const match = listLine.exec(line);
  if (!match) return null;

  const [, indent, bullet, number, delimiter, spacing, checkbox, content] = match;
  if (content.trim() === "") return { kind: "clear" };

  const marker = bullet ?? `${Number(number) + 1}${delimiter}`;
  // Новый чекбокс всегда пустой: продолжать список галочками «уже сделано»
  // не имеет смысла.
  return { kind: "continue", insert: `\n${indent}${marker}${spacing}${checkbox ? "[ ] " : ""}` };
}
