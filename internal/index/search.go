package index

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SearchOptions — то, что не относится к самому запросу.
type SearchOptions struct {
	// Limit ограничивает выдачу. Ноль — без ограничения.
	Limit int
	// IncludeTrashed добавляет к выдаче корзину. По умолчанию её нет: удалённое
	// не должно всплывать в обычном поиске.
	IncludeTrashed bool

	// HideCompleted убирает завершённое и брошенное. Так выглядит список
	// ноутбука по умолчанию (SPEC §8.3).
	//
	// Отдельным флагом, а не через язык запросов: там условия соединяются
	// только через И, а запрос из одних отрицаний он справедливо отвергает.
	HideCompleted bool
}

// Search выполняет разобранный запрос.
//
// Тело заметок не возвращается: таблица полнотекстового индекса contentless,
// содержимое в ней не хранится, а читать файлы ради списка незачем.
func (ix *Index) Search(ctx context.Context, q Query, opts SearchOptions) ([]Record, error) {
	where, args, err := q.conditions()
	if err != nil {
		return nil, err
	}
	if !opts.IncludeTrashed {
		where = append(where, "n.trashed = 0")
	}
	if opts.HideCompleted {
		where = append(where, "n.status NOT IN ('completed', 'dropped')")
	}

	sqlText := `
		SELECT n.id, n.path, n.notebook, n.title, n.status, n.pinned,
		       n.created, n.updated, n.mtime, n.size, n.num_tasks, n.num_done,
		       n.excerpt, n.trashed
		FROM notes n`
	if len(where) > 0 {
		sqlText += "\n\t\tWHERE " + strings.Join(where, "\n\t\t  AND ")
	}
	// Закреплённые сверху, дальше по дате изменения по убыванию. rowid в конце —
	// чтобы порядок не плавал у заметок с одинаковой датой.
	sqlText += "\n\t\tORDER BY n.pinned DESC, n.updated DESC, n.rowid DESC"
	if opts.Limit > 0 {
		sqlText += "\n\t\tLIMIT ?"
		args = append(args, opts.Limit)
	}

	rows, err := ix.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()

	var out []Record
	for rows.Next() {
		var (
			r                       Record
			created, updated, mtime int64
		)
		if err := rows.Scan(&r.ID, &r.Path, &r.Notebook, &r.Title, &r.Status, &r.Pinned,
			&created, &updated, &mtime, &r.Size, &r.NumTasks, &r.NumDone,
			&r.Excerpt, &r.Trashed); err != nil {
			return nil, fmt.Errorf("search: %w", err)
		}
		r.Created = time.Unix(created, 0)
		r.Updated = time.Unix(updated, 0)
		r.ModTime = time.Unix(0, mtime)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	if err := ix.attachTags(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// attachTags доливает теги одним запросом на всю выдачу, а не по запросу на
// заметку.
func (ix *Index) attachTags(ctx context.Context, records []Record) error {
	if len(records) == 0 {
		return nil
	}

	byID := make(map[string]*Record, len(records))
	args := make([]any, 0, len(records))
	placeholders := make([]string, 0, len(records))
	for i := range records {
		byID[records[i].ID] = &records[i]
		args = append(args, records[i].ID)
		placeholders = append(placeholders, "?")
	}

	rows, err := ix.db.QueryContext(ctx,
		`SELECT note_id, tag FROM note_tags WHERE note_id IN (`+
			strings.Join(placeholders, ",")+`) ORDER BY note_id, tag`, args...)
	if err != nil {
		return fmt.Errorf("search tags: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, tag string
		if err := rows.Scan(&id, &tag); err != nil {
			return fmt.Errorf("search tags: %w", err)
		}
		if r, ok := byID[id]; ok {
			r.Tags = append(r.Tags, tag)
		}
	}
	return rows.Err()
}

// conditions превращает запрос в условия WHERE и аргументы к ним.
//
// Положительные текстовые условия собираются в одно выражение MATCH: FTS5
// умеет соединять их сам, и это один проход по индексу вместо нескольких.
// Отрицания вынесены в отдельные NOT IN — смешивать их с MATCH в одном
// выражении можно, но читается это потом плохо.
func (q Query) conditions() ([]string, []any, error) {
	var (
		clauses []string
		args    []any
		match   []string
	)

	for _, t := range q.Terms {
		switch t.Kind {
		case TermText:
			expr := ftsExpr(t.Field, t.Value)
			if !t.Negated {
				match = append(match, expr)
				continue
			}
			clauses = append(clauses,
				"n.rowid NOT IN (SELECT rowid FROM notes_fts WHERE notes_fts MATCH ?)")
			args = append(args, expr)

		case TermNotebook:
			// Ноутбук со всеми вложенными. Сравнение с добавленным слэшем, а не
			// просто по префиксу строки: иначе book:Работа зацепит «Работа-старая».
			clauses = append(clauses,
				negate(t, `(n.notebook = ? OR n.notebook LIKE ? ESCAPE '\')`))
			args = append(args, t.Value, escapeLike(t.Value)+`/%`)

		case TermTag:
			clauses = append(clauses,
				negate(t, `EXISTS (SELECT 1 FROM note_tags nt WHERE nt.note_id = n.id AND nt.tag = ?)`))
			args = append(args, t.Value)

		case TermStatus:
			clauses = append(clauses, negate(t, `n.status = ?`))
			args = append(args, t.Value)

		case TermPinned:
			clauses = append(clauses, negate(t, `n.pinned = 1`))

		case TermTask:
			// «Есть незакрытые чекбоксы» (SPEC §8.5), а не «есть чекбоксы вообще».
			clauses = append(clauses, negate(t, `n.num_tasks > n.num_done`))

		default:
			return nil, nil, fmt.Errorf("compile query: unknown term kind %d", t.Kind)
		}
	}

	if len(match) > 0 {
		clauses = append([]string{"n.rowid IN (SELECT rowid FROM notes_fts WHERE notes_fts MATCH ?)"}, clauses...)
		args = append([]any{strings.Join(match, " AND ")}, args...)
	}
	return clauses, args, nil
}

func negate(t Term, clause string) string {
	if t.Negated {
		return "NOT " + clause
	}
	return clause
}

// ftsExpr готовит выражение для MATCH. Значение всегда идёт фразой в кавычках:
// у токенизатора trigram это ровно поиск подстроки, а заодно снимает вопрос,
// что делать со словами вроде AND и NOT внутри запроса.
func ftsExpr(field, value string) string {
	phrase := `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	if field == "" {
		return phrase
	}
	return field + ":" + phrase
}

// escapeLike обезвреживает символы шаблона: ноутбук может называться «100%».
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func escapeLike(s string) string { return likeEscaper.Replace(s) }
