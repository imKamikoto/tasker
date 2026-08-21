/** Статусы заметки-задачи (SPEC §8.3). */
export const statuses = ["none", "active", "onHold", "completed", "dropped"] as const;

export type Status = (typeof statuses)[number];

/**
 * Глифы статусов. Взяты из макета: цвет один отличает статусы плохо — при
 * приглушённой палитре active и dropped на глаз почти одинаковы, а форма
 * различается сразу.
 */
export const statusGlyphs: Record<Status, string> = {
  none: "\u00b7",
  active: "\u25b6",
  onHold: "\u2016",
  completed: "\u2713",
  dropped: "\u2717",
};

/** Префикс команд смены статуса в раскладке клавиш. */
export const statusCommandPrefix = "note.status.";

/**
 * statusForCommand достаёт статус из имени команды вида note.status.onhold.
 *
 * Имена команд пишутся в keymap.json руками, поэтому они целиком в нижнем
 * регистре: заставлять человека помнить про заглавную H в onHold — верный
 * способ получить молча не работающую привязку.
 */
export function statusForCommand(command: string): Status | undefined {
  if (!command.startsWith(statusCommandPrefix)) return undefined;
  const name = command.slice(statusCommandPrefix.length);
  return statuses.find((status) => status.toLowerCase() === name);
}
