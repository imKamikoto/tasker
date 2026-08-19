package index

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"tasker/internal/vault"
)

// maxExcerptLen — сколько рун превью хранится в индексе. Список показывает две
// строки текста (SPEC §8.4), с запасом на любую ширину колонки.
const maxExcerptLen = 200

// trashDir — каталог корзины. Единственный скрытый каталог, который vault не
// игнорирует (SPEC §4.1, §4.3).
const trashDir = ".trash"

// Record — то, что индекс хранит про заметку.
type Record struct {
	ID       string
	Path     string // относительно корня vault, всегда со слэшами
	Notebook string
	Title    string
	Status   string
	Pinned   bool
	Created  time.Time
	Updated  time.Time
	ModTime  time.Time
	Size     int64
	NumTasks int
	NumDone  int
	Excerpt  string
	Trashed  bool
	Tags     []string
	Links    []string

	// Body уходит в полнотекстовый индекс и обратно не читается: таблица
	// contentless, содержимое в ней не хранится.
	Body string
}

// RecordFrom превращает прочитанную заметку в строку индекса.
func RecordFrom(n *vault.Note) (Record, error) {
	f := n.Doc.Meta

	title, err := f.Title()
	if err != nil {
		return Record{}, fmt.Errorf("record from %s: %w", n.Path, err)
	}
	status, err := f.Status()
	if err != nil {
		return Record{}, fmt.Errorf("record from %s: %w", n.Path, err)
	}
	pinned, err := f.Pinned()
	if err != nil {
		return Record{}, fmt.Errorf("record from %s: %w", n.Path, err)
	}
	tags, err := f.Tags()
	if err != nil {
		return Record{}, fmt.Errorf("record from %s: %w", n.Path, err)
	}
	created, err := f.Created()
	if err != nil {
		return Record{}, fmt.Errorf("record from %s: %w", n.Path, err)
	}
	updated, err := f.Updated()
	if err != nil {
		return Record{}, fmt.Errorf("record from %s: %w", n.Path, err)
	}

	// Времени в заголовке может не быть — файл мог прийти в vault снаружи.
	// Тогда берём mtime (SPEC §4.2).
	if created.IsZero() {
		created = n.ModTime
	}
	if updated.IsZero() {
		updated = n.ModTime
	}

	rel := path.Join(n.Notebook, filepath.Base(n.Path))
	total, done := CountTasks(n.Doc.Body)

	return Record{
		ID:       f.ID(),
		Path:     rel,
		Notebook: n.Notebook,
		Title:    title,
		Status:   string(status),
		Pinned:   pinned,
		Created:  created,
		Updated:  updated,
		ModTime:  n.ModTime,
		Size:     n.Size,
		NumTasks: total,
		NumDone:  done,
		Excerpt:  Excerpt(n.Doc.Body),
		Trashed:  inTrash(n.Notebook),
		Tags:     tags,
		Links:    ExtractLinks(n.Doc.Body),
		Body:     n.Doc.Body,
	}, nil
}

func inTrash(notebook string) bool {
	return notebook == trashDir || strings.HasPrefix(notebook, trashDir+"/")
}

var (
	reFence   = regexp.MustCompile("^\\s*```")
	reMarkers = regexp.MustCompile(`^\s*(?:#{1,6}\s+|>\s?|[-*+]\s+(?:\[[ xX]\]\s+)?|\d+\.\s+)`)
	reSpaces  = regexp.MustCompile(`\s+`)
	reTask    = regexp.MustCompile(`(?m)^[ \t]*[-*+][ \t]+\[([ xX])\]`)
	reNoteURL = regexp.MustCompile(`tasker://note/([0-9A-Z]{26})`)
)

// Excerpt делает из тела заметки одну строку для списка: снимает разметку в
// начале строк, схлопывает пробелы и режет по длине.
//
// Это не рендер markdown, а именно превью: разбирать тело целиком ради двух
// строк в списке было бы расточительно, а разница на глаз незаметна.
func Excerpt(body string) string {
	var parts []string
	for _, line := range strings.Split(body, "\n") {
		if reFence.MatchString(line) {
			continue
		}
		for {
			stripped := reMarkers.ReplaceAllString(line, "")
			if stripped == line {
				break
			}
			line = stripped
		}
		if line = strings.TrimSpace(line); line != "" {
			parts = append(parts, line)
		}
	}

	out := reSpaces.ReplaceAllString(strings.Join(parts, " "), " ")
	out = strings.TrimSpace(out)

	runes := []rune(out)
	if len(runes) > maxExcerptLen {
		out = strings.TrimSpace(string(runes[:maxExcerptLen]))
	}
	return out
}

// CountTasks считает чекбоксы GFM: сколько всего и сколько отмечено.
func CountTasks(body string) (total, done int) {
	for _, m := range reTask.FindAllStringSubmatch(body, -1) {
		total++
		if m[1] != " " {
			done++
		}
	}
	return total, done
}

// ExtractLinks собирает id заметок, на которые ссылается тело. Порядок первого
// появления сохраняется, дубли схлопываются.
func ExtractLinks(body string) []string {
	var out []string
	seen := make(map[string]struct{})
	for _, m := range reNoteURL.FindAllStringSubmatch(body, -1) {
		id := m[1]
		if !vault.ValidID(id) {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
