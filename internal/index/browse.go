package index

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Notebook — ноутбук и то, что о нём знает индекс.
type Notebook struct {
	// Path — путь относительно корня vault. Пустая строка — сам корень.
	Path string
	// Count — заметок непосредственно в этом ноутбуке, без вложенных.
	// Свёрнутый ноутбук показывает сумму с детьми, и считает её интерфейс
	// (SPEC §8.1).
	Count int
	// Children — пути дочерних ноутбуков, на один уровень вниз.
	Children []string
}

// Tag — тег и число заметок с ним.
type Tag struct {
	Name  string
	Count int
	Color string
}

// Notebooks возвращает дерево ноутбуков, отсортированное по пути.
//
// Промежуточные ноутбуки попадают в список, даже если своих заметок в них нет:
// «Личное» существует, если существует «Личное/Покупки». Корзина ноутбуком не
// считается — её заметки помечены как удалённые и сюда не попадают.
func (ix *Index) Notebooks(ctx context.Context) ([]Notebook, error) {
	rows, err := ix.db.QueryContext(ctx,
		`SELECT notebook, count(*) FROM notes WHERE trashed = 0 GROUP BY notebook`)
	if err != nil {
		return nil, fmt.Errorf("list notebooks: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var (
			nb string
			n  int
		)
		if err := rows.Scan(&nb, &n); err != nil {
			return nil, fmt.Errorf("list notebooks: %w", err)
		}
		counts[nb] = n
		// Достраиваем предков: у них может не быть своих заметок, но сами они
		// есть, и без них дерево разваливается.
		for parent := parentOf(nb); parent != ""; parent = parentOf(parent) {
			if _, ok := counts[parent]; !ok {
				counts[parent] = 0
			}
		}
		if nb != "" {
			if _, ok := counts[""]; !ok {
				counts[""] = 0
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notebooks: %w", err)
	}
	if len(counts) == 0 {
		return nil, nil
	}

	children := make(map[string][]string, len(counts))
	for nb := range counts {
		if nb == "" {
			continue
		}
		parent := parentOf(nb)
		children[parent] = append(children[parent], nb)
	}

	books := make([]Notebook, 0, len(counts))
	for nb, count := range counts {
		kids := children[nb]
		sort.Strings(kids)
		books = append(books, Notebook{Path: nb, Count: count, Children: kids})
	}
	sort.Slice(books, func(i, j int) bool { return books[i].Path < books[j].Path })
	return books, nil
}

// parentOf возвращает родительский ноутбук. Для верхнего уровня — корень.
func parentOf(notebook string) string {
	i := strings.LastIndex(notebook, "/")
	if i < 0 {
		return ""
	}
	return notebook[:i]
}

// Tags возвращает теги с числом живых заметок, отсортированные по имени.
//
// Тег, оставшийся только у удалённых заметок, из списка не пропадает: имя и
// цвет переживают удаление, иначе восстановление заметки теряло бы цвет.
func (ix *Index) Tags(ctx context.Context) ([]Tag, error) {
	rows, err := ix.db.QueryContext(ctx, `
		SELECT t.name, t.color, count(n.id)
		FROM tags t
		LEFT JOIN note_tags nt ON nt.tag = t.name
		LEFT JOIN notes n ON n.id = nt.note_id AND n.trashed = 0
		GROUP BY t.name, t.color
		ORDER BY t.name`)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var tag Tag
		if err := rows.Scan(&tag.Name, &tag.Color, &tag.Count); err != nil {
			return nil, fmt.Errorf("list tags: %w", err)
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	return tags, nil
}

// Backlinks возвращает заметки, которые ссылаются на указанную.
//
// Это и есть панель бэклинков под редактором (SPEC §8.9): один индекс по
// links.dst, никакого обхода тела заметок.
func (ix *Index) Backlinks(ctx context.Context, id string) ([]Record, error) {
	rows, err := ix.db.QueryContext(ctx, `
		SELECT n.id, n.path, n.notebook, n.title, n.status, n.pinned,
		       n.created, n.updated, n.mtime, n.size, n.num_tasks, n.num_done,
		       n.excerpt, n.trashed
		FROM notes n
		JOIN links l ON l.src = n.id
		WHERE l.dst = ? AND n.trashed = 0
		ORDER BY n.pinned DESC, n.updated DESC, n.rowid DESC`, id)
	if err != nil {
		return nil, fmt.Errorf("backlinks of %s: %w", id, err)
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
			return nil, fmt.Errorf("backlinks of %s: %w", id, err)
		}
		r.Created = time.Unix(created, 0)
		r.Updated = time.Unix(updated, 0)
		r.ModTime = time.Unix(0, mtime)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("backlinks of %s: %w", id, err)
	}
	if err := ix.attachTags(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}
