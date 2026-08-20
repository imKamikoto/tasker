package vault

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrash(t *testing.T) {
	v, _ := testVault(t)

	created, err := v.Create(NewNote{Title: "Ненужная заметка", Body: "тело\n", Notebook: "Работа/Баги"})
	if err != nil {
		t.Fatal(err)
	}
	original := created.Path
	id := created.Doc.Meta.ID()

	if err := v.Trash(created); err != nil {
		t.Fatalf("Trash: %v", err)
	}

	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Errorf("исходный файл на месте: %v", err)
	}
	if created.Notebook != trashDirName {
		t.Errorf("Notebook = %q, ожидался %q", created.Notebook, trashDirName)
	}
	// Сравниваем с v.Root(), а не с root: vault хранит путь уже после
	// EvalSymlinks, и на macOS это /private/var против /var.
	if filepath.Dir(created.Path) != filepath.Join(v.Root(), trashDirName) {
		t.Errorf("файл не в корзине: %s", created.Path)
	}
	if _, err := os.Stat(created.Path); err != nil {
		t.Fatalf("файла нет в корзине: %v", err)
	}

	// Заметка должна остаться собой: id тот же, тело на месте.
	moved, err := v.Load(created.Path)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Doc.Meta.ID() != id {
		t.Errorf("id сменился: %q → %q", id, moved.Doc.Meta.ID())
	}
	if moved.Doc.Body != "тело\n" {
		t.Errorf("тело = %q", moved.Doc.Body)
	}

	// Откуда и когда — иначе восстанавливать некуда (SPEC §4.3).
	from, err := moved.Doc.Meta.TrashedFrom()
	if err != nil {
		t.Fatal(err)
	}
	if from != "Работа/Баги/nenuzhnaya-zametka.md" {
		t.Errorf("trashedFrom = %q", from)
	}
	at, err := moved.Doc.Meta.TrashedAt()
	if err != nil {
		t.Fatal(err)
	}
	if at.IsZero() {
		t.Error("trashedAt не проставлен")
	}
}

// Две заметки с одинаковым именем из разных ноутбуков не должны затирать друг
// друга в корзине.
func TestTrashResolvesNameCollisions(t *testing.T) {
	v, _ := testVault(t)

	var paths []string
	for _, book := range []string{"Работа", "Личное", "Архив"} {
		n, err := v.Create(NewNote{Title: "Одинаковая", Notebook: book, Body: book + "\n"})
		if err != nil {
			t.Fatal(err)
		}
		if err := v.Trash(n); err != nil {
			t.Fatalf("Trash из %s: %v", book, err)
		}
		paths = append(paths, filepath.Base(n.Path))
	}

	want := []string{"odinakovaya.md", "odinakovaya-2.md", "odinakovaya-3.md"}
	for i := range want {
		if paths[i] != want[i] {
			t.Errorf("имя %d = %q, ожидалось %q", i, paths[i], want[i])
		}
	}

	entries, err := os.ReadDir(filepath.Join(v.Root(), trashDirName))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Errorf("в корзине %d файлов, ожидалось 3", len(entries))
	}

	// И содержимое не перепуталось.
	for i, book := range []string{"Работа", "Личное", "Архив"} {
		raw, err := os.ReadFile(filepath.Join(v.Root(), trashDirName, paths[i]))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(string(raw), book+"\n") {
			t.Errorf("файл %s содержит не то:\n%s", paths[i], raw)
		}
	}
}

func TestTrashAlreadyTrashed(t *testing.T) {
	v, _ := testVault(t)
	n, err := v.Create(NewNote{Title: "Заметка"})
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Trash(n); err != nil {
		t.Fatal(err)
	}
	if err := v.Trash(n); !errors.Is(err, ErrAlreadyTrashed) {
		t.Errorf("повторный Trash вернул %v, ожидалась ErrAlreadyTrashed", err)
	}
}

// Корзина заводится сама: до первого удаления её в vault нет.
func TestTrashCreatesTrashDir(t *testing.T) {
	v, _ := testVault(t)
	if _, err := os.Stat(filepath.Join(v.Root(), trashDirName)); !os.IsNotExist(err) {
		t.Fatalf("корзина существует до первого удаления: %v", err)
	}
	n, err := v.Create(NewNote{Title: "Заметка"})
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Trash(n); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(v.Root(), trashDirName)); err != nil {
		t.Errorf("корзина не создана: %v", err)
	}
}

// Заметка из корня vault: trashedFrom должен быть просто именем файла.
func TestTrashFromRoot(t *testing.T) {
	v, _ := testVault(t)
	n, err := v.Create(NewNote{Title: "Корневая"})
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Trash(n); err != nil {
		t.Fatal(err)
	}
	moved, err := v.Load(n.Path)
	if err != nil {
		t.Fatal(err)
	}
	from, err := moved.Doc.Meta.TrashedFrom()
	if err != nil {
		t.Fatal(err)
	}
	if from != "kornevaya.md" {
		t.Errorf("trashedFrom = %q", from)
	}
}

func TestRestore(t *testing.T) {
	v, _ := testVault(t)

	n, err := v.Create(NewNote{Title: "Вернётся", Body: "тело\n", Notebook: "Работа/Баги"})
	if err != nil {
		t.Fatal(err)
	}
	id := n.Doc.Meta.ID()
	original := n.Path
	if err := v.Trash(n); err != nil {
		t.Fatal(err)
	}

	if err := v.Restore(n); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if n.Notebook != "Работа/Баги" {
		t.Errorf("ноутбук = %q", n.Notebook)
	}
	if n.Path != original {
		t.Errorf("путь = %q, ожидался %q", n.Path, original)
	}

	restored, err := v.Load(n.Path)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Doc.Meta.ID() != id {
		t.Errorf("id сменился")
	}
	if restored.Doc.Body != "тело\n" {
		t.Errorf("тело = %q", restored.Doc.Body)
	}
	// Следы корзины должны исчезнуть.
	for _, field := range []string{"trashedFrom", "trashedAt"} {
		if ok, _ := restored.Doc.Meta.Get(field, new(string)); ok {
			t.Errorf("поле %s осталось", field)
		}
	}
}

// Ноутбук успели удалить: он создаётся заново, и заметка возвращается ровно
// туда, где была.
func TestRestoreRecreatesMissingNotebook(t *testing.T) {
	v, _ := testVault(t)
	n, err := v.Create(NewNote{Title: "Сирота", Notebook: "Исчезнет"})
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Trash(n); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(v.Root(), "Исчезнет")); err != nil {
		t.Fatal(err)
	}

	if err := v.Restore(n); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if n.Notebook != "Исчезнет" {
		t.Errorf("ноутбук = %q, ожидался «Исчезнет» пересозданным", n.Notebook)
	}
	if _, err := os.Stat(n.Path); err != nil {
		t.Fatalf("файла нет: %v", err)
	}
}

// trashedFrom поправили руками так, что писать туда нельзя. Заметку всё равно
// надо вернуть — в корень, но вернуть.
func TestRestoreFallsBackToRootOnBadPath(t *testing.T) {
	for _, bad := range []string{"../снаружи/note.md", ".git/note.md", "/etc/note.md"} {
		t.Run(bad, func(t *testing.T) {
			v, _ := testVault(t)
			n, err := v.Create(NewNote{Title: "Заметка", Notebook: "Работа"})
			if err != nil {
				t.Fatal(err)
			}
			if err := v.Trash(n); err != nil {
				t.Fatal(err)
			}
			if err := n.Doc.Meta.Set("trashedFrom", bad); err != nil {
				t.Fatal(err)
			}

			if err := v.Restore(n); err != nil {
				t.Fatalf("Restore: %v", err)
			}
			if n.Notebook != "" {
				t.Errorf("ноутбук = %q, ожидался корень", n.Notebook)
			}
			if filepath.Dir(n.Path) != v.Root() {
				t.Errorf("файл не в корне: %s", n.Path)
			}
		})
	}
}

// Имя в исходном ноутбуке успели занять, пока заметка лежала в корзине.
func TestRestoreResolvesNameCollision(t *testing.T) {
	v, _ := testVault(t)
	first, err := v.Create(NewNote{Title: "Одинаковая", Notebook: "Работа", Body: "первая\n"})
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Trash(first); err != nil {
		t.Fatal(err)
	}
	second, err := v.Create(NewNote{Title: "Одинаковая", Notebook: "Работа", Body: "вторая\n"})
	if err != nil {
		t.Fatal(err)
	}

	if err := v.Restore(first); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if filepath.Base(first.Path) == filepath.Base(second.Path) {
		t.Fatalf("оба файла называются %q", filepath.Base(first.Path))
	}
	raw, err := os.ReadFile(second.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(raw), "вторая\n") {
		t.Errorf("занявший файл затёрт:\n%s", raw)
	}
}

func TestRestoreRejectsLiveNote(t *testing.T) {
	v, _ := testVault(t)
	n, err := v.Create(NewNote{Title: "Живая"})
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Restore(n); !errors.Is(err, ErrNotTrashed) {
		t.Errorf("ошибка = %v, ожидалась ErrNotTrashed", err)
	}
}

func TestDelete(t *testing.T) {
	v, _ := testVault(t)
	n, err := v.Create(NewNote{Title: "Насовсем"})
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Trash(n); err != nil {
		t.Fatal(err)
	}

	path := n.Path
	if err := v.Delete(n); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("файл на месте: %v", err)
	}
}

// Мимо корзины удалить нельзя: это единственная защита от необратимой ошибки.
func TestDeleteRejectsLiveNote(t *testing.T) {
	v, _ := testVault(t)
	n, err := v.Create(NewNote{Title: "Живая", Body: "тело\n"})
	if err != nil {
		t.Fatal(err)
	}

	if err := v.Delete(n); !errors.Is(err, ErrNotTrashed) {
		t.Errorf("ошибка = %v, ожидалась ErrNotTrashed", err)
	}
	if _, err := os.Stat(n.Path); err != nil {
		t.Errorf("файл всё-таки удалён: %v", err)
	}
}
