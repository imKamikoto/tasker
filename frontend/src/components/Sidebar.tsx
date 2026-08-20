import type { Notebook, Tag } from "../api";

export type Filter =
  | { kind: "active" }
  | { kind: "all" }
  | { kind: "notebook"; path: string }
  | { kind: "tag"; name: string };

type Props = {
  notebooks: Notebook[];
  tags: Tag[];
  filter: Filter;
  onFilter: (filter: Filter) => void;
};

/** Sidebar рисует то, что ему дали: дерево ноутбуков и список тегов. */
export function Sidebar({ notebooks, tags, filter, onFilter }: Props) {
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
      {notebooks.map((notebook) => (
        <button
          key={notebook.Path}
          className={notebook.Path.includes("/") ? "row row--nested" : "row"}
          aria-selected={filter.kind === "notebook" && filter.path === notebook.Path}
          onClick={() => onFilter({ kind: "notebook", path: notebook.Path })}
        >
          <span className="row__label">{leafName(notebook.Path)}</span>
          <span className="row__count">{notebook.Count || ""}</span>
        </button>
      ))}

      <div className="section-title">Теги</div>
      {tags.map((tag) => (
        <button
          key={tag.Name}
          className="row"
          aria-selected={filter.kind === "tag" && filter.name === tag.Name}
          onClick={() => onFilter({ kind: "tag", name: tag.Name })}
        >
          <span className="row__label tag">#{tag.Name}</span>
          <span className="row__count">{tag.Count || ""}</span>
        </button>
      ))}
    </nav>
  );
}

/** leafName показывает последний сегмент пути: дерево и так задаёт вложенность. */
function leafName(path: string): string {
  if (path === "") return "Корень";
  const i = path.lastIndexOf("/");
  return i < 0 ? path : path.slice(i + 1);
}
