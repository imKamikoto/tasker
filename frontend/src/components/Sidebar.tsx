import { useState } from "react";

import type { Notebook, Tag } from "../api";
import { notebookRows } from "../tree";
import { NameInput } from "./NameInput";

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
  onCreateNotebook: (path: string) => void;
  onRenameNotebook: (from: string, to: string) => void;
  onDeleteNotebook: (path: string) => void;
  onRenameTag: (from: string, to: string) => void;
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
  onCreateNotebook,
  onRenameNotebook,
  onDeleteNotebook,
  onRenameTag,
}: Props) {
  // Куда добавляем ноутбук: null — не добавляем, "" — в корень, иначе внутрь.
  const [adding, setAdding] = useState<string | null>(null);
  const [renaming, setRenaming] = useState<string | null>(null);
  const [menu, setMenu] = useState<string | null>(null);
  const [renamingTag, setRenamingTag] = useState<string | null>(null);

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

      <div className="section-title section-title--actions">
        Ноутбуки
        <button
          className="section-title__add"
          aria-label="новый ноутбук"
          title="Новый ноутбук"
          onClick={() => setAdding("")}
        >
          +
        </button>
      </div>

      {adding === "" && (
        <NameInput
          initial=""
          placeholder="Имя ноутбука"
          onCancel={() => setAdding(null)}
          onCommit={(name) => {
            setAdding(null);
            onCreateNotebook(name);
          }}
        />
      )}
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
          {renaming === row.path ? (
            <NameInput
              initial={row.name}
              placeholder="Имя ноутбука"
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
              <button className="row__label" onClick={() => onFilter({ kind: "notebook", path: row.path })}>
                {row.name}
              </button>
              {row.path !== "" && (
                <button
                  className="row__more"
                  aria-label="действия с ноутбуком"
                  onClick={() => setMenu(menu === row.path ? null : row.path)}
                >
                  ⋯
                </button>
              )}
              <span className="row__count">{row.count || ""}</span>
            </>
          )}

          {menu === row.path && (
            <div className="menu" onMouseLeave={() => setMenu(null)}>
              <button onClick={() => { setMenu(null); setAdding(row.path); }}>Вложенный ноутбук</button>
              <button onClick={() => { setMenu(null); setRenaming(row.path); }}>Переименовать</button>
              <button onClick={() => { setMenu(null); onDeleteNotebook(row.path); }}>
                Удалить в корзину
              </button>
            </div>
          )}
        </div>
      ))}

      {adding !== null && adding !== "" && (
        <NameInput
          initial=""
          placeholder={`Внутри «${adding}»`}
          onCancel={() => setAdding(null)}
          onCommit={(name) => {
            setAdding(null);
            onCreateNotebook(`${adding}/${name}`);
          }}
        />
      )}

      <div className="section-title">Теги</div>
      {tags.map((tag) =>
        renamingTag === tag.Name ? (
          <NameInput
            key={tag.Name}
            initial={tag.Name}
            placeholder="Имя тега"
            onCancel={() => setRenamingTag(null)}
            onCommit={(name) => {
              setRenamingTag(null);
              onRenameTag(tag.Name, name);
            }}
          />
        ) : (
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
          <span
            className="row__more"
            role="button"
            aria-label="переименовать тег"
            title="Переименовать во всех заметках"
            onClick={(event) => {
              // Щелчок по многоточию не должен ещё и отбирать по тегу.
              event.stopPropagation();
              setRenamingTag(tag.Name);
            }}
          >
            ⋯
          </span>
          <span className="row__count">{tag.Count || ""}</span>
        </button>
        ),
      )}

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
