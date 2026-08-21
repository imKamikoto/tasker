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
  const key = keyNames[event.key] ?? event.key.toLowerCase();

  const parts: string[] = [];
  for (const modifier of modifierOrder) {
    const pressed =
      (modifier === "cmd" && event.metaKey) ||
      (modifier === "ctrl" && event.ctrlKey) ||
      (modifier === "alt" && event.altKey) ||
      // Shift не пишем для символов: cmd+shift+/ и cmd+? — одно нажатие, и
      // различать их в файле настроек значит ловить человека на мелочи.
      (modifier === "shift" && event.shiftKey && key.length > 1);
    if (pressed) parts.push(modifier);
  }
  parts.push(key);
  return parts.join("+");
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
