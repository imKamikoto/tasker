package notes

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tasker/internal/vault"
)

// writeTemplate кладёт файл шаблона прямо на диск: так его и заводят руками.
func writeTemplate(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, "templates")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExpandPlaceholders(t *testing.T) {
	now := time.Date(2026, 8, 24, 15, 4, 5, 0, time.UTC)
	id := func() string { return "01M0K9V866QABD4VZ8PKH9WPJM" }

	cases := []struct {
		in   string
		want string
	}{
		{"дата {{date}}", "дата 2026-08-24"},
		// Формат Go, а не strftime: второго языка форматов здесь нет.
		{"{{date:02.01.2006}}", "24.08.2026"},
		{"{{time}}", "15:04"},
		{"{{time:15:04:05}}", "15:04:05"},
		{"{{uuid}}", "01M0K9V866QABD4VZ8PKH9WPJM"},
		// Незнакомое имя остаётся текстом: шаблон правит человек, и молча
		// съедать непонятое — худший из вариантов.
		{"{{unknown}}", "{{unknown}}"},
		{"без плейсхолдеров", "без плейсхолдеров"},
	}
	for _, c := range cases {
		got, _ := expandPlaceholders(c.in, now, id)
		if got != c.want {
			t.Errorf("expandPlaceholders(%q) = %q, ожидалось %q", c.in, got, c.want)
		}
	}
}

func TestExpandPlaceholdersCursor(t *testing.T) {
	now := time.Date(2026, 8, 24, 15, 4, 5, 0, time.UTC)
	id := func() string { return "X" }

	body, cursor := expandPlaceholders("начало{{cursor}}конец", now, id)
	if body != "началоконец" {
		t.Errorf("тело %q — метка каретки осталась в тексте", body)
	}
	if cursor != len("начало") {
		t.Errorf("каретка на %d, ожидалось %d", cursor, len("начало"))
	}

	// Каретка одна: из двух одинаковых меток выбирать нечем, побеждает первая.
	_, first := expandPlaceholders("а{{cursor}}б{{cursor}}в", now, id)
	if first != len("а") {
		t.Errorf("каретка на %d, ожидалась первая метка", first)
	}

	// Нет метки — нет и позиции.
	if _, none := expandPlaceholders("текст", now, id); none != -1 {
		t.Errorf("каретка %d, ожидалось -1", none)
	}

	// Метка после раскрытой даты считается по уже раскрытому тексту.
	body, after := expandPlaceholders("{{date}} {{cursor}}", now, id)
	if after != len("2026-08-24 ") {
		t.Errorf("каретка на %d в %q", after, body)
	}
}

func TestTemplatesList(t *testing.T) {
	s, root := testService(t, vault.OriginUser)
	ctx := context.Background()

	writeTemplate(t, root, "bug.md", "---\n_template:\n  title: \"Баг: \"\n---\n\nЧто сломалось\n")
	writeTemplate(t, root, "daily.md", "---\n---\n\nПлан на день\n")
	// Не markdown — не шаблон.
	writeTemplate(t, root, "readme.txt", "не шаблон")

	got, err := s.Templates(ctx)
	if err != nil {
		t.Fatalf("Templates: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("шаблонов %d: %+v", len(got), got)
	}
	if got[0].Name != "bug" || got[1].Name != "daily" {
		t.Errorf("порядок: %+v", got)
	}
	if got[0].Title != "Баг: " {
		t.Errorf("заголовок из _template: %q", got[0].Title)
	}
	if got[1].Preview != "План на день" {
		t.Errorf("превью: %q", got[1].Preview)
	}
}

// Папки нет — это не ошибка: шаблоны заводит человек, и до тех пор их просто ноль.
func TestTemplatesWithoutFolder(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	got, err := s.Templates(context.Background())
	if err != nil {
		t.Fatalf("Templates: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("шаблонов %d", len(got))
	}
}

func TestApplyTemplate(t *testing.T) {
	s, root := testService(t, vault.OriginUser)
	ctx := context.Background()

	writeTemplate(t, root, "bug.md", strings.Join([]string{
		"---",
		"_template:",
		"  title: Ошибка в парсере",
		"  notebook: Работа/Баги",
		"  tags: [баг]",
		"  status: active",
		"---",
		"",
		"## Что сломалось",
		"{{cursor}}",
		"",
		"Заведено {{date}}",
	}, "\n"))

	created, err := s.Create(ctx, CreateParams{Title: "Новая заметка"})
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.ApplyTemplate(ctx, created.ID, "templates/bug.md")
	if err != nil {
		t.Fatalf("ApplyTemplate: %v", err)
	}

	if got.Record.Title != "Ошибка в парсере" {
		t.Errorf("заголовок %q", got.Record.Title)
	}
	if got.Record.Notebook != "Работа/Баги" {
		t.Errorf("ноутбук %q", got.Record.Notebook)
	}
	if got.Record.Status != "active" {
		t.Errorf("статус %q", got.Record.Status)
	}
	if len(got.Record.Tags) != 1 || got.Record.Tags[0] != "баг" {
		t.Errorf("теги %v", got.Record.Tags)
	}
	// Имя файла следует за заголовком и после шаблона тоже.
	if filepath.Base(got.Record.Path) != "oshibka-v-parsere.md" {
		t.Errorf("путь %q", got.Record.Path)
	}
	if got.Cursor < 0 {
		t.Error("шаблон задавал каретку, а позиции нет")
	}

	full, err := s.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(full.Body, "## Что сломалось") {
		t.Errorf("тело не подставилось:\n%s", full.Body)
	}
	if strings.Contains(full.Body, "{{") {
		t.Errorf("плейсхолдеры остались:\n%s", full.Body)
	}
}

// Проставленное человеком шаблон не затирает (SPEC §8.10).
func TestApplyTemplateKeepsChosenStatusAndTags(t *testing.T) {
	s, root := testService(t, vault.OriginUser)
	ctx := context.Background()

	writeTemplate(t, root, "bug.md", "---\n_template:\n  tags: [баг]\n  status: active\n---\n\nтело\n")

	created, err := s.Create(ctx, CreateParams{
		Title:  "Уже начатая",
		Tags:   []string{"срочно"},
		Status: vault.StatusOnHold,
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.ApplyTemplate(ctx, created.ID, "templates/bug.md")
	if err != nil {
		t.Fatalf("ApplyTemplate: %v", err)
	}
	if got.Record.Status != "onHold" {
		t.Errorf("статус %q — шаблон затёр выбранный", got.Record.Status)
	}
	if len(got.Record.Tags) != 2 {
		t.Errorf("теги %v — ожидались оба", got.Record.Tags)
	}
}
