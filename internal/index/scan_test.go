package index

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tasker/internal/vault"
)

func testScan(t *testing.T) (*Index, *vault.Vault, string) {
	t.Helper()
	root := t.TempDir()
	v, err := vault.Open(root)
	if err != nil {
		t.Fatalf("vault.Open: %v", err)
	}
	ix, _ := testIndex(t)
	return ix, v, root
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func note(title, body string) string {
	return "---\nid: " + vault.NewID() + "\ntitle: " + title + "\n---\n" + body
}

func TestScanEmpty(t *testing.T) {
	ix, v, _ := testScan(t)
	res, err := ix.Scan(context.Background(), v)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if res.Added+res.Updated+res.Removed+res.Unchanged != 0 {
		t.Errorf("результат = %+v, ожидался пустой", res)
	}
}

func TestScanAddsNotes(t *testing.T) {
	ix, v, root := testScan(t)
	ctx := context.Background()

	writeFile(t, root, "корневая.md", note("Корневая", "тело\n"))
	writeFile(t, root, "Работа/Баги/bug.md", note("Баг", "описание\n"))
	writeFile(t, root, "Работа/задача.md", note("Задача", "текст\n"))

	res, err := ix.Scan(ctx, v)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if res.Added != 3 || res.Updated != 0 || res.Removed != 0 {
		t.Errorf("результат = %+v, ожидалось 3 добавленных", res)
	}
	if n, _ := ix.Count(ctx); n != 3 {
		t.Errorf("в индексе %d заметок", n)
	}

	states, err := ix.States(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"корневая.md", "Работа/Баги/bug.md", "Работа/задача.md"} {
		if _, ok := states[want]; !ok {
			t.Errorf("нет пути %q в индексе: %v", want, keys(states))
		}
	}
}

// Второй скан по нетронутому vault не должен делать ничего: на этом стоит
// скорость старта (SPEC §5.2).
func TestScanIncremental(t *testing.T) {
	ix, v, root := testScan(t)
	ctx := context.Background()
	writeFile(t, root, "a.md", note("A", "тело\n"))
	writeFile(t, root, "b.md", note("B", "тело\n"))

	if _, err := ix.Scan(ctx, v); err != nil {
		t.Fatal(err)
	}
	res, err := ix.Scan(ctx, v)
	if err != nil {
		t.Fatalf("повторный Scan: %v", err)
	}
	if res.Unchanged != 2 || res.Added != 0 || res.Updated != 0 || res.Removed != 0 {
		t.Errorf("результат = %+v, ожидалось 2 неизменённых", res)
	}
}

func TestScanUpdatesChanged(t *testing.T) {
	ix, v, root := testScan(t)
	ctx := context.Background()
	writeFile(t, root, "a.md", note("A", "старое тело\n"))
	if _, err := ix.Scan(ctx, v); err != nil {
		t.Fatal(err)
	}

	states, _ := ix.States(ctx)
	id := states["a.md"].ID

	writeFile(t, root, "a.md", "---\nid: "+id+"\ntitle: A\n---\nсовсем другое тело подлиннее\n")
	res, err := ix.Scan(ctx, v)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if res.Updated != 1 || res.Added != 0 {
		t.Errorf("результат = %+v, ожидалось 1 обновление", res)
	}

	r, err := ix.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if r.Excerpt != "совсем другое тело подлиннее" {
		t.Errorf("превью = %q — строка не перечитана", r.Excerpt)
	}
}

func TestScanRemovesGone(t *testing.T) {
	ix, v, root := testScan(t)
	ctx := context.Background()
	writeFile(t, root, "a.md", note("A", "тело\n"))
	writeFile(t, root, "b.md", note("B", "тело\n"))
	if _, err := ix.Scan(ctx, v); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(root, "b.md")); err != nil {
		t.Fatal(err)
	}
	res, err := ix.Scan(ctx, v)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if res.Removed != 1 || res.Unchanged != 1 {
		t.Errorf("результат = %+v, ожидалось 1 удаление и 1 неизменённая", res)
	}
	if n, _ := ix.Count(ctx); n != 1 {
		t.Errorf("в индексе %d заметок", n)
	}
}

// Переезд файла — это не «удалили и завели новую»: id тот же, и удалять
// заметку нельзя, иначе оборвутся ссылки на неё.
func TestScanHandlesMove(t *testing.T) {
	ix, v, root := testScan(t)
	ctx := context.Background()
	writeFile(t, root, "a.md", note("A", "тело\n"))
	if _, err := ix.Scan(ctx, v); err != nil {
		t.Fatal(err)
	}
	states, _ := ix.States(ctx)
	id := states["a.md"].ID

	if err := os.MkdirAll(filepath.Join(root, "Работа"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "a.md"), filepath.Join(root, "Работа", "a.md")); err != nil {
		t.Fatal(err)
	}

	res, err := ix.Scan(ctx, v)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if res.Removed != 0 {
		t.Errorf("результат = %+v — переезд посчитан удалением", res)
	}
	if n, _ := ix.Count(ctx); n != 1 {
		t.Errorf("в индексе %d заметок", n)
	}
	r, err := ix.Get(ctx, id)
	if err != nil {
		t.Fatalf("заметка потеряна после переезда: %v", err)
	}
	if r.Path != "Работа/a.md" || r.Notebook != "Работа" {
		t.Errorf("путь = %q, ноутбук = %q", r.Path, r.Notebook)
	}
}

func TestScanSkipsHiddenAndNonMarkdown(t *testing.T) {
	ix, v, root := testScan(t)
	ctx := context.Background()

	writeFile(t, root, "видимая.md", note("Видимая", "тело\n"))
	writeFile(t, root, ".tasker/служебная.md", note("Служебная", "тело\n"))
	writeFile(t, root, ".git/объект.md", note("Гит", "тело\n"))
	writeFile(t, root, "Работа/.скрытая/внутри.md", note("Скрытая", "тело\n"))
	writeFile(t, root, ".спрятанная.md", note("Точка", "тело\n"))
	writeFile(t, root, "заметки.txt", "просто текст")
	writeFile(t, root, "attachments/2026/08/картинка.png", "PNG")

	res, err := ix.Scan(ctx, v)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if res.Added != 1 {
		t.Errorf("добавлено %d, ожидалась одна видимая заметка (%+v)", res.Added, res)
	}
}

// Корзина — единственный скрытый каталог, который индексируется (SPEC §4.3).
func TestScanIndexesTrash(t *testing.T) {
	ix, v, root := testScan(t)
	ctx := context.Background()
	writeFile(t, root, "живая.md", note("Живая", "тело\n"))
	writeFile(t, root, ".trash/удалённая.md", note("Удалённая", "тело\n"))

	if _, err := ix.Scan(ctx, v); err != nil {
		t.Fatal(err)
	}
	states, _ := ix.States(ctx)
	if len(states) != 2 {
		t.Fatalf("в индексе %d путей: %v", len(states), keys(states))
	}

	r, err := ix.Get(ctx, states[".trash/удалённая.md"].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Trashed {
		t.Error("заметка из .trash не помечена как удалённая")
	}
	live, err := ix.Get(ctx, states["живая.md"].ID)
	if err != nil {
		t.Fatal(err)
	}
	if live.Trashed {
		t.Error("живая заметка помечена как удалённая")
	}
}

// Одна заметка со сломанным заголовком не должна валить весь скан.
func TestScanReportsBrokenNotes(t *testing.T) {
	ix, v, root := testScan(t)
	ctx := context.Background()
	writeFile(t, root, "хорошая.md", note("Хорошая", "тело\n"))
	writeFile(t, root, "битая.md", "---\ntitle: A\n  сдвинуто: сломано\n\tтаб: сюда нельзя\n---\nтело\n")

	res, err := ix.Scan(ctx, v)
	if err != nil {
		t.Fatalf("Scan вернул ошибку вместо отчёта: %v", err)
	}
	if res.Added != 1 {
		t.Errorf("добавлено %d, ожидалась одна", res.Added)
	}
	if len(res.Failed) != 1 {
		t.Fatalf("сбоев %d, ожидался один: %+v", len(res.Failed), res.Failed)
	}
	if res.Failed[0].Path != "битая.md" || res.Failed[0].Err == nil {
		t.Errorf("сбой = %+v", res.Failed[0])
	}
}

func TestScanFillsRecordFields(t *testing.T) {
	ix, v, root := testScan(t)
	ctx := context.Background()

	id := vault.NewID()
	writeFile(t, root, "Работа/задача.md",
		"---\nid: "+id+"\ntitle: Задача\nstatus: active\ntags: [работа, баг]\npinned: true\n"+
			"created: 2026-08-01T10:00:00+03:00\n---\n"+
			"Описание.\n\n- [x] сделано\n- [ ] нет\n\nсм. [другая](tasker://note/01K3QF8ZN7X2WPBV4YHMC6TDAF)\n")

	if _, err := ix.Scan(ctx, v); err != nil {
		t.Fatal(err)
	}
	r, err := ix.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if r.Notebook != "Работа" || r.Path != "Работа/задача.md" {
		t.Errorf("путь = %q, ноутбук = %q", r.Path, r.Notebook)
	}
	if r.Status != "active" || !r.Pinned {
		t.Errorf("статус = %q, закрепление = %v", r.Status, r.Pinned)
	}
	if r.NumTasks != 2 || r.NumDone != 1 {
		t.Errorf("чеклист = %d/%d", r.NumDone, r.NumTasks)
	}
	if len(r.Tags) != 2 {
		t.Errorf("теги = %v", r.Tags)
	}
	if len(r.Links) != 1 || r.Links[0] != "01K3QF8ZN7X2WPBV4YHMC6TDAF" {
		t.Errorf("ссылки = %v", r.Links)
	}
	want := time.Date(2026, 8, 1, 10, 0, 0, 0, time.FixedZone("", 3*3600))
	if !r.Created.Equal(want) {
		t.Errorf("created = %v, ожидалось %v", r.Created, want)
	}
}

// Заметка снаружи без frontmatter: скан дописывает недостающее прямо в файл и
// индексирует её как обычную. SPEC §4.1 — «достраивается при первом открытии»,
// и скан это открытие и есть.
func TestScanBackfillsNotesWithoutID(t *testing.T) {
	ix, v, root := testScan(t)
	ctx := context.Background()
	writeFile(t, root, "мои-мысли.md", "# Просто файл\n\nбез заголовка\n")
	writeFile(t, root, "нормальная.md", note("Нормальная", "тело\n"))

	res, err := ix.Scan(ctx, v)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if res.Added != 2 || len(res.Failed) != 0 {
		t.Fatalf("результат = %+v, ожидалось две добавленных без сбоев", res)
	}

	raw, err := os.ReadFile(filepath.Join(root, "мои-мысли.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(raw), "---\nid: ") {
		t.Errorf("frontmatter не дописан в файл:\n%s", raw)
	}
	if !strings.HasSuffix(string(raw), "# Просто файл\n\nбез заголовка\n") {
		t.Errorf("тело потеряно:\n%s", raw)
	}

	states, _ := ix.States(ctx)
	st, ok := states["мои-мысли.md"]
	if !ok {
		t.Fatalf("заметки нет в индексе: %v", keys(states))
	}
	r, err := ix.Get(ctx, st.ID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Title != "мои-мысли" {
		t.Errorf("title = %q, ожидалось имя файла", r.Title)
	}

	// И следующий скан не должен считать её изменившейся: stat обновлён.
	res2, err := ix.Scan(ctx, v)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Unchanged != 2 || res2.Updated != 0 {
		t.Errorf("повторный скан = %+v, ожидалось 2 неизменённых", res2)
	}
}

// А вот негодный id, который кто-то уже проставил, трогать нельзя: неизвестно,
// чем он был и кто на него ссылается.
func TestScanReportsBadID(t *testing.T) {
	ix, v, root := testScan(t)
	ctx := context.Background()
	writeFile(t, root, "чужая.md", "---\nid: не-ulid\ntitle: A\n---\nтело\n")

	res, err := ix.Scan(ctx, v)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if res.Added != 0 {
		t.Errorf("добавлено %d, ожидалось 0", res.Added)
	}
	if len(res.Failed) != 1 || !errors.Is(res.Failed[0].Err, ErrMissingID) {
		t.Fatalf("сбои = %+v, ожидался один ErrMissingID", res.Failed)
	}

	raw, _ := os.ReadFile(filepath.Join(root, "чужая.md"))
	if !strings.Contains(string(raw), "id: не-ulid") {
		t.Errorf("чужой id переписан:\n%s", raw)
	}
}

// Копия файла заметки даёт два одинаковых id. Индексировать оба нельзя: id —
// уникальный ключ, и строки начали бы затирать друг друга от скана к скану.
func TestScanReportsDuplicateID(t *testing.T) {
	ix, v, root := testScan(t)
	ctx := context.Background()
	content := note("Одна и та же", "тело\n")
	writeFile(t, root, "оригинал.md", content)
	writeFile(t, root, "копия.md", content)

	res, err := ix.Scan(ctx, v)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if res.Added != 1 {
		t.Errorf("добавлено %d, ожидалась одна", res.Added)
	}
	if len(res.Failed) != 1 || !errors.Is(res.Failed[0].Err, ErrDuplicateID) {
		t.Errorf("сбои = %+v, ожидался один ErrDuplicateID", res.Failed)
	}
}

func keys(m map[string]FileState) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Правка, не изменившая длину файла: по размеру такая заметка выглядит
// нетронутой, и без сверки mtime скан бы её пропустил.
func TestScanDetectsSameSizeEdit(t *testing.T) {
	ix, v, root := testScan(t)
	ctx := context.Background()

	id := vault.NewID()
	const before, after = "telo-AAA", "telo-BBB" // ровно одна длина в байтах
	writeFile(t, root, "a.md", "---\nid: "+id+"\ntitle: A\n---\n"+before+"\n")
	if _, err := ix.Scan(ctx, v); err != nil {
		t.Fatal(err)
	}

	// Та же длина, другое содержимое, заведомо другой mtime.
	if len(before) != len(after) {
		t.Fatalf("тест бессмыслен: длины %d и %d различаются", len(before), len(after))
	}
	writeFile(t, root, "a.md", "---\nid: "+id+"\ntitle: A\n---\n"+after+"\n")
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(filepath.Join(root, "a.md"), later, later); err != nil {
		t.Fatal(err)
	}

	res, err := ix.Scan(ctx, v)
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 1 || res.Unchanged != 0 {
		t.Fatalf("результат = %+v, ожидалось 1 обновление", res)
	}
	r, err := ix.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if r.Excerpt != after {
		t.Errorf("превью = %q — строка не перечитана", r.Excerpt)
	}
}

// Файл, положенный в vault снаружи, может не иметь created и updated. Брать их
// неоткуда, кроме mtime (SPEC §4.2), иначе заметка уедет в 1970 год.
func TestScanFallsBackToModTime(t *testing.T) {
	ix, v, root := testScan(t)
	ctx := context.Background()

	id := vault.NewID()
	writeFile(t, root, "снаружи.md", "---\nid: "+id+"\ntitle: Без дат\n---\nтело\n")
	moment := time.Date(2026, 5, 4, 12, 30, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(root, "снаружи.md"), moment, moment); err != nil {
		t.Fatal(err)
	}

	if _, err := ix.Scan(ctx, v); err != nil {
		t.Fatal(err)
	}
	r, err := ix.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Created.Equal(moment) {
		t.Errorf("created = %v, ожидался mtime %v", r.Created, moment)
	}
	if !r.Updated.Equal(moment) {
		t.Errorf("updated = %v, ожидался mtime %v", r.Updated, moment)
	}
}

func TestScanErrorWrapsCause(t *testing.T) {
	e := ScanError{Path: "Работа/битая.md", Err: ErrMissingID}

	if !errors.Is(e, ErrMissingID) {
		t.Error("errors.Is не достаёт причину через ScanError")
	}
	msg := e.Error()
	if !strings.Contains(msg, "Работа/битая.md") || !strings.Contains(msg, "no valid id") {
		t.Errorf("текст ошибки = %q: в нём должны быть и путь, и причина", msg)
	}
}
