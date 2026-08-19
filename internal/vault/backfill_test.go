package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mtime, который заведомо отличается от «сейчас»: даты должны браться из файла,
// а не из часов.
func agedNote(t *testing.T, root, rel, content string) time.Time {
	t.Helper()
	writeNote(t, filepath.Join(root, rel), content)
	moment := time.Date(2026, 5, 4, 12, 30, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(root, rel), moment, moment); err != nil {
		t.Fatal(err)
	}
	return moment
}

func TestBackfillPlainMarkdown(t *testing.T) {
	v, root := testVault(t)
	moment := agedNote(t, root, "мои-мысли.md", "# Просто файл\n\nбез заголовка\n")

	n, err := v.Load("мои-мысли.md")
	if err != nil {
		t.Fatal(err)
	}
	changed, err := v.Backfill(n)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if !changed {
		t.Fatal("Backfill вернул false для файла без frontmatter")
	}

	if !ValidID(n.Doc.Meta.ID()) {
		t.Errorf("id = %q — не ULID", n.Doc.Meta.ID())
	}
	title, err := n.Doc.Meta.Title()
	if err != nil || title != "мои-мысли" {
		t.Errorf("title = %q, %v — ожидалось имя файла без расширения", title, err)
	}
	created, err := n.Doc.Meta.Created()
	if err != nil || !created.Equal(moment) {
		t.Errorf("created = %v, %v — ожидался mtime %v", created, err, moment)
	}
	updated, err := n.Doc.Meta.Updated()
	if err != nil || !updated.Equal(moment) {
		t.Errorf("updated = %v, ожидался mtime %v", updated, moment)
	}

	// Порядок ключей — как в SPEC §4.2.
	keys := n.Doc.Meta.Keys()
	want := []string{"id", "title", "created", "updated"}
	if len(keys) != len(want) {
		t.Fatalf("ключи = %v, ожидались %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Errorf("ключ %d = %q, ожидался %q", i, keys[i], want[i])
		}
	}

	// И всё это должно оказаться на диске, а не только в памяти.
	raw, err := os.ReadFile(n.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(raw), "---\nid: ") {
		t.Errorf("файл не переписан:\n%s", raw)
	}
	if !strings.HasSuffix(string(raw), "# Просто файл\n\nбез заголовка\n") {
		t.Errorf("тело потеряно:\n%s", raw)
	}
}

// Дописываем только то, чего нет. Чужие поля и уже проставленные значения
// трогать нельзя.
func TestBackfillFillsOnlyMissing(t *testing.T) {
	v, root := testVault(t)
	agedNote(t, root, "note.md",
		"---\ntitle: Настоящий заголовок\nчужое_поле: не трогать\n# комментарий\n---\nтело\n")

	n, err := v.Load("note.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Backfill(n); err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	title, _ := n.Doc.Meta.Title()
	if title != "Настоящий заголовок" {
		t.Errorf("title = %q — существующий заголовок перезаписан", title)
	}
	raw, _ := os.ReadFile(n.Path)
	if !strings.Contains(string(raw), "чужое_поле: не трогать") {
		t.Errorf("чужое поле потеряно:\n%s", raw)
	}
	if !strings.Contains(string(raw), "# комментарий") {
		t.Errorf("комментарий потерян:\n%s", raw)
	}
	if !ValidID(n.Doc.Meta.ID()) {
		t.Errorf("id не дописан: %q", n.Doc.Meta.ID())
	}
}

// Заметка, у которой всё на месте, не должна переписываться: это лишний коммит
// и лишнее событие watcher'а.
func TestBackfillNoopOnCompleteNote(t *testing.T) {
	v, _ := testVault(t)
	created, err := v.Create(NewNote{Title: "Полная заметка", Body: "тело\n"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(created.Path)
	if err != nil {
		t.Fatal(err)
	}

	n, err := v.Load(created.Path)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := v.Backfill(n)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if changed {
		t.Error("Backfill сообщил об изменениях в полной заметке")
	}
	after, _ := os.Stat(created.Path)
	if !before.ModTime().Equal(after.ModTime()) {
		t.Error("файл перезаписан без нужды")
	}
}

// Повторный вызов после первого ничего не делает: id уже есть.
func TestBackfillIsIdempotent(t *testing.T) {
	v, root := testVault(t)
	agedNote(t, root, "note.md", "тело без заголовка\n")

	n, err := v.Load("note.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Backfill(n); err != nil {
		t.Fatal(err)
	}
	firstID := n.Doc.Meta.ID()

	n2, err := v.Load("note.md")
	if err != nil {
		t.Fatal(err)
	}
	changed, err := v.Backfill(n2)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("повторный Backfill переписал заметку")
	}
	if n2.Doc.Meta.ID() != firstID {
		t.Errorf("id сменился: %q → %q — ссылки бы оборвались", firstID, n2.Doc.Meta.ID())
	}
}

// Существующий id не заменяется, даже если он не похож на ULID: перебить его
// значит оборвать ссылки, а мы не знаем, чем он был.
func TestBackfillKeepsExistingID(t *testing.T) {
	v, root := testVault(t)
	agedNote(t, root, "note.md", "---\nid: не-ulid-но-чей-то\ntitle: A\n---\nтело\n")

	n, err := v.Load("note.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Backfill(n); err != nil {
		t.Fatal(err)
	}
	if got := n.Doc.Meta.ID(); got != "не-ulid-но-чей-то" {
		t.Errorf("id = %q — существующий заменён", got)
	}
}

func TestBackfillEmptyTitleIsReplaced(t *testing.T) {
	v, root := testVault(t)
	agedNote(t, root, "имя-файла.md", "---\ntitle: \"   \"\n---\nтело\n")

	n, err := v.Load("имя-файла.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Backfill(n); err != nil {
		t.Fatal(err)
	}
	title, _ := n.Doc.Meta.Title()
	if title != "имя-файла" {
		t.Errorf("title = %q, ожидалось имя файла", title)
	}
}

// После записи заметка должна знать свои новые mtime и размер, иначе
// следующий скан посчитает её изменившейся и перечитает зря.
func TestBackfillRefreshesStat(t *testing.T) {
	v, root := testVault(t)
	agedNote(t, root, "note.md", "тело\n")

	n, err := v.Load("note.md")
	if err != nil {
		t.Fatal(err)
	}
	oldMod, oldSize := n.ModTime, n.Size
	if _, err := v.Backfill(n); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(n.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !n.ModTime.Equal(info.ModTime()) || n.Size != info.Size() {
		t.Errorf("stat не обновлён: %v/%d против %v/%d на диске",
			n.ModTime, n.Size, info.ModTime(), info.Size())
	}
	if n.ModTime.Equal(oldMod) && n.Size == oldSize {
		t.Error("stat не изменился, хотя файл переписан")
	}
	if n.Doc.Modified() {
		t.Error("документ остался помеченным как изменённый")
	}
}
