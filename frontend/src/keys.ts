// С расширением, а не без: этот модуль прогоняется и сборщиком, и node в
// тестах, а node без расширения соседний файл не найдёт.
import { latinKey } from "./langmap.ts";

/**
 * Разбор нажатия в строку вида «cmd+shift+k».
 *
 * Порядок модификаторов зафиксирован, регистр опущен: иначе одно и то же
 * сочетание пришлось бы писать в keymap.json несколькими способами, и человек
 * гадал бы, какой из них работает.
 */
const modifierOrder = ["cmd", "ctrl", "alt", "shift"] as const;

/** Имена клавиш, у которых event.key неудобен для файла настроек. */
const keyNames: Record<string, string> = {
  " ": "space",
  Enter: "enter",
  Escape: "escape",
  Backspace: "backspace",
  Delete: "delete",
  Tab: "tab",
  ArrowUp: "up",
  ArrowDown: "down",
  ArrowLeft: "left",
  ArrowRight: "right",
};

export type KeyEventLike = {
  key: string;
  metaKey: boolean;
  ctrlKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
};

export function combination(event: KeyEventLike): string {
  // Раскладка приводится к латинской по физической позиции клавиши. Без этого
  // при включённом ЙЦУКЕН не работает вообще ни одно буквенное сочетание:
  // Cmd+, приходит как Cmd+б, Cmd+N — как Cmd+т, а j и k в списке — как о и л.
  // Переключать раскладку ради шотката человек не должен, поэтому здесь то же
  // правило, что у вима, и та же таблица.
  const named = keyNames[event.key];
  const translated = named ?? latinKey(event.key);
  const key = named ?? translated.toLowerCase();

  const parts: string[] = [];
  for (const modifier of modifierOrder) {
    const pressed =
      (modifier === "cmd" && event.metaKey) ||
      (modifier === "ctrl" && event.ctrlKey) ||
      (modifier === "alt" && event.altKey) ||
      (modifier === "shift" && event.shiftKey && shiftMatters(translated));
    if (pressed) parts.push(modifier);
  }
  parts.push(key);
  return parts.join("+");
}

/**
 * shiftMatters решает, писать ли shift в сочетание.
 *
 * У букв и именованных клавиш пишем: Ctrl+Shift+H и Ctrl+H — разные нажатия,
 * и без этого второе съедало бы первое (буква всё равно приводится к нижнему
 * регистру, так что отличить их было бы нечем).
 *
 * У остальных символов не пишем: shift там уже поменял сам символ. Cmd+Shift+/
 * приходит как «?», и записать это ещё и с shift значит развести одно нажатие
 * на две разные записи в keymap.json.
 *
 * Проверяется переведённый символ, а не исходный: русская «Б» — буква, но
 * стоит на физической запятой и переводится в «<», где shift уже учтён.
 */
function shiftMatters(key: string): boolean {
  if (key.length > 1) return true;
  return key.toLowerCase() !== key.toUpperCase();
}

export type Keymap = Record<string, Record<string, string>>;

/**
 * resolveCommand ищет команду по нажатию, перебирая контексты по порядку.
 *
 * Контексты идут от частного к общему: «note-list», потом «global». Так клавиша
 * j принадлежит списку, но не мешает виму — в тексте контекст «editor» пуст, и
 * до глобального j просто не дойдёт.
 */
export function resolveCommand(
  keymap: Keymap,
  contexts: string[],
  event: KeyEventLike,
): string | undefined {
  const combo = combination(event);
  for (const context of contexts) {
    const command = keymap[context]?.[combo];
    if (command) return command;
  }
  return undefined;
}
