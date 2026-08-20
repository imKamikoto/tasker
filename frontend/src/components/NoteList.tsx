import type { Note } from "../api";

type Props = {
  notes: Note[];
  selected: string | null;
  query: string;
  error: string | null;
  onQuery: (query: string) => void;
  onSelect: (id: string) => void;
};

/** NoteList — поле запроса и список найденного. */
export function NoteList({ notes, selected, query, error, onQuery, onSelect }: Props) {
  return (
    <div className="pane pane--list">
      <input
        className="search"
        placeholder="Поиск: слово, tag:баг, status:active…"
        value={query}
        onChange={(event) => onQuery(event.target.value)}
        spellCheck={false}
        autoCorrect="off"
      />

      {error && <div className="error">{error}</div>}
      {!error && notes.length === 0 && <div className="empty">Ничего не найдено</div>}

      {notes.map((note) => (
        <button
          key={note.ID}
          className="note"
          aria-selected={note.ID === selected}
          onClick={() => onSelect(note.ID)}
        >
          <div className="note__title">
            {note.Pinned && <span className="pin">★</span>}
            <span>{note.Title}</span>
          </div>
          {note.Excerpt && <div className="note__excerpt">{note.Excerpt}</div>}
          <div className="note__meta">
            <span>{formatDate(note.Updated)}</span>
            {note.Status !== "none" && <span className="status">{note.Status}</span>}
            {note.NumTasks > 0 && (
              <span>
                {note.NumDone}/{note.NumTasks}
              </span>
            )}
            {note.Tags.map((tag) => (
              <span key={tag} className="tag">
                #{tag}
              </span>
            ))}
          </div>
        </button>
      ))}
    </div>
  );
}

/**
 * formatDate печатает дату так, как её показал бы Finder: сегодняшнее время,
 * всё остальное — датой.
 */
function formatDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";

  const now = new Date();
  const sameDay =
    date.getFullYear() === now.getFullYear() &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate();

  return sameDay
    ? date.toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit" })
    : date.toLocaleDateString("ru-RU", { day: "2-digit", month: "2-digit", year: "numeric" });
}
