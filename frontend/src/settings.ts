/** Что помнит интерфейс между запусками. Хранится в .tasker/config.json. */
export type UISettings = {
  sidebarWidth: number;
  listWidth: number;
  sortField: SortField;
  sortReversed: boolean;
  /** Пути свёрнутых ноутбуков. Свёрнутых, а не развёрнутых: новые ветки
   *  должны появляться открытыми, а не прятаться. */
  collapsed: string[];

  /** Оформление: по системе, светлое или тёмное. */
  theme: Theme;
  /** Акцент — единственный цвет интерфейса, всё остальное серое. */
  accent: Accent;
  /** Оттенок своего акцента, 0–359. Работает только при accent = "custom". */
  accentHue: number;

  /** Насколько панели пропускают обои, 0–100. */
  transparency: number;
  /** Размытие того, что просвечивает, 0–100. */
  blur: number;
  /** Отключать прозрачность, когда машина на батарее. */
  opaqueOnBattery: boolean;
  /** Дизер-текстура на панелях. */
  dither: boolean;

  /** Кегль тела заметки в пикселях. */
  fontSize: number;
  /** Интерлиньяж как множитель кегля. */
  lineHeight: number;
  /** Номера строк в редакторе. */
  lineNumbers: boolean;
  /** Переносить длинные строки. */
  lineWrap: boolean;
  /** Сколько ждать после последней правки перед записью, мс. */
  saveDelay: number;

  /**
   * Вим-режим в редакторе.
   *
   * Включён по умолчанию и остаётся тем, ради чего приложение написано.
   * Выключатель существует ради второго человека: тому, кто вима не знает,
   * редактор без него не «хуже настроенный», а просто неработающий — набор
   * текста в нормальном режиме не печатает ничего.
   */
  vim: boolean;
  /**
   * Движения вима в списке и сайдбаре: j, k, h, l.
   *
   * Отдельно от редактора, потому что это разные вещи: вим в тексте можно
   * любить, а буквы вместо стрелок в списке — нет, и наоборот. Стрелки,
   * Enter и остальные команды работают в любом случае.
   */
  vimNavigation: boolean;

  /** Окно автокоммита в секундах. Ноль — коммитить каждое сохранение. */
  commitWindow: number;

  /** Показывать метку у заметок, заведённых агентом. */
  agentBadge: boolean;

  /** Сайдбар свёрнут: колонка ноутбуков и тегов не показывается. */
  sidebarHidden: boolean;
  /**
   * Свёрнутые секции сайдбара целиком.
   *
   * Отдельно от collapsed: там пути свёрнутых ноутбуков, а это две секции с
   * постоянными именами. Складывать сентинелы в список путей — верный способ
   * однажды получить ноутбук с именем «tags».
   */
  notebooksCollapsed: boolean;
  tagsCollapsed: boolean;
  /**
   * Порядок и видимость верхних пунктов сайдбара.
   *
   * Списком имён, а не числами: имя переживает добавление нового пункта, а
   * позиция — нет. Незнакомое имя и недостающий пункт разбираются в toprows.ts,
   * а не здесь: испорченный файл не должен прятать пункты навсегда.
   */
  topOrder: string[];
  topHidden: string[];
  /**
   * Масштаб текста. Единица — исходный размер.
   *
   * Растёт только кегль: колонки сохраняют ширину, и на экране остаётся
   * столько же колонок, просто читать легче. Не зум вебвью — тот увеличивает
   * всё разом, включая ширины, и в окно попросту влезает меньше.
   */
  textScale: number;
};

export const sortFields = ["updated", "created", "title"] as const;
export type SortField = (typeof sortFields)[number];

export const themes = ["auto", "light", "dark"] as const;
export type Theme = (typeof themes)[number];

/** Готовые акценты плюс «свой». Порядок — как в макете настроек. */
export const accents = [
  "graphite",
  "steel",
  "sage",
  "lavender",
  "khaki",
  "clay",
  "custom",
] as const;
export type Accent = (typeof accents)[number];

/** Человеческие имена акцентов. */
export const accentNames: Record<Accent, string> = {
  graphite: "Графит",
  steel: "Сталь",
  sage: "Шалфей",
  lavender: "Лаванда",
  khaki: "Хаки",
  clay: "Глина",
  custom: "Свой",
};

/**
 * Границы числовых настроек: минимум, максимум, умолчание.
 *
 * Таблицей, а не россыпью литералов по разбору, потому что те же границы нужны
 * ползункам в настройках — иначе интерфейс позволит выставить значение, которое
 * разбор потом молча зажмёт, и настройка «не сохранится».
 */
export const limits = {
  sidebarWidth: { min: 180, max: 320, step: 1 },
  listWidth: { min: 280, max: 480, step: 1 },
  accentHue: { min: 0, max: 359, step: 1 },
  transparency: { min: 0, max: 100, step: 1 },
  blur: { min: 0, max: 100, step: 1 },
  fontSize: { min: 11, max: 20, step: 1 },
  lineHeight: { min: 1.2, max: 2.2, step: 0.05 },
  saveDelay: { min: 100, max: 5000, step: 100 },
  // Потолок совпадает с maxCommitWindow в internal/app/git.go.
  commitWindow: { min: 0, max: 1800, step: 15 },
  // Ниже 0.8 метаполосы нечитаемы, выше 1.6 в колонку по умолчанию не
  // влезает ни строчки — ширину придётся тянуть руками.
  textScale: { min: 0.8, max: 1.6, step: 0.1 },
} as const;

export const defaultSettings: UISettings = {
  sidebarWidth: 216,
  listWidth: 320,
  sortField: "updated",
  sortReversed: false,
  collapsed: [],

  // Тёмная по умолчанию, а не «по системе»: приложение открыто часами,
  // и белое окно на весь экран — не то, чего ждут от него по умолчанию.
  theme: "dark",
  accent: "steel",
  accentHue: 210,

  // Прозрачность выключена: она красива на подобранных обоях и мешает на
  // случайных, поэтому включает её человек, а не умолчание.
  transparency: 0,
  blur: 60,
  opaqueOnBattery: true,
  dither: false,

  fontSize: 13,
  lineHeight: 1.7,
  lineNumbers: true,
  lineWrap: true,
  saveDelay: 400,

  // Оба включены: вим — умолчание проекта, а не опция, которую предлагают.
  vim: true,
  vimNavigation: true,

  // Ноль — коммит на каждое сохранение, как работало всегда.
  commitWindow: 0,

  agentBadge: true,

  sidebarHidden: false,
  notebooksCollapsed: false,
  tagsCollapsed: false,
  // Пустой порядок означает «как было»: toprows.ts дописывает недостающее.
  topOrder: [],
  topHidden: [],
  textScale: 1,
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
  const num = (key: keyof typeof limits, fallback: number) =>
    clamp(source[key], fallback, limits[key].min, limits[key].max, limits[key].step);
  const strings = (value: unknown): string[] =>
    Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
  const flag = (key: keyof UISettings) =>
    typeof source[key] === "boolean" ? (source[key] as boolean) : (defaultSettings[key] as boolean);

  return {
    sidebarWidth: num("sidebarWidth", defaultSettings.sidebarWidth),
    listWidth: num("listWidth", defaultSettings.listWidth),
    sortField: sortFields.includes(source.sortField as SortField)
      ? (source.sortField as SortField)
      : defaultSettings.sortField,
    sortReversed: source.sortReversed === true,
    collapsed: strings(source.collapsed),

    theme: themes.includes(source.theme as Theme)
      ? (source.theme as Theme)
      : defaultSettings.theme,
    accent: accents.includes(source.accent as Accent)
      ? (source.accent as Accent)
      : defaultSettings.accent,
    accentHue: num("accentHue", defaultSettings.accentHue),

    transparency: num("transparency", defaultSettings.transparency),
    blur: num("blur", defaultSettings.blur),
    opaqueOnBattery: flag("opaqueOnBattery"),
    dither: flag("dither"),

    fontSize: num("fontSize", defaultSettings.fontSize),
    lineHeight: num("lineHeight", defaultSettings.lineHeight),
    lineNumbers: flag("lineNumbers"),
    lineWrap: flag("lineWrap"),
    saveDelay: num("saveDelay", defaultSettings.saveDelay),

    vim: flag("vim"),
    vimNavigation: flag("vimNavigation"),

    commitWindow: num("commitWindow", defaultSettings.commitWindow),

    agentBadge: flag("agentBadge"),

    sidebarHidden: flag("sidebarHidden"),
    notebooksCollapsed: flag("notebooksCollapsed"),
    tagsCollapsed: flag("tagsCollapsed"),
    topOrder: strings(source.topOrder),
    topHidden: strings(source.topHidden),
    textScale: num("textScale", defaultSettings.textScale),
  };
}

/**
 * nextZoom считает следующий масштаб по команде клавиатуры.
 *
 * Шаг и границы берутся из тех же limits, что и ползунок в настройках: иначе
 * клавиатура доедет туда, куда ползунок не пускает, и значение молча зажмётся
 * при записи — со стороны это выглядит как «настройка не сохранилась».
 */
export function nextZoom(current: number, command: string): number {
  if (command === "view.zoom.reset") return defaultSettings.textScale;

  const step = command === "view.zoom.in" ? limits.textScale.step : -limits.textScale.step;
  const next = Math.min(
    limits.textScale.max,
    Math.max(limits.textScale.min, current + step),
  );
  // Арифметика с дробным шагом даёт 1.2000000000000002 — в файл этому не место.
  return Number(next.toFixed(2));
}

/**
 * clamp зажимает число в границы и притягивает к шагу.
 *
 * Притягивание к шагу нужно ради дробных настроек: интерлиньяж 1.7000000000002
 * от арифметики ползунка попадёт в файл ровно в таком виде и будет там жить.
 */
function clamp(value: unknown, fallback: number, min: number, max: number, step: number): number {
  if (typeof value !== "number" || !Number.isFinite(value)) return fallback;
  const bounded = Math.min(max, Math.max(min, value));
  const snapped = Math.round(bounded / step) * step;
  // Одна цифра после запятой хватает всем настройкам, где шаг дробный.
  return Number(snapped.toFixed(2));
}
