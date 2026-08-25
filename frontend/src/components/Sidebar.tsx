import { useEffect, useMemo, useRef, useState } from "react";

import type { Counts, Notebook, Tag } from "../api";
import { tagColor, tagStyle } from "../tags";
import { notebookRows } from "../tree";
import type { RampState } from "../focus";
import { FocusRamp } from "./FocusRamp";
import { topAccented, topGlyphs, topLabels, topRows, type TopKind } from "../toprows";
import { NameInput } from "./NameInput";

/** Какой отбор включает верхний пункт. */
function topFilter(kind: TopKind): Filter {
  return kind === "active" ? { kind: "active" } : kind === "all" ? { kind: "all" } : { kind: "agent" };
}

/** Счётчик рядом с верхним пунктом. */
function topCount(kind: TopKind, counts: Counts): number {
  return kind === "active" ? counts.Active : kind === "all" ? counts.All : counts.Agent;
}

export type Filter =
  | { kind: "active" }
  | { kind: "all" }
  | { kind: "agent" }
  | { kind: "notebook"; path: string }
  | { kind: "tags"; names: string[] }
  | { kind: "trash" };

type Props = {
  notebooks: Notebook[];
  tags: Tag[];
  counts: Counts;
  /** Выбранные вручную цвета тегов: имя → номер в палитре. */
  tagColors: Record<string, number>;
  filter: Filter;
  onFilter: (filter: Filter) => void;
  collapsed: string[];
  /** Секции сайдбара, свёрнутые целиком. */
  /** Порядок и видимость верхних пунктов — из настроек. */
  /** Что показать в полосе колонки: рампу, точки или ничего. */
  ramp: RampState;
  topOrder: string[];
  topHidden: string[];
  notebooksCollapsed: boolean;
  tagsCollapsed: boolean;
  onToggleSection: (section: "notebooks" | "tags") => void;
  onToggle: (path: string) => void;
  /** Сколько заметок прилетит, если бросить сюда: показывается вместо счётчика. */
  dragging: number;
  /** Колонка принимает клавиши: показываем рамку и курсор. */
  focused: boolean;
  /** Команда из раскладки: сюда доходит только то, что назначено на сайдбар. */
  command: { name: string; token: number };
  /** Открыт ли экран настроек — шестерёнка это показывает. */
  settingsOpen: boolean;
  onSettings: () => void;
  onDropNote: (id: string, notebook: string) => void;
  onCreateNotebook: (path: string) => void;
  onRenameNotebook: (from: string, to: string) => void;
  onDeleteNotebook: (path: string) => void;
  onRenameTag: (from: string, to: string) => void;
  onDeleteTag: (name: string) => void;
};

/**
 * Один шаг курсора по сайдбару.
 *
 * Ноутбуки несут ещё и состояние ветки: свернуть и развернуть — это тоже
 * движение курсора, только поперёк.
 */
type Item = {
  key: string;
  filter: Filter;
  path?: string;
  hasChildren?: boolean;
  collapsed?: boolean;
};

/** Sidebar рисует то, что ему дали: дерево ноутбуков и список тегов. */
export function Sidebar({
  notebooks,
  tags,
  counts,
  tagColors,
  filter,
  onFilter,
  collapsed,
  ramp,
  topOrder,
  topHidden,
  notebooksCollapsed,
  tagsCollapsed,
  onToggleSection,
  onToggle,
  dragging,
  focused,
  command,
  settingsOpen,
  onSettings,
  onDropNote,
  onCreateNotebook,
  onRenameNotebook,
  onDeleteNotebook,
  onRenameTag,
  onDeleteTag,
}: Props) {
  // Куда добавляем ноутбук: null — не добавляем, "" — в корень, иначе внутрь.
  const [adding, setAdding] = useState<string | null>(null);
  const [renaming, setRenaming] = useState<string | null>(null);
  // Открытое меню одно на весь сайдбар, поэтому ключ общий для ноутбуков и
  // тегов — и по той же схеме, что и cursorKey: путь ноутбука и имя тега живут
  // в разных пространствах и без префикса могли бы совпасть.
  const [menu, setMenu] = useState<string | null>(null);
  const [renamingTag, setRenamingTag] = useState<string | null>(null);
  // Ноутбук под курсором с перетаскиваемой заметкой. Подсветка нужна до
  // отпускания кнопки: иначе непонятно, куда именно попадёт.
  const [dropTarget, setDropTarget] = useState<string | null>(null);

  const tops = useMemo(
    () => topRows(topOrder, topHidden, counts.Agent > 0),
    [topOrder, topHidden, counts.Agent],
  );

  const rows = notebookRows(
    notebooks.map((notebook) => ({ path: notebook.Path, count: notebook.Count })),
    collapsed,
  );

  // Плоский список того, по чему ходит курсор. Собирается из тех же данных,
  // что и разметка, и в том же порядке: два независимых списка разошлись бы
  // на первом же ноутбуке без заметок.
  const items = useMemo<Item[]>(() => {
    // Из того же списка, что и разметка: два независимых порядка разошлись бы
    // на первой же перестановке, и курсор пошёл бы не туда, куда смотрят.
    const out: Item[] = tops.map((kind) => ({ key: kind, filter: topFilter(kind) }));
    // Свёрнутая секция выпадает и отсюда: иначе курсор уезжал бы в строки,
    // которых на экране нет, и j/k выглядели бы сломанными.
    if (!notebooksCollapsed) {
      for (const row of rows) {
        out.push({
          key: `book:${row.path}`,
          filter: { kind: "notebook", path: row.path },
          path: row.path,
          hasChildren: row.hasChildren,
          collapsed: row.collapsed,
        });
      }
    }
    if (!tagsCollapsed) {
      for (const tag of tags) {
        out.push({ key: `tag:${tag.Name}`, filter: { kind: "tags", names: [tag.Name] } });
      }
    }
    out.push({ key: "trash", filter: { kind: "trash" } });
    return out;
  }, [tops, rows, tags, notebooksCollapsed, tagsCollapsed]);

  const [cursor, setCursor] = useState(0);
  // Список мог укоротиться под курсором: ноутбук свернули или удалили тег.
  useEffect(() => setCursor((at) => Math.min(at, Math.max(0, items.length - 1))), [items.length]);

  // Курсор ведём в ref: обработчик команды живёт вне рендера, а состояние из
  // замыкания того рендера, в котором его повесили, устаревает мгновенно.
  const latest = useRef({ items, cursor, onFilter, onToggle });
  latest.current = { items, cursor, onFilter, onToggle };

  useEffect(() => {
    if (command.token === 0) return;
    const { items: list, cursor: at, onFilter: filterNow, onToggle: toggle } = latest.current;
    const item = list[at];

    switch (command.name) {
      case "sidebar.down":
        setCursor((current) => Math.min(list.length - 1, current + 1));
        return;
      case "sidebar.up":
        setCursor((current) => Math.max(0, current - 1));
        return;
      case "sidebar.open":
        if (item) filterNow(item.filter);
        return;
      case "sidebar.expand":
        // Развернуть можно только свёрнутую ветку; на остальном молчим, а не
        // делаем что-нибудь наугад.
        if (item?.hasChildren && item.collapsed && item.path !== undefined) toggle(item.path);
        return;
      case "sidebar.collapse":
        if (item?.hasChildren && !item.collapsed && item.path !== undefined) toggle(item.path);
        return;
    }
  }, [command]);

  // Курсор помечается атрибутом, а не ref на каждой строке: строк переменное
  // число, и раздавать им ссылки ради одной подсветки незачем.
  const cursorKey = focused ? items[cursor]?.key : undefined;
  const nav = useRef<HTMLElement | null>(null);
  useEffect(() => {
    nav.current?.querySelector("[data-cursor='true']")?.scrollIntoView({ block: "nearest" });
  }, [cursorKey]);

  return (
    <nav className="pane pane--sidebar" ref={nav} data-focused={focused}>
      {/* Шестерёнка стоит на одном уровне со светофором: это верхняя полоса
          окна, и другого места «над всем» в интерфейсе нет. Полоса тянет окно,
          поэтому кнопка отменяет перетаскивание у себя. */}
      {/* В полосе остались только светофор и рампа: кликабельного здесь быть
          не должно — это зона перетаскивания окна. */}
      <div className="drag-strip drag-strip--sidebar">
        <FocusRamp kind="sidebar" state={ramp} />
      </div>

      {/* Порядок и видимость правит человек в настройках; правила «чего не
          бывает» и «что появляется само» — в toprows.ts. */}
      <div className="sidebar__group sidebar__group--top">
        {tops.map((kind) => (
          <button
            key={kind}
            className="row row--top"
            data-cursor={cursorKey === kind}
            aria-selected={filter.kind === kind}
            onClick={() => onFilter(topFilter(kind))}
          >
            <span className={topAccented[kind] ? "row__glyph row__glyph--accent" : "row__glyph"}>
              {topGlyphs[kind]}
            </span>
            <span className="row__label">{topLabels[kind]}</span>
            <span className="row__count">{topCount(kind, counts) || ""}</span>
          </button>
        ))}
      </div>

      <div className="sidebar__scroll">
        <div className="section">
          {/* Заголовок сам сворачивает секцию: отдельный треугольник рядом
              означал бы два места для одного действия. */}
          <button
            className="section__toggle"
            aria-expanded={!notebooksCollapsed}
            title={notebooksCollapsed ? "Развернуть ноутбуки" : "Свернуть ноутбуки"}
            onClick={() => onToggleSection("notebooks")}
          >
            <span className="section__caret">{notebooksCollapsed ? "▸" : "▾"}</span>
            <span className="section__label">Ноутбуки</span>
          </button>
          <span className="section__rule" />
          <button
            className="section__add"
            aria-label="новый ноутбук"
            title="Новый ноутбук"
            onClick={() => {
              // Заводить ноутбук в свёрнутой секции незачем: поле ввода и
              // новая строка появятся там, где их не видно.
              if (notebooksCollapsed) onToggleSection("notebooks");
              setAdding("");
            }}
          >
            +
          </button>
        </div>

        {!notebooksCollapsed && adding === "" && (
          <NameInput
            initial=""
            placeholder="Имя ноутбука"
            hint="⏎ создать · esc отменить"
            onCancel={() => setAdding(null)}
            onCommit={(name) => {
              setAdding(null);
              onCreateNotebook(name);
            }}
          />
        )}

        <div className="sidebar__group" hidden={notebooksCollapsed}>
          {rows.map((row) => (
            <div
              key={row.path}
              className="row"
              data-cursor={cursorKey === `book:${row.path}`}
              aria-selected={filter.kind === "notebook" && filter.path === row.path}
              data-drop={dropTarget === row.path}
              style={{ paddingLeft: 8 + row.depth * 22 }}
              // preventDefault на dragOver — единственный способ сказать браузеру,
              // что сюда можно бросать. Без него drop не случится вовсе.
              onDragOver={(event) => {
                if (!event.dataTransfer.types.includes("text/tasker-note")) return;
                event.preventDefault();
                setDropTarget(row.path);
              }}
              onDragLeave={() => setDropTarget((current) => (current === row.path ? null : current))}
              onDrop={(event) => {
                const id = event.dataTransfer.getData("text/tasker-note");
                setDropTarget(null);
                if (id) {
                  event.preventDefault();
                  onDropNote(id, row.path);
                }
              }}
            >
              {/* Треугольник отдельной кнопкой: свернуть ветку и выбрать её —
                  разные действия, и путать их щелчком по одному месту нельзя.
                  У листьев на его месте стоит уголок — он ничего не делает. */}
              {row.hasChildren ? (
                <button
                  className="row__glyph row__glyph--accent"
                  aria-label={row.collapsed ? "развернуть" : "свернуть"}
                  onClick={() => onToggle(row.path)}
                >
                  {row.collapsed ? "▸" : "▾"}
                </button>
              ) : (
                <span className="row__glyph">{row.depth > 0 ? "└" : ""}</span>
              )}

              {renaming === row.path ? (
                <NameInput
                  initial={row.name}
                  placeholder="Имя ноутбука"
                  hint="⏎ переименовать · esc отменить"
                  onCancel={() => setRenaming(null)}
                  onCommit={(name) => {
                    setRenaming(null);
                    // Переименовываем последний сегмент, оставляя ноутбук на месте
                    // в дереве: сменить имя и переехать — разные действия.
                    const parent = row.path.includes("/")
                      ? row.path.slice(0, row.path.lastIndexOf("/")) + "/"
                      : "";
                    onRenameNotebook(row.path, parent + name);
                  }}
                />
              ) : (
                <>
                  <button
                    className="row__label"
                    onClick={() => onFilter({ kind: "notebook", path: row.path })}
                  >
                    {row.name}
                  </button>
                  {row.path !== "" && (
                    <button
                      className="row__more"
                      aria-label="действия с ноутбуком"
                      onClick={() =>
                        setMenu(menu === `book:${row.path}` ? null : `book:${row.path}`)
                      }
                    >
                      ⋯
                    </button>
                  )}
                  <span className="row__count">
                    {dropTarget === row.path && dragging > 0 ? `+${dragging}` : row.count || ""}
                  </span>
                </>
              )}

              {menu === `book:${row.path}` && (
                <div className="menu menu--right" onMouseLeave={() => setMenu(null)}>
                  <button
                    className="menu__item"
                    onClick={() => {
                      setMenu(null);
                      setAdding(row.path);
                    }}
                  >
                    <span className="menu__label">Вложенный ноутбук</span>
                  </button>
                  <button
                    className="menu__item"
                    onClick={() => {
                      setMenu(null);
                      setRenaming(row.path);
                    }}
                  >
                    <span className="menu__label">Переименовать</span>
                  </button>
                  <button
                    className="menu__item menu__item--danger"
                    onClick={() => {
                      setMenu(null);
                      onDeleteNotebook(row.path);
                    }}
                  >
                    <span className="menu__label">Удалить в корзину</span>
                  </button>
                </div>
              )}
            </div>
          ))}
        </div>

        {!notebooksCollapsed && adding !== null && adding !== "" && (
          <NameInput
            initial=""
            placeholder={`Внутри «${adding}»`}
            hint="⏎ создать · esc отменить"
            onCancel={() => setAdding(null)}
            onCommit={(name) => {
              setAdding(null);
              onCreateNotebook(`${adding}/${name}`);
            }}
          />
        )}

        {tags.length > 0 && (
          <div className="section">
            <button
              className="section__toggle"
              aria-expanded={!tagsCollapsed}
              title={tagsCollapsed ? "Развернуть теги" : "Свернуть теги"}
              onClick={() => onToggleSection("tags")}
            >
              <span className="section__caret">{tagsCollapsed ? "▸" : "▾"}</span>
              <span className="section__label">Теги</span>
            </button>
            <span className="section__rule" />
          </div>
        )}

        <div className="sidebar__group" hidden={tagsCollapsed}>
          {tags.map((tag) =>
            renamingTag === tag.Name ? (
              <NameInput
                key={tag.Name}
                initial={tag.Name}
                placeholder="Имя тега"
                hint="⏎ переименовать во всех заметках · esc отменить"
                onCancel={() => setRenamingTag(null)}
                onCommit={(name) => {
                  setRenamingTag(null);
                  onRenameTag(tag.Name, name);
                }}
              />
            ) : (
              <div
                key={tag.Name}
                className="row"
                data-cursor={cursorKey === `tag:${tag.Name}`}
                aria-selected={filter.kind === "tags" && filter.names.includes(tag.Name)}
              >
                <span className="row__swatch" style={tagStyle(tagColor(tag.Name, tagColors))} />
                {/* Cmd добавляет тег к отбору: несколько тегов соединяются через И
                    (SPEC §8.2). Обычный щелчок начинает отбор заново. */}
                <button
                  className="row__label"
                  onClick={(event) => {
                    const current = filter.kind === "tags" ? filter.names : [];
                    const names = event.metaKey
                      ? current.includes(tag.Name)
                        ? current.filter((name) => name !== tag.Name)
                        : [...current, tag.Name]
                      : [tag.Name];
                    onFilter(names.length > 0 ? { kind: "tags", names } : { kind: "all" });
                  }}
                >
                  {tag.Name}
                </button>
                <button
                  className="row__more"
                  aria-label="действия с тегом"
                  onClick={() => setMenu(menu === `tag:${tag.Name}` ? null : `tag:${tag.Name}`)}
                >
                  ⋯
                </button>
                <span className="row__count">{tag.Count || ""}</span>

                {menu === `tag:${tag.Name}` && (
                  <div className="menu menu--right" onMouseLeave={() => setMenu(null)}>
                    <button
                      className="menu__item"
                      onClick={() => {
                        setMenu(null);
                        setRenamingTag(tag.Name);
                      }}
                    >
                      <span className="menu__label">Переименовать</span>
                    </button>
                    <button
                      className="menu__item menu__item--danger"
                      onClick={() => {
                        setMenu(null);
                        onDeleteTag(tag.Name);
                      }}
                    >
                      <span className="menu__label">Удалить во всех заметках</span>
                    </button>
                  </div>
                )}
              </div>
            ),
          )}
        </div>
      </div>

      <div className="sidebar__foot">
        <button
          className="row row--top"
          data-cursor={cursorKey === "trash"}
          aria-selected={filter.kind === "trash"}
          onClick={() => onFilter({ kind: "trash" })}
        >
          <span className="row__glyph">▚</span>
          <span className="row__label">Корзина</span>
          <span className="row__count">{counts.Trashed || ""}</span>
        </button>
        {/* Настройки строкой под корзиной, а не иконкой в титульной строке:
            наверху теперь рампа, а сама полоса — зона перетаскивания окна. */}
        <button
          className="row row--top"
          aria-expanded={settingsOpen}
          title="Настройки (⌘,)"
          onClick={onSettings}
        >
          <span className="row__glyph">{"\u2699\uFE0E"}</span>
          <span className="row__label">Настройки</span>
        </button>
      </div>
    </nav>
  );
}
