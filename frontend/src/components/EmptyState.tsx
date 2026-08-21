import { allowedValues, badTokens } from "../querytokens";

/**
 * Рамка из псевдографики вместо иллюстрации.
 *
 * Картинку пришлось бы тащить файлом и красить под две темы; символы берут цвет
 * от текста и живут в тех же переменных, что и всё остальное.
 */
const frame = ["┌──────────────┐", "│  ░ ░ ░ ░ ░   │", "│  ░ ░ ░ ░ ░   │", "└──────────────┘"].join(
  "\n",
);

/** Пустой vault: заметок нет вообще, а не «не нашлось». */
export function EmptyVault({ onCreate }: { onCreate: () => void }) {
  return (
    <div className="empty">
      <div className="empty__frame">{frame}</div>
      <div className="empty__title">Здесь пока ничего нет</div>
      <div className="empty__text">
        Заметки — обычные <code>.md</code> файлы в выбранной папке. Их можно править чем угодно
        ещё: Tasker подхватит изменения сам.
      </div>
      <div className="empty__actions">
        <button className="button button--accent" onClick={onCreate}>
          Новая заметка
        </button>
      </div>
      <div className="empty__keys">
        <span className="key">⌘N</span>
        <span className="key">⌘⌃1..5</span>
        <span className="key">m</span>
      </div>
    </div>
  );
}

type EmptySearchProps = {
  query: string;
  /** Убрать из запроса испорченный кусок. */
  onDropToken: (token: string) => void;
  /** Сбросить отбор в сайдбаре и искать по всему vault. */
  onSearchAll: () => void;
};

/**
 * Пустая выдача. Разделена на два случая: опечатка в закрытом перечислении —
 * это ошибка, которую надо назвать, а не «ничего не найдено», после которого
 * человек будет смотреть на пустой список и гадать.
 */
export function EmptySearch({ query, onDropToken, onSearchAll }: EmptySearchProps) {
  const bad = badTokens(query);

  if (bad.length === 0) {
    return (
      <div className="empty">
        <div className="empty__frame">{"·  ·  ·  ·  ·\n   ·  ·  ·  ·\n·  ·  ·  ·  ·"}</div>
        <div className="empty__title">Ничего не найдено</div>
        <div className="empty__text">
          По этому запросу нет ни одной заметки. Можно снять фильтры и поискать по всему vault.
        </div>
        <div className="empty__actions">
          <button className="button" onClick={onSearchAll}>
            Искать во всех
          </button>
        </div>
      </div>
    );
  }

  const token = bad[0];
  const name = token.replace(/^-/, "").split(":")[0];
  const values = allowedValues(name);

  return (
    <div className="empty">
      <div className="empty__frame">{"·  ·  ·  ·  ·\n   ·  ·  ·  ·"}</div>
      <div className="empty__title">
        <code>{token}</code> — такого значения нет
      </div>
      {values.length > 0 && (
        <div className="empty__text">
          У <code>{name}:</code> допустимы: {values.join(", ")}.
        </div>
      )}
      <div className="empty__actions">
        <button className="button button--link" onClick={() => onDropToken(token)}>
          убрать токен
        </button>
        <button className="button button--link" onClick={onSearchAll}>
          искать во всех
        </button>
      </div>
    </div>
  );
}
