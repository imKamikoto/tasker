package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Сквозной тест поверх настоящих vault и индекса: он же и проверка того, что
// слои стыкуются без фронтенда.
func exec(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errBuf bytes.Buffer
	err = run(args, &out, &errBuf)
	return out.String(), errBuf.String(), err
}

func vaultWithNotes(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("Работа/Баги/schetchik.md", "---\nid: 01K3QF8ZN7X2WPBV4YHMC6TDA1\ntitle: Счётчик перерасчёта\n"+
		"created: 2026-08-01T10:00:00+03:00\nupdated: 2026-08-01T10:00:00+03:00\n"+
		"status: active\ntags: [баг, armz]\npinned: true\n---\n"+
		"Счётчик не пересчитывается после ручной правки.\n\n- [x] найти\n- [ ] починить\n")
	write("Личное/pokupki.md", "---\nid: 01K3QF8ZN7X2WPBV4YHMC6TDA2\ntitle: Покупки\n"+
		"created: 2026-08-01T10:00:00+03:00\nupdated: 2026-08-01T10:00:00+03:00\n---\n"+
		"Купить чего-нибудь к чаю.\n")
	return root
}

func TestRunScanAndSearch(t *testing.T) {
	root := vaultWithNotes(t)

	out, _, err := exec(t, root)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "+2") {
		t.Errorf("не видно двух добавленных заметок:\n%s", out)
	}

	// Индекс должен оказаться в служебном каталоге vault (SPEC §4.1).
	if _, err := os.Stat(filepath.Join(root, ".tasker", "index.sqlite")); err != nil {
		t.Errorf("индекс не создан: %v", err)
	}

	out, _, err = exec(t, root, "перерасч")
	if err != nil {
		t.Fatalf("run с запросом: %v", err)
	}
	if !strings.Contains(out, "найдено 1") {
		t.Errorf("запрос ничего не нашёл:\n%s", out)
	}
	for _, want := range []string{"Счётчик перерасчёта", "Работа/Баги/schetchik.md", "active", "#баг", "1/2", "★"} {
		if !strings.Contains(out, want) {
			t.Errorf("в выдаче нет %q:\n%s", want, out)
		}
	}
}

// Повторный запуск не должен ничего переиндексировать.
func TestRunSecondScanIsQuiet(t *testing.T) {
	root := vaultWithNotes(t)
	if _, _, err := exec(t, root); err != nil {
		t.Fatal(err)
	}

	out, _, err := exec(t, root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "+0 ~0 -0") || !strings.Contains(out, "без изменений 2") {
		t.Errorf("повторный скан не пуст:\n%s", out)
	}
}

// Части языка запросов доезжают через CLI, а не только через пакет.
func TestRunQueryForms(t *testing.T) {
	root := vaultWithNotes(t)
	if _, _, err := exec(t, root); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		query string
		want  string
	}{
		{"tag:баг", "найдено 1"},
		{"book:Работа", "найдено 1"},
		{"status:active", "найдено 1"},
		{"is:pinned", "найдено 1"},
		{"has:task", "найдено 1"},
		{`"ручной правки"`, "найдено 1"},
		{"чаю -tag:баг", "найдено 1"},
		{"tag:несуществующий", "найдено 0"},
	}
	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			out, _, err := exec(t, root, "-no-scan", c.query)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("ожидалось %q:\n%s", c.want, out)
			}
		})
	}
}

// Несколько слов запроса приходят отдельными аргументами, если их не закавычить
// в шелле, — склеивать их обязан сам CLI.
func TestRunJoinsQueryArgs(t *testing.T) {
	root := vaultWithNotes(t)
	out, _, err := exec(t, root, "счётчик", "tag:баг")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "найдено 1") {
		t.Errorf("запрос из двух аргументов не сработал:\n%s", out)
	}
}

func TestRunReportsBackfill(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "снаружи.md"), []byte("просто текст\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _, err := exec(t, root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "дописан frontmatter в 1 файл") {
		t.Errorf("правка на диске не названа:\n%s", out)
	}
	raw, err := os.ReadFile(filepath.Join(root, "снаружи.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(raw), "---\nid: ") {
		t.Errorf("файл не дописан:\n%s", raw)
	}
}

func TestRunReportsBrokenNotes(t *testing.T) {
	root := vaultWithNotes(t)
	if err := os.WriteFile(filepath.Join(root, "битая.md"),
		[]byte("---\ntitle: A\n  сдвинуто: сломано\n\tтаб: нельзя\n---\nтело\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _, err := exec(t, root)
	if err != nil {
		t.Fatalf("одна битая заметка уронила весь запуск: %v", err)
	}
	if !strings.Contains(out, "не прочитано 1") || !strings.Contains(out, "битая.md") {
		t.Errorf("битая заметка не названа:\n%s", out)
	}
	if !strings.Contains(out, "+2") {
		t.Errorf("остальные заметки не проиндексированы:\n%s", out)
	}
}

// Опечатку в запросе надо показать сразу, а не после минуты индексации.
func TestRunRejectsBadQueryBeforeScanning(t *testing.T) {
	root := vaultWithNotes(t)

	out, _, err := exec(t, root, "status:почтиГотово")
	if err == nil {
		t.Fatal("ожидалась ошибка разбора запроса")
	}
	if out != "" {
		t.Errorf("до проверки запроса что-то напечаталось:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(root, ".tasker")); !os.IsNotExist(err) {
		t.Error("индекс создан, хотя запрос не разобрался")
	}
}

func TestRunUsageErrors(t *testing.T) {
	t.Run("без аргументов", func(t *testing.T) {
		_, errOut, err := exec(t)
		if err == nil {
			t.Fatal("ожидалась ошибка")
		}
		if !strings.Contains(errOut, "usage:") {
			t.Errorf("не показана подсказка:\n%s", errOut)
		}
	})

	t.Run("несуществующая папка", func(t *testing.T) {
		_, _, err := exec(t, filepath.Join(t.TempDir(), "нет"))
		if err == nil {
			t.Fatal("ожидалась ошибка")
		}
	})
}

func TestRunLimit(t *testing.T) {
	root := vaultWithNotes(t)
	out, _, err := exec(t, "-limit", "1", root, "")
	if err != nil {
		t.Fatal(err)
	}
	// Пустой запрос печатает только отчёт скана.
	if strings.Contains(out, "найдено") {
		t.Errorf("пустой запрос не должен искать:\n%s", out)
	}

	out, _, err = exec(t, "-limit", "1", "-no-scan", root, "book:Работа book:Личное")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "найдено 0") {
		t.Errorf("два разных ноутбука через И должны дать пусто:\n%s", out)
	}
}

func TestPlural(t *testing.T) {
	cases := map[int]string{1: "файл", 2: "файла", 4: "файла", 5: "файлов",
		11: "файлов", 12: "файлов", 14: "файлов", 21: "файл", 22: "файла", 111: "файлов", 101: "файл"}
	for n, want := range cases {
		if got := plural(n, "файл", "файла", "файлов"); got != want {
			t.Errorf("plural(%d) = %q, ожидалось %q", n, got, want)
		}
	}
}
