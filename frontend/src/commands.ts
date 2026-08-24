/**
 * Каталог команд для экрана шоткатов.
 *
 * Список живёт здесь, а не выводится из keymap.json: файл описывает, что на что
 * назначено, и снятая привязка стёрла бы команду из настроек вместе с
 * возможностью назначить её заново.
 *
 * Контексты и имена команд обязаны совпадать с defaultKeymap в
 * internal/app/keymap.go — на это есть тест.
 */
export type CommandContext = "global" | "sidebar" | "note-list" | "editor";

export type CommandInfo = {
  id: string;
  context: CommandContext;
  label: string;
};

export const contextNames: Record<CommandContext, string> = {
  global: "Везде",
  sidebar: "В сайдбаре",
  "note-list": "В списке",
  editor: "В тексте",
};

export const contextHints: Record<CommandContext, string> = {
  global: "Работают откуда угодно, в том числе из текста заметки.",
  sidebar: "Работают, когда фокус на ноутбуках и тегах.",
  "note-list": "Работают, когда фокус на списке заметок.",
  editor: "Пусто намеренно: в тексте всё принадлежит редактору и виму. Сюда можно добавить своё.",
};

export const commands: CommandInfo[] = [
  { id: "note.create", context: "global", label: "Новая заметка" },
  { id: "note.status.none", context: "global", label: "Статус: без статуса" },
  { id: "note.status.active", context: "global", label: "Статус: в работе" },
  { id: "note.status.onhold", context: "global", label: "Статус: отложена" },
  { id: "note.status.completed", context: "global", label: "Статус: завершена" },
  { id: "note.status.dropped", context: "global", label: "Статус: брошена" },
  { id: "note.settings", context: "global", label: "Настройки" },
  { id: "note.template", context: "global", label: "Применить шаблон" },
  { id: "focus.prev", context: "global", label: "Фокус на колонку левее" },
  { id: "focus.next", context: "global", label: "Фокус на колонку правее" },
  { id: "view.sidebar", context: "global", label: "Скрыть и показать сайдбар" },
  { id: "view.zoom.in", context: "global", label: "Масштаб больше" },
  { id: "view.zoom.out", context: "global", label: "Масштаб меньше" },
  { id: "view.zoom.reset", context: "global", label: "Масштаб как был" },

  { id: "sidebar.down", context: "sidebar", label: "Ниже по сайдбару" },
  { id: "sidebar.up", context: "sidebar", label: "Выше по сайдбару" },
  { id: "sidebar.open", context: "sidebar", label: "Открыть выбранное" },
  { id: "sidebar.expand", context: "sidebar", label: "Развернуть ветку" },
  { id: "sidebar.collapse", context: "sidebar", label: "Свернуть ветку" },

  { id: "list.down", context: "note-list", label: "Ниже по списку" },
  { id: "list.up", context: "note-list", label: "Выше по списку" },
  { id: "list.open", context: "note-list", label: "Перейти в текст" },
  { id: "note.pin", context: "note-list", label: "Закрепить и открепить" },
  { id: "note.move", context: "note-list", label: "Перенести в ноутбук" },
  { id: "note.duplicate", context: "note-list", label: "Дублировать" },
  { id: "note.trash", context: "note-list", label: "В корзину" },
];

/**
 * Клавиши, которые в списке заняты вимом, если их нажать в тексте.
 *
 * Список неполный намеренно: сюда попали только однобуквенные команды
 * нормального режима, которые человек реально попробует назначить. Проверка
 * предупреждает, а не запрещает — назначение в контексте списка вим не ломает
 * (SPEC §8.6, CLAUDE.md), а вот в контексте редактора ломает.
 */
const vimKeys = new Set([
  "h", "j", "k", "l", "w", "b", "e", "0", "$", "g", "i", "a", "o", "x", "d", "c",
  "y", "p", "u", "v", "n", "f", "t", "r", "s", "m", "q", "z", ".", "/", ":", "escape",
]);

/** Что не так с сочетанием. */
export type Conflict =
  | { kind: "none" }
  /** Сочетание уже занято другой командой в том же контексте. */
  | { kind: "taken"; command: string }
  /** Однобуквенная привязка в контексте редактора отнимет клавишу у вима. */
  | { kind: "vim"; key: string };

/**
 * checkBinding проверяет, можно ли назначить сочетание.
 *
 * Занятость проверяется только внутри контекста: одна и та же клавиша в списке
 * и в тексте — это норма, ради того контексты и заведены.
 */
export function checkBinding(
  bindings: Record<string, string>,
  context: CommandContext,
  combo: string,
  command: string,
): Conflict {
  const taken = bindings[combo];
  if (taken && taken !== command) return { kind: "taken", command: taken };

  // Модификатор снимает спор с вимом: cmd+j вим не использует.
  if (context === "editor" && !combo.includes("+") && vimKeys.has(combo)) {
    return { kind: "vim", key: combo };
  }
  return { kind: "none" };
}

/** Печатает сочетание так, как его пишут на клавишах мака. */
export function prettyCombo(combo: string): string {
  if (combo === "") return "—";
  const glyphs: Record<string, string> = {
    cmd: "⌘",
    ctrl: "⌃",
    alt: "⌥",
    shift: "⇧",
    enter: "⏎",
    escape: "esc",
    backspace: "⌫",
    delete: "⌦",
    tab: "⇥",
    space: "␣",
    up: "↑",
    down: "↓",
    left: "←",
    right: "→",
  };
  const parts = combo.split("+");
  return parts
    .map((part, i) => {
      const glyph = glyphs[part];
      if (glyph) return glyph;
      // Последний кусок — сама клавиша; буквы показываем заглавными.
      return i === parts.length - 1 ? part.toUpperCase() : part;
    })
    .join("");
}

/** Все сочетания контекста, назначенные на команду. */
export function bindingsFor(
  keymap: Record<string, Record<string, string>>,
  context: CommandContext,
  command: string,
): string[] {
  const bindings = keymap[context] ?? {};
  return Object.keys(bindings)
    .filter((combo) => bindings[combo] === command)
    .sort();
}
