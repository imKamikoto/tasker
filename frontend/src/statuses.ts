/** Статусы в порядке шоткатов Cmd+Ctrl+1..5 (SPEC §8.3). */
export const statuses = ["none", "active", "onHold", "completed", "dropped"] as const;

export type Status = (typeof statuses)[number];

/** Скрыт ли статус из списка по умолчанию. */
export function isDone(status: string): boolean {
  return status === "completed" || status === "dropped";
}

/**
 * statusForKey сопоставляет цифру со статусом.
 *
 * Возвращает undefined для всего остального: обработчик не должен угадывать.
 */
export function statusForKey(key: string): Status | undefined {
  const index = Number(key) - 1;
  return Number.isInteger(index) ? statuses[index] : undefined;
}
