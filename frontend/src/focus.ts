/**
 * Какая из трёх колонок принимает клавиши.
 *
 * Раньше этого состояния не было вовсе: клавиши списка работали, когда каретка
 * не в тексте, а сайдбар с клавиатуры был недоступен. Три колонки — три места,
 * где может быть фокус, и переключаться между ними надо уметь, не трогая мышь.
 */
export const panes = ["sidebar", "list", "editor"] as const;

export type Pane = (typeof panes)[number];

/** Человеческие имена — для подсказок и подписей команд. */
export const paneNames: Record<Pane, string> = {
  sidebar: "ноутбуки и теги",
  list: "список заметок",
  editor: "текст заметки",
};

/**
 * movePane сдвигает фокус по кругу.
 *
 * По кругу, а не до упора: колонок три, и «дальше некуда» на краю означало бы,
 * что из редактора в сайдбар надо жать дважды в одну сторону и один раз в
 * другую — правило, которое никто не запомнит.
 */
export function movePane(current: Pane, direction: 1 | -1): Pane {
  const at = panes.indexOf(current);
  const next = (at + direction + panes.length) % panes.length;
  return panes[next];
}

/** Команды смены фокуса. Совпадают с каталогом и с умолчаниями в Go. */
export const focusCommands: Record<string, 1 | -1> = {
  "focus.next": 1,
  "focus.prev": -1,
};

/**
 * Сочетание, на которое редактор может рассчитывать при наборе.
 *
 * Одинокий Ctrl с буквой — территория текста: и вим, и CodeMirror держат там
 * десятки команд (`Ctrl+H` backspace, `Ctrl+K` до конца строки, `Ctrl+W` слово
 * назад и так далее), а `Ctrl+Alt+H` — удалить группу. Правилом, а не списком:
 * список пришлось бы сверять с двумя чужими раскладками на каждом обновлении.
 *
 * Cmd сюда не попадает: на macOS сочетания с ним принадлежат приложению.
 * Shift тоже снимает вопрос — ни вим, ни CodeMirror Ctrl+Shift+буква не
 * занимают.
 */
function editorTerritory(combo: string): boolean {
  return combo.startsWith("ctrl+") && !combo.includes("shift+") && !combo.includes("cmd+");
}

/**
 * stealsFromEditor решает, можно ли перехватить нажатие у редактора.
 *
 * Смена фокуса обязана работать прямо из текста, иначе выйти из него можно
 * только мышью. Но во время набора отбирать у текста его же клавиши нельзя:
 * человек ждёт, что Ctrl+H сотрёт символ, а не уедет в другую колонку.
 *
 * Поэтому запрет узкий: только на территории редактора и только в режимах,
 * где набирают. В нормальном и визуальном перехват безопасен всегда — там эти
 * команды всё равно не нужны.
 */
export function stealsFromEditor(combo: string, vimMode: string): boolean {
  if (!editorTerritory(combo)) return true;

  const mode = vimMode.toUpperCase();
  return mode !== "INSERT" && mode !== "REPLACE";
}
