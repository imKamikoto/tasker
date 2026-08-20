import type { Note } from "../api";

type Props = {
  note: Note | null;
  error: string | null;
};

/**
 * Editor пока только показывает заметку.
 *
 * Здесь будет CodeMirror 6 с вимом — следующий пункт фазы 3. Пока это место
 * занято текстом как есть, чтобы видеть, что данные доезжают целиком.
 */
export function Editor({ note, error }: Props) {
  if (error) {
    return (
      <div className="pane pane--editor">
        <div className="error">{error}</div>
      </div>
    );
  }
  if (!note) {
    return (
      <div className="pane pane--editor">
        <div className="empty">Выберите заметку слева</div>
      </div>
    );
  }

  return (
    <div className="pane pane--editor">
      <input className="editor__title" value={note.Title} readOnly />
      <div className="editor__meta">
        <span>{note.Notebook || "Корень"}</span>
        {note.Status !== "none" && <span className="status">{note.Status}</span>}
        {note.Tags.map((tag) => (
          <span key={tag} className="tag">
            #{tag}
          </span>
        ))}
        {note.Backlinks.length > 0 && <span>ссылаются: {note.Backlinks.length}</span>}
      </div>
      <div className="editor__body">{note.Body}</div>
    </div>
  );
}
