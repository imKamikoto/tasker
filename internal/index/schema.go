package index

// schemaVersion меняется при любой правке схемы ниже. Не совпала с тем, что
// лежит в файле, — индекс сносится и строится заново. Это всегда безопасно:
// правда в файлах, индекс производный (SPEC §5.2).
const schemaVersion = "1"

// schemaSQL — схема из SPEC §5.1.
//
// contentless_delete=1 у notes_fts обязателен и добавлен по итогам дымового
// теста фазы 0: без него строку из полнотекстового индекса нельзя удалить, не
// передав старое содержимое, а при удалении файла его уже нет.
const schemaSQL = `
CREATE TABLE notes (
  rowid       INTEGER PRIMARY KEY,
  id          TEXT NOT NULL UNIQUE,
  path        TEXT NOT NULL UNIQUE,
  notebook    TEXT NOT NULL,
  title       TEXT NOT NULL,
  status      TEXT NOT NULL DEFAULT 'none',
  pinned      INTEGER NOT NULL DEFAULT 0,
  created     INTEGER NOT NULL,
  updated     INTEGER NOT NULL,
  mtime       INTEGER NOT NULL,   -- в наносекундах: правка той же длины
                                  -- в ту же секунду иначе не отличается
  size        INTEGER NOT NULL,
  num_tasks   INTEGER NOT NULL DEFAULT 0,
  num_done    INTEGER NOT NULL DEFAULT 0,
  excerpt     TEXT NOT NULL DEFAULT '',
  trashed     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_notes_notebook ON notes(notebook);
CREATE INDEX idx_notes_updated  ON notes(updated DESC);
CREATE INDEX idx_notes_status   ON notes(status);

CREATE TABLE tags (
  name  TEXT PRIMARY KEY COLLATE NOCASE,
  color TEXT NOT NULL DEFAULT 'default'
);

CREATE TABLE note_tags (
  note_id TEXT NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
  tag     TEXT NOT NULL COLLATE NOCASE,
  PRIMARY KEY (note_id, tag)
);
CREATE INDEX idx_note_tags_tag ON note_tags(tag);

CREATE TABLE links (
  src TEXT NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
  dst TEXT NOT NULL,
  PRIMARY KEY (src, dst)
);
CREATE INDEX idx_links_dst ON links(dst);

CREATE VIRTUAL TABLE notes_fts USING fts5(
  title, body,
  tokenize = 'trigram',
  content = '',
  contentless_delete = 1
);

CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
`

// dropSQL сносит всё, что создаёт schemaSQL. Порядок обратный созданию, чтобы
// внешние ключи не мешали.
var dropSQL = []string{
	`DROP TABLE IF EXISTS meta`,
	`DROP TABLE IF EXISTS notes_fts`,
	`DROP TABLE IF EXISTS links`,
	`DROP TABLE IF EXISTS note_tags`,
	`DROP TABLE IF EXISTS tags`,
	`DROP TABLE IF EXISTS notes`,
}
