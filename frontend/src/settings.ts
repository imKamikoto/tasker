/** Что помнит интерфейс между запусками. Хранится в .tasker/config.json. */
export type UISettings = {
  sidebarWidth: number;
  listWidth: number;
  sortField: SortField;
  sortReversed: boolean;
  /** Пути свёрнутых ноутбуков. Свёрнутых, а не развёрнутых: новые ветки
   *  должны появляться открытыми, а не прятаться. */
  collapsed: string[];
};

export const sortFields = ["updated", "created", "title"] as const;
export type SortField = (typeof sortFields)[number];

export const defaultSettings: UISettings = {
  sidebarWidth: 216,
  listWidth: 320,
  sortField: "updated",
  sortReversed: false,
  collapsed: [],
};

/**
 * parseSettings приводит прочитанное к известному виду.
 *
 * Модуль намеренно не знает про биндинги: разбор — чистая функция, и её надо
 * уметь прогонять тестами без Wails. Читает и пишет файл api.ts.
 *
 * Файл правится руками и переживает смену версий, поэтому каждое поле
 * проверяется по отдельности: одна испорченная настройка не должна утаскивать
 * за собой остальные.
 */
export function parseSettings(raw: string): UISettings {
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return { ...defaultSettings };
  }
  if (typeof parsed !== "object" || parsed === null) return { ...defaultSettings };

  const source = parsed as Record<string, unknown>;
  return {
    sidebarWidth: width(source.sidebarWidth, defaultSettings.sidebarWidth, 160, 400),
    listWidth: width(source.listWidth, defaultSettings.listWidth, 240, 600),
    sortField: sortFields.includes(source.sortField as SortField)
      ? (source.sortField as SortField)
      : defaultSettings.sortField,
    sortReversed: source.sortReversed === true,
    collapsed: Array.isArray(source.collapsed)
      ? source.collapsed.filter((item): item is string => typeof item === "string")
      : [],
  };
}

function width(value: unknown, fallback: number, min: number, max: number): number {
  if (typeof value !== "number" || !Number.isFinite(value)) return fallback;
  return Math.min(max, Math.max(min, Math.round(value)));
}
