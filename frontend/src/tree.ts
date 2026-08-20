/** Ноутбук, каким его отдаёт Go: путь и число своих заметок. */
export type NotebookNode = { path: string; count: number };

/** Строка дерева, готовая к отрисовке. */
export type NotebookRow = {
  path: string;
  /** Последний сегмент пути; для корня vault — «Корень». */
  name: string;
  depth: number;
  /**
   * Счётчик по правилу SPEC §8.1: свёрнутый ноутбук считает вложенные,
   * развёрнутый — только свои. Так число под треугольником всегда отвечает на
   * вопрос «сколько там внутри», а не «сколько лежит ровно здесь».
   */
  count: number;
  hasChildren: boolean;
  collapsed: boolean;
};

/**
 * notebookRows разворачивает плоский список в видимые строки дерева.
 *
 * Вложенность выводится из самих путей, а не берётся отдельным полем: путь и
 * есть структура, и держать её в двух местах — значит однажды их рассинхронить.
 */
export function notebookRows(notebooks: NotebookNode[], collapsed: string[]): NotebookRow[] {
  const hidden = new Set(collapsed);
  const own = new Map(notebooks.map((notebook) => [notebook.path, notebook.count]));

  // Корень vault — обычная строка верхнего уровня, а не родитель всего дерева:
  // иначе его сворачивание прятало бы вообще все заметки.
  const sorted = [...notebooks].sort((a, b) => a.path.localeCompare(b.path, "ru"));

  const rows: NotebookRow[] = [];
  for (const notebook of sorted) {
    if (hiddenByAncestor(notebook.path, hidden)) continue;

    const children = sorted.filter((other) => parentOf(other.path) === notebook.path);
    const isCollapsed = hidden.has(notebook.path);
    rows.push({
      path: notebook.path,
      name: leafName(notebook.path),
      depth: depthOf(notebook.path),
      count: isCollapsed ? withDescendants(notebook.path, own) : notebook.count,
      hasChildren: children.length > 0,
      collapsed: isCollapsed,
    });
  }
  return rows;
}

/** Строка скрыта, если свёрнут любой из её предков. */
function hiddenByAncestor(path: string, hidden: Set<string>): boolean {
  for (let parent = parentOf(path); parent !== null; parent = parentOf(parent)) {
    if (hidden.has(parent)) return true;
  }
  return false;
}

/** withDescendants складывает заметки ноутбука и всех вложенных. */
function withDescendants(path: string, own: Map<string, number>): number {
  let total = 0;
  for (const [other, count] of own) {
    if (other === path || other.startsWith(path + "/")) total += count;
  }
  return total;
}

/** parentOf возвращает родителя или null для верхнего уровня. */
export function parentOf(path: string): string | null {
  const i = path.lastIndexOf("/");
  return i < 0 ? null : path.slice(0, i);
}

function depthOf(path: string): number {
  return path === "" ? 0 : path.split("/").length - 1;
}

function leafName(path: string): string {
  if (path === "") return "Корень";
  const i = path.lastIndexOf("/");
  return i < 0 ? path : path.slice(i + 1);
}
