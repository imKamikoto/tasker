package index

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func testIndex(t *testing.T) (*Index, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "index.sqlite")
	ix, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { ix.Close() })
	return ix, path
}

func sampleRecord() Record {
	moment := time.Date(2026, 8, 19, 15, 0, 0, 0, time.UTC)
	return Record{
		ID:       "01K3QF8ZN7X2WPBV4YHMC6TDAE",
		Path:     "Работа/Баги/schetchik.md",
		Notebook: "Работа/Баги",
		Title:    "Счётчик перерасчёта не обновляется",
		Status:   "active",
		Pinned:   true,
		Created:  moment,
		Updated:  moment,
		ModTime:  moment,
		Size:     512,
		NumTasks: 3,
		NumDone:  1,
		Excerpt:  "Описание бага",
		Tags:     []string{"работа", "баг"},
		Links:    []string{"01K3QF8ZN7X2WPBV4YHMC6TDAF"},
		Body:     "Счётчик не пересчитывается после ручной правки значения",
	}
}

func TestOpenCreatesSchema(t *testing.T) {
	ix, _ := testIndex(t)
	ctx := context.Background()

	for _, pragma := range []struct{ q, want string }{
		{"journal_mode", "wal"},
		{"busy_timeout", "5000"},
		{"foreign_keys", "1"},
	} {
		var got string
		if err := ix.db.QueryRowContext(ctx, "PRAGMA "+pragma.q).Scan(&got); err != nil {
			t.Fatalf("PRAGMA %s: %v", pragma.q, err)
		}
		if got != pragma.want {
			t.Errorf("PRAGMA %s = %q, ожидалось %q", pragma.q, got, pragma.want)
		}
	}

	for _, table := range []string{"notes", "tags", "note_tags", "links", "notes_fts", "meta"} {
		var name string
		err := ix.db.QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE name = ?", table).Scan(&name)
		if err != nil {
			t.Errorf("таблицы %s нет: %v", table, err)
		}
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "index.sqlite")

	ix, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ix.Put(ctx, sampleRecord()); err != nil {
		t.Fatal(err)
	}
	if err := ix.Close(); err != nil {
		t.Fatal(err)
	}

	ix2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("повторное Open: %v", err)
	}
	defer ix2.Close()

	n, err := ix2.Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("после переоткрытия заметок %d, ожидалась 1", n)
	}
}

// SPEC §5.2: не совпала версия схемы — индекс сносится и строится заново.
// Это всегда безопасно, правда в файлах.
func TestOpenRebuildsOnSchemaMismatch(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "index.sqlite")

	ix, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ix.Put(ctx, sampleRecord()); err != nil {
		t.Fatal(err)
	}
	if _, err := ix.db.ExecContext(ctx,
		"UPDATE meta SET value = ? WHERE key = 'schema_version'", "999"); err != nil {
		t.Fatal(err)
	}
	ix.Close()

	ix2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open после смены версии: %v", err)
	}
	defer ix2.Close()

	n, err := ix2.Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("заметок %d — индекс не пересобран", n)
	}

	var version string
	if err := ix2.db.QueryRowContext(ctx,
		"SELECT value FROM meta WHERE key = 'schema_version'").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Errorf("schema_version = %q, ожидалось %q", version, schemaVersion)
	}
}

func TestPutAndGet(t *testing.T) {
	ix, _ := testIndex(t)
	ctx := context.Background()
	want := sampleRecord()

	if err := ix.Put(ctx, want); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := ix.Get(ctx, want.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.ID != want.ID || got.Path != want.Path || got.Notebook != want.Notebook {
		t.Errorf("путь/ноутбук: %+v", got)
	}
	if got.Title != want.Title || got.Status != want.Status || !got.Pinned {
		t.Errorf("заголовок/статус/закрепление: %+v", got)
	}
	if got.NumTasks != 3 || got.NumDone != 1 || got.Size != 512 {
		t.Errorf("счётчики: %+v", got)
	}
	if !got.Created.Equal(want.Created) || !got.ModTime.Equal(want.ModTime) {
		t.Errorf("времена: created=%v mtime=%v", got.Created, got.ModTime)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "баг" || got.Tags[1] != "работа" {
		t.Errorf("теги = %v, ожидались отсортированные [баг работа]", got.Tags)
	}
	if len(got.Links) != 1 || got.Links[0] != "01K3QF8ZN7X2WPBV4YHMC6TDAF" {
		t.Errorf("ссылки = %v", got.Links)
	}
}

func TestGetMissing(t *testing.T) {
	ix, _ := testIndex(t)
	if _, err := ix.Get(context.Background(), "01K3QF8ZN7X2WPBV4YHMC6TDAE"); !errors.Is(err, ErrNotFound) {
		t.Errorf("ошибка = %v, ожидалась ErrNotFound", err)
	}
}

// Повторный Put по тому же id обновляет строку, а не плодит вторую.
func TestPutUpdatesInPlace(t *testing.T) {
	ix, _ := testIndex(t)
	ctx := context.Background()

	r := sampleRecord()
	if err := ix.Put(ctx, r); err != nil {
		t.Fatal(err)
	}

	r.Title = "Новый заголовок"
	r.Path = "Личное/perenesli.md"
	r.Notebook = "Личное"
	r.Tags = []string{"личное"}
	r.Links = nil
	r.Body = "совершенно другой текст"
	if err := ix.Put(ctx, r); err != nil {
		t.Fatalf("повторный Put: %v", err)
	}

	if n, _ := ix.Count(ctx); n != 1 {
		t.Errorf("заметок %d, ожидалась 1", n)
	}
	got, err := ix.Get(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Новый заголовок" || got.Path != "Личное/perenesli.md" {
		t.Errorf("строка не обновилась: %+v", got)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "личное" {
		t.Errorf("теги = %v — старые не отцепились", got.Tags)
	}
	if len(got.Links) != 0 {
		t.Errorf("ссылки = %v — старые не удалились", got.Links)
	}
}

// Полнотекстовый поиск: подстрока из середины слова, ради чего и взят trigram.
func TestFullTextSearch(t *testing.T) {
	ix, _ := testIndex(t)
	ctx := context.Background()
	if err := ix.Put(ctx, sampleRecord()); err != nil {
		t.Fatal(err)
	}

	for _, q := range []string{`"ерерасч"`, `"счётчик"`, `"СЧЁТЧИК"`, `title:"обновляется"`, `"ручной правки"`} {
		var n int
		if err := ix.db.QueryRowContext(ctx,
			"SELECT count(*) FROM notes_fts WHERE notes_fts MATCH ?", q).Scan(&n); err != nil {
			t.Fatalf("MATCH %s: %v", q, err)
		}
		if n != 1 {
			t.Errorf("MATCH %s нашёл %d, ожидалась 1", q, n)
		}
	}
}

// Без contentless_delete=1 это не работает: обновление FTS-строки требует
// удаления старой, а старого содержимого у нас уже нет.
func TestFullTextUpdatedOnPut(t *testing.T) {
	ix, _ := testIndex(t)
	ctx := context.Background()

	r := sampleRecord()
	if err := ix.Put(ctx, r); err != nil {
		t.Fatal(err)
	}
	r.Body = "совершенно другое содержимое про миграции"
	r.Title = "Другой заголовок"
	if err := ix.Put(ctx, r); err != nil {
		t.Fatal(err)
	}

	count := func(q string) int {
		var n int
		if err := ix.db.QueryRowContext(ctx,
			"SELECT count(*) FROM notes_fts WHERE notes_fts MATCH ?", q).Scan(&n); err != nil {
			t.Fatalf("MATCH %s: %v", q, err)
		}
		return n
	}
	if n := count(`"ерерасч"`); n != 0 {
		t.Errorf("старый текст всё ещё находится (%d совпадений)", n)
	}
	if n := count(`"миграц"`); n != 1 {
		t.Errorf("новый текст не находится (%d совпадений)", n)
	}
}

func TestDelete(t *testing.T) {
	ix, _ := testIndex(t)
	ctx := context.Background()
	r := sampleRecord()
	if err := ix.Put(ctx, r); err != nil {
		t.Fatal(err)
	}

	if err := ix.Delete(ctx, r.Path); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if n, _ := ix.Count(ctx); n != 0 {
		t.Errorf("заметок %d после удаления", n)
	}

	// Связи должны уйти вместе со строкой, иначе индекс копит мусор.
	var n int
	if err := ix.db.QueryRowContext(ctx, "SELECT count(*) FROM note_tags").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("note_tags = %d — каскад не сработал", n)
	}
	if err := ix.db.QueryRowContext(ctx, "SELECT count(*) FROM links").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("links = %d", n)
	}
	if err := ix.db.QueryRowContext(ctx, "SELECT count(*) FROM notes_fts").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("notes_fts = %d — строка полнотекстового индекса осталась", n)
	}
}

func TestDeleteMissingIsNotAnError(t *testing.T) {
	ix, _ := testIndex(t)
	if err := ix.Delete(context.Background(), "нет/такой.md"); err != nil {
		t.Errorf("Delete отсутствующего: %v", err)
	}
}

// States — то, на чём стоит инкрементальный скан (SPEC §5.2).
func TestStates(t *testing.T) {
	ix, _ := testIndex(t)
	ctx := context.Background()
	r := sampleRecord()
	if err := ix.Put(ctx, r); err != nil {
		t.Fatal(err)
	}

	states, err := ix.States(ctx)
	if err != nil {
		t.Fatalf("States: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("состояний %d, ожидалось 1", len(states))
	}
	st, ok := states[r.Path]
	if !ok {
		t.Fatalf("нет состояния для %q: %v", r.Path, states)
	}
	if st.ID != r.ID || st.Size != r.Size || !st.ModTime.Equal(r.ModTime) {
		t.Errorf("состояние = %+v, ожидалось id=%s size=%d mtime=%v", st, r.ID, r.Size, r.ModTime)
	}
}

// Путь уникален: если файл переехал, а старая строка ещё висит, вставка не
// должна падать на UNIQUE.
func TestPutReclaimsPathFromAnotherNote(t *testing.T) {
	ix, _ := testIndex(t)
	ctx := context.Background()

	first := sampleRecord()
	if err := ix.Put(ctx, first); err != nil {
		t.Fatal(err)
	}

	second := sampleRecord()
	second.ID = "01K3QF8ZN7X2WPBV4YHMC6TDAF"
	second.Title = "Другая заметка на том же пути"
	if err := ix.Put(ctx, second); err != nil {
		t.Fatalf("Put на занятый путь: %v", err)
	}

	if n, _ := ix.Count(ctx); n != 1 {
		t.Errorf("заметок %d, ожидалась 1", n)
	}
	if _, err := ix.Get(ctx, first.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("старая заметка осталась: %v", err)
	}
}

func TestEmptyTagsAndLinks(t *testing.T) {
	ix, _ := testIndex(t)
	ctx := context.Background()
	r := sampleRecord()
	r.Tags = nil
	r.Links = nil
	if err := ix.Put(ctx, r); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := ix.Get(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tags) != 0 || len(got.Links) != 0 {
		t.Errorf("теги = %v, ссылки = %v", got.Tags, got.Links)
	}
}

var _ = sql.ErrNoRows
