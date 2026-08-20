package vault

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixedClock делает время в тестах предсказуемым: иначе updated нечем проверить.
func testVault(t *testing.T) (*Vault, string) {
	t.Helper()
	root := t.TempDir()
	v, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	moment := time.Date(2026, 8, 19, 15, 0, 0, 0, time.FixedZone("", 3*3600))
	v.now = func() time.Time { return moment }
	return v, root
}

func TestOpen(t *testing.T) {
	t.Run("существующий каталог", func(t *testing.T) {
		dir := t.TempDir()
		v, err := Open(dir)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		if !filepath.IsAbs(v.Root()) {
			t.Errorf("Root() = %q, ожидался абсолютный путь", v.Root())
		}
		// На macOS t.TempDir() лежит под /var, который сам симлинк на /private/var.
		// Корень обязан быть уже разрешённым, иначе все проверки вложенности врут.
		resolved, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatal(err)
		}
		if v.Root() != resolved {
			t.Errorf("Root() = %q, ожидалось %q", v.Root(), resolved)
		}
	})

	t.Run("каталога нет", func(t *testing.T) {
		if _, err := Open(filepath.Join(t.TempDir(), "нет")); err == nil {
			t.Error("ожидалась ошибка")
		}
	})

	t.Run("файл вместо каталога", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(path); err == nil {
			t.Error("ожидалась ошибка")
		}
	})
}

// Ключевая проверка из MCP.md §4: выход за пределы vault ловится по реальному
// пути, а не по префиксу строки.
func TestVaultPathContainment(t *testing.T) {
	v, root := testVault(t)
	writeNote(t, filepath.Join(root, "note.md"), "---\ntitle: A\n---\nтело\n")

	t.Run("путь внутри", func(t *testing.T) {
		if _, err := v.Load("note.md"); err != nil {
			t.Errorf("Load: %v", err)
		}
		if _, err := v.Load(filepath.Join(root, "note.md")); err != nil {
			t.Errorf("Load по абсолютному пути: %v", err)
		}
	})

	t.Run("выход через ..", func(t *testing.T) {
		_, err := v.Load("../снаружи.md")
		if !errors.Is(err, ErrOutsideVault) {
			t.Errorf("ошибка = %v, ожидалась ErrOutsideVault", err)
		}
	})

	t.Run("абсолютный путь снаружи", func(t *testing.T) {
		outside := filepath.Join(t.TempDir(), "чужая.md")
		writeNote(t, outside, "---\ntitle: B\n---\n")
		_, err := v.Load(outside)
		if !errors.Is(err, ErrOutsideVault) {
			t.Errorf("ошибка = %v, ожидалась ErrOutsideVault", err)
		}
	})

	// Симлинк из vault наружу — ровно тот случай, ради которого нужен
	// EvalSymlinks: по строке путь выглядит вложенным, по факту нет.
	t.Run("симлинк наружу", func(t *testing.T) {
		outsideDir := t.TempDir()
		writeNote(t, filepath.Join(outsideDir, "секрет.md"), "---\ntitle: C\n---\n")
		if err := os.Symlink(outsideDir, filepath.Join(root, "ссылка")); err != nil {
			t.Skipf("симлинки недоступны: %v", err)
		}
		_, err := v.Load("ссылка/секрет.md")
		if !errors.Is(err, ErrOutsideVault) {
			t.Errorf("ошибка = %v, ожидалась ErrOutsideVault — симлинк открыл весь диск", err)
		}
	})

	// Ловушка обычного strings.HasPrefix: соседний каталог с общим префиксом.
	t.Run("сосед с общим префиксом", func(t *testing.T) {
		sibling := v.Root() + "-evil"
		if err := os.MkdirAll(sibling, 0o755); err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(sibling)
		writeNote(t, filepath.Join(sibling, "чужая.md"), "---\ntitle: D\n---\n")

		_, err := v.Load(filepath.Join(sibling, "чужая.md"))
		if !errors.Is(err, ErrOutsideVault) {
			t.Errorf("ошибка = %v, ожидалась ErrOutsideVault", err)
		}
	})
}

func TestVaultLoad(t *testing.T) {
	v, root := testVault(t)
	writeNote(t, filepath.Join(root, "Работа", "Баги", "note.md"),
		"---\nid: 01K3QF8ZN7X2WPBV4YHMC6TDAE\ntitle: Заголовок\n---\nтело заметки\n")

	n, err := v.Load("Работа/Баги/note.md")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if n.Notebook != "Работа/Баги" {
		t.Errorf("Notebook = %q", n.Notebook)
	}
	if n.Doc.Body != "тело заметки\n" {
		t.Errorf("Body = %q", n.Doc.Body)
	}
	if n.Doc.Meta.ID() != "01K3QF8ZN7X2WPBV4YHMC6TDAE" {
		t.Errorf("ID = %q", n.Doc.Meta.ID())
	}
	if n.Size == 0 || n.ModTime.IsZero() {
		t.Errorf("Size = %d, ModTime = %v — нужны индексу для инкрементального скана", n.Size, n.ModTime)
	}

	t.Run("заметка в корне", func(t *testing.T) {
		writeNote(t, filepath.Join(root, "корневая.md"), "---\ntitle: A\n---\n")
		n, err := v.Load("корневая.md")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if n.Notebook != "" {
			t.Errorf("Notebook = %q, ожидалась пустая строка", n.Notebook)
		}
	})

	t.Run("заметки нет", func(t *testing.T) {
		_, err := v.Load("нет-такой.md")
		if !errors.Is(err, ErrNoteNotFound) {
			t.Errorf("ошибка = %v, ожидалась ErrNoteNotFound", err)
		}
	})
}

func TestVaultSave(t *testing.T) {
	t.Run("неизменённая заметка не перезаписывается", func(t *testing.T) {
		v, root := testVault(t)
		path := filepath.Join(root, "note.md")
		writeNote(t, path, "---\ntitle: A\nupdated: 2026-01-01T00:00:00+03:00\n---\nтело\n")

		before, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		n, err := v.Load("note.md")
		if err != nil {
			t.Fatal(err)
		}
		if err := v.Save(n); err != nil {
			t.Fatalf("Save: %v", err)
		}

		after, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !before.ModTime().Equal(after.ModTime()) {
			t.Error("файл перезаписан без изменений: лишний коммит, лишнее событие watcher'а")
		}
		got, _ := os.ReadFile(path)
		if !strings.Contains(string(got), "updated: 2026-01-01T00:00:00+03:00") {
			t.Errorf("updated изменился на пустом месте:\n%s", got)
		}
	})

	t.Run("изменённая пишется и обновляет updated", func(t *testing.T) {
		v, root := testVault(t)
		path := filepath.Join(root, "note.md")
		writeNote(t, path, "---\ntitle: Старый\nчужое: не трогать\nupdated: 2026-01-01T00:00:00+03:00\n---\nтело\n")

		n, err := v.Load("note.md")
		if err != nil {
			t.Fatal(err)
		}
		if err := n.Doc.Meta.SetTitle("Новый"); err != nil {
			t.Fatal(err)
		}
		if err := v.Save(n); err != nil {
			t.Fatalf("Save: %v", err)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		want := "---\ntitle: Новый\nчужое: не трогать\nupdated: 2026-08-19T15:00:00+03:00\n---\nтело\n"
		if string(got) != want {
			t.Errorf("--- ожидалось ---\n%s\n--- получено ---\n%s", want, got)
		}
	})

	t.Run("правка тела тоже обновляет updated", func(t *testing.T) {
		v, root := testVault(t)
		writeNote(t, filepath.Join(root, "note.md"), "---\ntitle: A\n---\nстарое тело\n")

		n, err := v.Load("note.md")
		if err != nil {
			t.Fatal(err)
		}
		n.Doc.Body = "новое тело\n"
		if err := v.Save(n); err != nil {
			t.Fatalf("Save: %v", err)
		}

		got, _ := os.ReadFile(filepath.Join(root, "note.md"))
		if !strings.Contains(string(got), "новое тело") {
			t.Errorf("тело не сохранено:\n%s", got)
		}
		if !strings.Contains(string(got), "updated: 2026-08-19T15:00:00+03:00") {
			t.Errorf("updated не проставлен:\n%s", got)
		}
	})

	t.Run("после сохранения заметка снова чистая", func(t *testing.T) {
		v, root := testVault(t)
		writeNote(t, filepath.Join(root, "note.md"), "---\ntitle: A\n---\nтело\n")

		n, err := v.Load("note.md")
		if err != nil {
			t.Fatal(err)
		}
		if err := n.Doc.Meta.SetTitle("B"); err != nil {
			t.Fatal(err)
		}
		if err := v.Save(n); err != nil {
			t.Fatal(err)
		}
		if n.Doc.Modified() {
			t.Error("документ остался помеченным как изменённый")
		}

		before, _ := os.Stat(filepath.Join(root, "note.md"))
		if err := v.Save(n); err != nil {
			t.Fatal(err)
		}
		after, _ := os.Stat(filepath.Join(root, "note.md"))
		if !before.ModTime().Equal(after.ModTime()) {
			t.Error("повторный Save перезаписал файл")
		}
	})
}

func TestVaultCreate(t *testing.T) {
	t.Run("порядок ключей из SPEC §4.2", func(t *testing.T) {
		v, _ := testVault(t)
		n, err := v.Create(NewNote{
			Title:    "Счётчик перерасчёта не обновляется",
			Body:     "Описание бага.\n",
			Notebook: "Работа/Баги",
			Tags:     []string{"работа", "баг"},
			Status:   StatusActive,
			Origin:   OriginAgent,
			Context:  &Context{Repo: "tasker", Branch: "main", Commit: "3f9a1c2"},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		wantKeys := []string{"id", "title", "created", "updated", "status", "tags", "pinned", "origin", "context"}
		got := n.Doc.Meta.Keys()
		if len(got) != len(wantKeys) {
			t.Fatalf("ключи = %v, ожидались %v", got, wantKeys)
		}
		for i := range wantKeys {
			if got[i] != wantKeys[i] {
				t.Errorf("ключ %d = %q, ожидался %q (порядок = чистые git-диффы)", i, got[i], wantKeys[i])
			}
		}

		raw, err := os.ReadFile(n.Path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "tags: [работа, баг]") {
			t.Errorf("теги не в строку:\n%s", raw)
		}
		if !strings.Contains(string(raw), "created: 2026-08-19T15:00:00+03:00") {
			t.Errorf("created не по SPEC:\n%s", raw)
		}
		if !strings.HasSuffix(string(raw), "Описание бага.\n") {
			t.Errorf("тело не записано:\n%s", raw)
		}
	})

	t.Run("имя файла из slug, ноутбук создан", func(t *testing.T) {
		v, root := testVault(t)
		n, err := v.Create(NewNote{Title: "Ломается экранирование кавычек", Notebook: "Работа/Баги"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		want := filepath.Join(root, "Работа", "Баги", "lomaetsya-ekranirovanie-kavychek.md")
		if resolved, _ := filepath.EvalSymlinks(want); n.Path != resolved && n.Path != want {
			t.Errorf("путь = %q, ожидался %q", n.Path, want)
		}
		if n.Notebook != "Работа/Баги" {
			t.Errorf("Notebook = %q", n.Notebook)
		}
		if _, err := os.Stat(n.Path); err != nil {
			t.Errorf("файла нет: %v", err)
		}
	})

	t.Run("id — валидный ULID, статус по умолчанию none", func(t *testing.T) {
		v, _ := testVault(t)
		n, err := v.Create(NewNote{Title: "Заметка"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if !ValidID(n.Doc.Meta.ID()) {
			t.Errorf("id = %q — не ULID", n.Doc.Meta.ID())
		}
		status, err := n.Doc.Meta.Status()
		if err != nil || status != StatusNone {
			t.Errorf("status = %q, %v — ожидался none", status, err)
		}
		origin, err := n.Doc.Meta.Origin()
		if err != nil || origin != OriginUser {
			t.Errorf("origin = %q, %v — ожидался user", origin, err)
		}
	})

	t.Run("коллизия имён", func(t *testing.T) {
		v, _ := testVault(t)
		var paths []string
		for i := 0; i < 3; i++ {
			n, err := v.Create(NewNote{Title: "Одинаковый заголовок"})
			if err != nil {
				t.Fatalf("Create %d: %v", i, err)
			}
			paths = append(paths, filepath.Base(n.Path))
		}
		want := []string{"odinakovyy-zagolovok.md", "odinakovyy-zagolovok-2.md", "odinakovyy-zagolovok-3.md"}
		for i := range want {
			if paths[i] != want[i] {
				t.Errorf("имя %d = %q, ожидалось %q", i, paths[i], want[i])
			}
		}
	})

	t.Run("заголовок без пригодных символов — имя из id", func(t *testing.T) {
		v, _ := testVault(t)
		n, err := v.Create(NewNote{Title: "🎉🎉🎉"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		want := strings.ToLower(n.Doc.Meta.ID()) + ".md"
		if got := filepath.Base(n.Path); got != want {
			t.Errorf("имя = %q, ожидалось %q", got, want)
		}
	})

	t.Run("пустой заголовок отвергается", func(t *testing.T) {
		v, _ := testVault(t)
		if _, err := v.Create(NewNote{Title: "  "}); err == nil {
			t.Error("ожидалась ошибка: заголовок — единственный источник имени заметки")
		}
	})

	t.Run("ноутбук вне vault", func(t *testing.T) {
		v, _ := testVault(t)
		for _, nb := range []string{"../снаружи", "/etc", "Работа/../../снаружи"} {
			if _, err := v.Create(NewNote{Title: "Заметка", Notebook: nb}); !errors.Is(err, ErrOutsideVault) {
				t.Errorf("ноутбук %q: ошибка = %v, ожидалась ErrOutsideVault", nb, err)
			}
		}
	})

	t.Run("скрытый ноутбук отвергается", func(t *testing.T) {
		v, _ := testVault(t)
		for _, nb := range []string{".tasker", ".git", "Работа/.скрытый"} {
			if _, err := v.Create(NewNote{Title: "Заметка", Notebook: nb}); err == nil {
				t.Errorf("ноутбук %q принят, а скрытые каталоги vault игнорирует (SPEC §4.1)", nb)
			}
		}
	})

	t.Run("созданная заметка читается обратно", func(t *testing.T) {
		v, _ := testVault(t)
		created, err := v.Create(NewNote{
			Title:    "Заголовок: с двоеточием",
			Notebook: "Работа",
			Tags:     []string{"тег"},
			Status:   StatusOnHold,
			Pinned:   true,
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		loaded, err := v.Load(created.Path)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		title, err := loaded.Doc.Meta.Title()
		if err != nil || title != "Заголовок: с двоеточием" {
			t.Errorf("title = %q, %v", title, err)
		}
		if pinned, err := loaded.Doc.Meta.Pinned(); err != nil || !pinned {
			t.Errorf("pinned = %v, %v", pinned, err)
		}
		if status, err := loaded.Doc.Meta.Status(); err != nil || status != StatusOnHold {
			t.Errorf("status = %q, %v", status, err)
		}
	})
}

func writeNote(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Симлинк внутри vault, ведущий наружу: MkdirAll прошёл бы сквозь него и создал
// каталог на чужой территории, поэтому проверка обязана срабатывать до создания.
func TestVaultCreateThroughSymlinkOutside(t *testing.T) {
	v, root := testVault(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "ссылка")); err != nil {
		t.Skipf("симлинки недоступны: %v", err)
	}

	_, err := v.Create(NewNote{Title: "Заметка", Notebook: "ссылка/Баги"})
	if !errors.Is(err, ErrOutsideVault) {
		t.Errorf("ошибка = %v, ожидалась ErrOutsideVault", err)
	}

	// И ничего не должно быть создано снаружи.
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("снаружи создано %v — MkdirAll прошёл сквозь ссылку", names)
	}
}

// После записи заголовок обязан отдавать новые байты: иначе следующий читатель
// увидит то, что было до сохранения.
func TestVaultSaveRefreshesFrontmatterBytes(t *testing.T) {
	v, root := testVault(t)
	writeNote(t, filepath.Join(root, "note.md"), "---\ntitle: Старый\n---\nтело\n")

	n, err := v.Load("note.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Doc.Meta.SetTitle("Новый"); err != nil {
		t.Fatal(err)
	}
	if err := v.Save(n); err != nil {
		t.Fatal(err)
	}

	// Save дописывает updated, поэтому в заголовке ожидаются обе строки.
	const want = "title: Новый\nupdated: 2026-08-19T15:00:00+03:00\n"
	if got := string(n.Doc.Meta.Bytes()); got != want {
		t.Errorf("Meta.Bytes() = %q, ожидалось %q", got, want)
	}
	if got := string(n.Doc.Bytes()); !strings.Contains(got, "title: Новый") {
		t.Errorf("Doc.Bytes() = %q", got)
	}
}

// Тело новой заметки завершается переводом строки: без него git пишет
// «\ No newline at end of file» в каждом диффе.
func TestCreateEndsBodyWithNewline(t *testing.T) {
	v, _ := testVault(t)

	cases := map[string]string{
		"без перевода":    "тело без перевода",
		"с переводом":     "тело с переводом\n",
		"пустое":          "",
		"много переводов": "тело\n\n\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			n, err := v.Create(NewNote{Title: "Заметка " + name, Body: body})
			if err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(n.Path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(string(raw), "\n") {
				t.Errorf("файл не заканчивается переводом строки:\n%q", raw)
			}
			// Уже имевшиеся переводы не схлопываются: тело — это то, что прислали.
			if body != "" && strings.HasSuffix(body, "\n") && !strings.HasSuffix(string(raw), body) {
				t.Errorf("тело изменено: %q не оканчивается на %q", raw, body)
			}
		})
	}
}

// Каждая наша запись обязана быть объявлена: на этом стоит реестр «своих»
// записей у watcher'а, а без него редактор перечитывает собственный буфер.
func TestOnWriteReportsEveryWrite(t *testing.T) {
	v, _ := testVault(t)

	type record struct {
		path string
		mod  time.Time
	}
	var seen []record
	v.OnWrite(func(path string, mod time.Time) {
		seen = append(seen, record{path, mod})
	})

	// Создание.
	n, err := v.Create(NewNote{Title: "Заметка", Body: "тело\n", Notebook: "Работа"})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 || seen[0].path != n.Path {
		t.Fatalf("после Create: %+v", seen)
	}

	// Сохранение.
	if err := n.Doc.Meta.SetTitle("Новый"); err != nil {
		t.Fatal(err)
	}
	if err := v.Save(n); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen[1].path != n.Path {
		t.Fatalf("после Save: %+v", seen)
	}

	// Перемещение сообщает про оба пути: и откуда, и куда.
	before := n.Path
	if err := v.Move(n, "Личное"); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 4 || seen[2].path != before || seen[3].path != n.Path {
		t.Fatalf("после Move: %+v", seen)
	}

	// И время должно быть настоящим временем файла, иначе сверка по mtime врёт.
	info, err := os.Stat(n.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !seen[3].mod.Equal(info.ModTime()) {
		t.Errorf("mtime = %v, на диске %v", seen[3].mod, info.ModTime())
	}
}

// Неизменённая заметка не пишется — значит и объявлять нечего.
func TestOnWriteSilentWhenNothingChanged(t *testing.T) {
	v, _ := testVault(t)
	n, err := v.Create(NewNote{Title: "Заметка"})
	if err != nil {
		t.Fatal(err)
	}

	var count int
	v.OnWrite(func(string, time.Time) { count++ })
	if err := v.Save(n); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("объявлено %d записей, а записи не было", count)
	}
}

// Только что созданная заметка изменённой не считается: иначе первое же
// сохранение переписало бы файл впустую и сдвинуло updated.
func TestCreateLeavesDocumentClean(t *testing.T) {
	v, _ := testVault(t)
	n, err := v.Create(NewNote{Title: "Заметка", Body: "тело\n"})
	if err != nil {
		t.Fatal(err)
	}
	if n.Doc.Modified() {
		t.Error("свежесозданный документ помечен изменённым")
	}

	before, err := os.Stat(n.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Save(n); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(n.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Error("Save сразу после Create переписал файл")
	}
}
