package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound — заметки с таким id в индексе нет.
var ErrNotFound = errors.New("note not in index")

// Index — производный поисковый индекс поверх vault.
type Index struct {
	db *sql.DB
}

// FileState — то, по чему инкрементальный скан решает, менялся ли файл
// (SPEC §5.2).
type FileState struct {
	ID      string
	ModTime time.Time
	Size    int64
}

// Open открывает индекс, создавая схему при необходимости.
func Open(ctx context.Context, path string) (*Index, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("open index %s: %w", path, err)
	}

	// SPEC §5.2: запись идёт через одно соединение. В базу одновременно пишут
	// приложение и tasker-mcp, а внутри приложения — вызовы из вебвью, которые
	// приходят конкурентно и никем не сериализуются.
	db.SetMaxOpenConns(1)

	ix := &Index{db: db}
	if err := ix.ensureSchema(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("open index %s: %w", path, err)
	}
	return ix, nil
}

// dsn собирает строку подключения. Прагмы задаются здесь, а не отдельным Exec:
// они действуют на соединение, а пул волен закрыть и открыть его заново.
func dsn(path string) string {
	u := url.URL{
		Scheme:   "file",
		Path:     path,
		RawQuery: "_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)",
	}
	return u.String()
}

func (ix *Index) Close() error { return ix.db.Close() }

func (ix *Index) ensureSchema(ctx context.Context) error {
	version, err := ix.storedVersion(ctx)
	if err != nil {
		return err
	}
	if version == schemaVersion {
		return nil
	}

	for _, stmt := range dropSQL {
		if _, err := ix.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("drop schema: %w", err)
		}
	}
	if _, err := ix.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	if _, err := ix.db.ExecContext(ctx,
		`INSERT INTO meta(key, value) VALUES('schema_version', ?)`, schemaVersion); err != nil {
		return fmt.Errorf("write schema version: %w", err)
	}
	return nil
}

// storedVersion возвращает версию схемы из файла. Пустая строка означает, что
// индекса ещё нет или он от другой версии приложения.
func (ix *Index) storedVersion(ctx context.Context) (string, error) {
	var name string
	err := ix.db.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'meta'`).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read schema version: %w", err)
	}

	var version string
	err = ix.db.QueryRowContext(ctx,
		`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}

// Put вставляет или обновляет заметку целиком.
func (ix *Index) Put(ctx context.Context, r Record) error {
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("put note %s: %w", r.ID, err)
	}
	defer tx.Rollback()

	// Путь уникален. Если заметка переехала на место другой, а строку той ещё
	// не удалили, вставка упадёт на UNIQUE — убираем занявшего заранее.
	if err := deleteByPath(ctx, tx, r.Path, r.ID); err != nil {
		return fmt.Errorf("put note %s: %w", r.ID, err)
	}

	var rowid int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO notes (id, path, notebook, title, status, pinned,
		                   created, updated, mtime, size, num_tasks, num_done, excerpt, trashed)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			path = excluded.path, notebook = excluded.notebook, title = excluded.title,
			status = excluded.status, pinned = excluded.pinned, created = excluded.created,
			updated = excluded.updated, mtime = excluded.mtime, size = excluded.size,
			num_tasks = excluded.num_tasks, num_done = excluded.num_done,
			excerpt = excluded.excerpt, trashed = excluded.trashed
		RETURNING rowid`,
		r.ID, r.Path, r.Notebook, r.Title, r.Status, r.Pinned,
		r.Created.Unix(), r.Updated.Unix(), r.ModTime.UnixNano(), r.Size,
		r.NumTasks, r.NumDone, r.Excerpt, r.Trashed).Scan(&rowid)
	if err != nil {
		return fmt.Errorf("put note %s: %w", r.ID, err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM note_tags WHERE note_id = ?`, r.ID); err != nil {
		return fmt.Errorf("put note %s: %w", r.ID, err)
	}
	for _, tag := range r.Tags {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tags(name) VALUES(?)`, tag); err != nil {
			return fmt.Errorf("put note %s: %w", r.ID, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO note_tags(note_id, tag) VALUES(?,?)`, r.ID, tag); err != nil {
			return fmt.Errorf("put note %s: %w", r.ID, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM links WHERE src = ?`, r.ID); err != nil {
		return fmt.Errorf("put note %s: %w", r.ID, err)
	}
	for _, dst := range r.Links {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO links(src, dst) VALUES(?,?)`, r.ID, dst); err != nil {
			return fmt.Errorf("put note %s: %w", r.ID, err)
		}
	}

	// UPSERT на виртуальной таблице FTS5 не поддерживается вовсе, поэтому
	// обновление строки — это удаление и вставка. Работает только благодаря
	// contentless_delete=1.
	if _, err := tx.ExecContext(ctx, `DELETE FROM notes_fts WHERE rowid = ?`, rowid); err != nil {
		return fmt.Errorf("put note %s: %w", r.ID, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO notes_fts(rowid, title, body) VALUES(?,?,?)`, rowid, r.Title, r.Body); err != nil {
		return fmt.Errorf("put note %s: %w", r.ID, err)
	}

	return tx.Commit()
}

// Delete убирает из индекса заметку по пути. Отсутствие строки — не ошибка:
// скан удаляет пропавшие файлы и не обязан знать, были ли они там.
func (ix *Index) Delete(ctx context.Context, path string) error {
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete note %s: %w", path, err)
	}
	defer tx.Rollback()

	if err := deleteByPath(ctx, tx, path, ""); err != nil {
		return fmt.Errorf("delete note %s: %w", path, err)
	}
	return tx.Commit()
}

// deleteByPath удаляет строку по пути, пропуская заметку с id keepID.
// Строку полнотекстового индекса приходится убирать руками: внешних ключей у
// виртуальной таблицы нет, и каскад её не достаёт.
func deleteByPath(ctx context.Context, tx *sql.Tx, path, keepID string) error {
	var rowid int64
	err := tx.QueryRowContext(ctx,
		`SELECT rowid FROM notes WHERE path = ? AND id <> ?`, path, keepID).Scan(&rowid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM notes_fts WHERE rowid = ?`, rowid); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM notes WHERE rowid = ?`, rowid)
	return err
}

// Get читает заметку по id.
func (ix *Index) Get(ctx context.Context, id string) (Record, error) {
	var (
		r                       Record
		created, updated, mtime int64
	)
	err := ix.db.QueryRowContext(ctx, `
		SELECT id, path, notebook, title, status, pinned,
		       created, updated, mtime, size, num_tasks, num_done, excerpt, trashed
		FROM notes WHERE id = ?`, id).Scan(
		&r.ID, &r.Path, &r.Notebook, &r.Title, &r.Status, &r.Pinned,
		&created, &updated, &mtime, &r.Size, &r.NumTasks, &r.NumDone, &r.Excerpt, &r.Trashed)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, fmt.Errorf("get note %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return Record{}, fmt.Errorf("get note %s: %w", id, err)
	}
	r.Created = time.Unix(created, 0)
	r.Updated = time.Unix(updated, 0)
	r.ModTime = time.Unix(0, mtime)

	if r.Tags, err = ix.collect(ctx, `SELECT tag FROM note_tags WHERE note_id = ? ORDER BY tag`, id); err != nil {
		return Record{}, fmt.Errorf("get note %s: %w", id, err)
	}
	if r.Links, err = ix.collect(ctx, `SELECT dst FROM links WHERE src = ? ORDER BY dst`, id); err != nil {
		return Record{}, fmt.Errorf("get note %s: %w", id, err)
	}
	return r, nil
}

func (ix *Index) collect(ctx context.Context, query, arg string) ([]string, error) {
	rows, err := ix.db.QueryContext(ctx, query, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// States отдаёт снимок индекса для инкрементального скана: путь → что мы про
// него помним.
func (ix *Index) States(ctx context.Context) (map[string]FileState, error) {
	rows, err := ix.db.QueryContext(ctx, `SELECT path, id, mtime, size FROM notes`)
	if err != nil {
		return nil, fmt.Errorf("read index states: %w", err)
	}
	defer rows.Close()

	states := make(map[string]FileState)
	for rows.Next() {
		var (
			path  string
			st    FileState
			mtime int64
		)
		if err := rows.Scan(&path, &st.ID, &mtime, &st.Size); err != nil {
			return nil, fmt.Errorf("read index states: %w", err)
		}
		st.ModTime = time.Unix(0, mtime)
		states[path] = st
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read index states: %w", err)
	}
	return states, nil
}

// Count возвращает число заметок в индексе.
func (ix *Index) Count(ctx context.Context) (int, error) {
	var n int
	if err := ix.db.QueryRowContext(ctx, `SELECT count(*) FROM notes`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count notes: %w", err)
	}
	return n, nil
}

// GetByPath читает заметку по пути относительно корня vault.
//
// Нужен событиям: watcher знает пути, а интерфейсу нужны идентификаторы.
func (ix *Index) GetByPath(ctx context.Context, path string) (Record, error) {
	var id string
	err := ix.db.QueryRowContext(ctx, `SELECT id FROM notes WHERE path = ?`, path).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, fmt.Errorf("get note by path %s: %w", path, ErrNotFound)
	}
	if err != nil {
		return Record{}, fmt.Errorf("get note by path %s: %w", path, err)
	}
	return ix.Get(ctx, id)
}
