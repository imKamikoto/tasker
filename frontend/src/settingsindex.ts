/**
 * Что где лежит в настройках — для поиска по ним.
 *
 * Отдельным списком, а не обходом разметки: разметка — это JSX, и вытаскивать
 * из неё подписи значит разбирать компоненты в рантайме. Цена решения — второй
 * список, который обязан совпадать с первым; отсюда и тест, проверяющий, что
 * каждый раздел в нём представлен.
 *
 * `hints` — слова, которых нет в подписи, но по которым настройку ищут:
 * человек помнит «тёмная», а пункт называется «Оформление».
 */
export type SettingEntry = {
  section: string;
  label: string;
  hints?: string;
};

export const settingsIndex: SettingEntry[] = [
  { section: "appearance", label: "Оформление", hints: "тема тёмная светлая система" },
  { section: "appearance", label: "Акцент", hints: "цвет оттенок графит сталь шалфей" },
  { section: "appearance", label: "Прозрачность", hints: "обои размытие блюр" },
  { section: "appearance", label: "Размытие", hints: "блюр прозрачность" },
  { section: "appearance", label: "Экономия на батарее", hints: "питание аккумулятор" },
  { section: "appearance", label: "Дизер", hints: "текстура шум" },
  { section: "appearance", label: "Масштаб текста", hints: "кегль размер интерфейс" },
  { section: "appearance", label: "Метка заметок агента", hints: "агент mcp бейдж" },

  { section: "storage", label: "Папка с заметками", hints: "хранилище vault путь сменить" },
  { section: "storage", label: "Недавние папки", hints: "хранилище vault" },
  { section: "storage", label: "Пересобрать индекс", hints: "sqlite поиск сверка" },
  { section: "storage", label: "История", hints: "git версии коммиты репозиторий" },
  { section: "storage", label: "Окно автокоммита", hints: "git коммит пачка" },

  { section: "editor", label: "Кегль", hints: "размер шрифт текст" },
  { section: "editor", label: "Интерлиньяж", hints: "межстрочный высота строки" },
  { section: "editor", label: "Номера строк", hints: "гуттер" },
  { section: "editor", label: "Перенос длинных строк", hints: "wrap" },
  { section: "editor", label: "Автосохранение", hints: "задержка сохранение" },
  { section: "editor", label: "Vim", hints: "вим движения jk hl режим" },

  { section: "shortcuts", label: "Шоткаты", hints: "клавиши сочетания keymap перебинд" },

  { section: "agent", label: "Агент", hints: "mcp claude конфиг tasker-mcp" },

  { section: "about", label: "О программе", hints: "версия сборка пути" },
];

/**
 * searchSettings ищет настройку по подписи и подсказкам.
 *
 * Регистр не важен, слова — по вхождению подстроки: настройку ищут по обрывку
 * («проз», «вим»), а не по точному названию, которого как раз и не помнят.
 * Пустой запрос не находит ничего — это не «показать всё», а «ещё не искали».
 */
export function searchSettings(query: string): SettingEntry[] {
  const needle = query.trim().toLowerCase();
  if (needle === "") return [];
  return settingsIndex.filter((entry) =>
    `${entry.label} ${entry.hints ?? ""}`.toLowerCase().includes(needle),
  );
}
