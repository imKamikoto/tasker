import type { Notebook, Tag } from "../api";
import { notebookRows } from "../tree";

export type Filter =
  | { kind: "active" }
  | { kind: "all" }
  | { kind: "notebook"; path: string }
  | { kind: "tags"; names: string[] }
  | { kind: "trash" };

type Props = {
  notebooks: Notebook[];
  tags: Tag[];
  filter: Filter;
  onFilter: (filter: Filter) => void;
  collapsed: string[];
  onToggle: (path: string) => void;
  onDropNote: (id: string, notebook: string) => void;
};

/** Sidebar рисует то, что ему дали: дерево ноутбуков и список тегов. */
export function Sidebar({
  notebooks,
  tags,
  filter,
  onFilter,
  collapsed,
  onToggle,
  onDropNote,
}: Props) {
  const rows = notebookRows(
    notebooks.map((notebook) => ({ path: notebook.Path, count: notebook.Count })),
    collapsed,
  );

  return (
    <nav className="pane pane--sidebar">
      {/* «Активные» первым: это главный экран рабочего дня (SPEC §8.3). */}
      <button
        className="row"
        aria-selected={filter.kind === "active"}
        onClick={() => onFilter({ kind: "active" })}
      >
        <span className="row__label">Активные</span>
      </button>
      <button
        className="row"
        aria-selected={filter.kind === "all"}
        onClick={() => onFilter({ kind: "all" })}
      >
        <span className="row__label">Все заметки</span>
      </button>

      <div className="section-title">Ноутбуки</div>
      {rows.map((row) => (
        <div
          key={row.path}
          className="row"
          aria-selected={filter.kind === "notebook" && filter.path === row.path}
          style={{ paddingLeft: 16 + row.depth * 14 }}
          // preventDefault на dragOver — единственный способ сказать браузеру,
          // что сюда можно бросать. Без него drop не случится вовсе.
          onDragOver={(event) => {
            if (event.dataTransfer.types.includes("text/tasker-note")) event.preventDefault();
          }}
          onDrop={(event) => {
            const id = event.dataTransfer.getData("text/tasker-note");
            if (id) {
              event.preventDefault();
              onDropNote(id, row.path);
            }
          }}
        >
          {/* Треугольник отдельной кнопкой: свернуть ветку и выбрать её —
              разные действия, и путать их щелчком по одному месту нельзя. */}
          <button
            className="row__twisty"
            aria-label={row.collapsed ? "развернуть" : "свернуть"}
            disabled={!row.hasChildren}
            onClick={() => onToggle(row.path)}
          >
            {row.hasChildren ? (row.collapsed ? "▸" : "▾") : ""}
          </button>
          <button className="row__label" onClick={() => onFilter({ kind: "notebook", path: row.path })}>
            {row.name}
          </button>
          <span className="row__count">{row.count || ""}</span>
        </div>
      ))}

      <div className="section-title">Теги</div>
      {tags.map((tag) => (
        <button
          key={tag.Name}
          className="row"
          aria-selected={filter.kind === "tags" && filter.names.includes(tag.Name)}
          // Cmd добавляет тег к отбору: несколько тегов соединяются через И
          // (SPEC §8.2). Обычный щелчок начинает отбор заново.
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
          <span className="row__label tag">#{tag.Name}</span>
          <span className="row__count">{tag.Count || ""}</span>
        </button>
      ))}

      <div className="section-title">&nbsp;</div>
      <button
        className="row"
        aria-selected={filter.kind === "trash"}
        onClick={() => onFilter({ kind: "trash" })}
      >
        <span className="row__label">Корзина</span>
      </button>
    </nav>
  );
}
