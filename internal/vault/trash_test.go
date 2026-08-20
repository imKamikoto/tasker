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
