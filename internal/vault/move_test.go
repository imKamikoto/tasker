package vault

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMove(t *testing.T) {
	v, _ := testVault(t)

	n, err := v.Create(NewNote{Title: "Переезжающая", Body: "тело\n", Notebook: "Работа"})
	if err != nil {
		t.Fatal(err)
	}
	id := n.Doc.Meta.ID()
	before := n.Path

	if err := v.Move(n, "Личное/Идеи"); err != nil {
		t.Fatalf("Move: %v", err)
	}

	if _, err := os.Stat(before); !os.IsNotExist(err) {
		t.Errorf("файл остался на старом месте: %v", err)
	}
	if n.Notebook != "Личное/Идеи" {
		t.Errorf("Notebook = %q", n.Notebook)
	}
	if filepath.Base(n.Path) != "pereezzhayuschaya.md" {
		t.Errorf("имя файла изменилось: %q", filepath.Base(n.Path))
	}

	// Перемещение сохраняет id — на этом держатся ссылки (SPEC §8.1).
	moved, err := v.Load(n.Path)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Doc.Meta.ID() != id {
		t.Errorf("id сменился: %q → %q", id, moved.Doc.Meta.ID())
	}
	if moved.Doc.Body != "тело\n" {
		t.Errorf("тело = %q", moved.Doc.Body)
	}
}

// Перемещение — не правка содержимого, и updated трогать не должно.
func TestMoveKeepsUpdated(t *testing.T) {
	v, _ := testVault(t)
	n, err := v.Create(NewNote{Title: "Заметка", Notebook: "Работа"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := n.Doc.Meta.Updated()
	if err != nil {
		t.Fatal(err)
	}

	if err := v.Move(n, "Личное"); err != nil {
		t.Fatal(err)
	}
	after, err := n.Doc.Meta.Updated()
	if err != nil {
		t.Fatal(err)
	}
	if !after.Equal(before) {
		t.Errorf("updated = %v, было %v — перемещение выдало себя за правку", after, before)
	}
}

func TestMoveResolvesNameCollisions(t *testing.T) {
	v, _ := testVault(t)

	occupied, err := v.Create(NewNote{Title: "Одинаковая", Notebook: "Личное", Body: "первая\n"})
	if err != nil {
		t.Fatal(err)
	}
	n, err := v.Create(NewNote{Title: "Одинаковая", Notebook: "Работа", Body: "вторая\n"})
	if err != nil {
		t.Fatal(err)
	}

	if err := v.Move(n, "Личное"); err != nil {
		t.Fatalf("Move: %v", err)
	}
	if filepath.Base(n.Path) != "odinakovaya-2.md" {
		t.Errorf("имя = %q, ожидалось odinakovaya-2.md", filepath.Base(n.Path))
	}

	raw, err := os.ReadFile(occupied.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(raw), "первая\n") {
		t.Errorf("занятый файл затёрт:\n%s", raw)
	}
}

func TestMoveToSameNotebook(t *testing.T) {
	v, _ := testVault(t)
	n, err := v.Create(NewNote{Title: "Заметка", Notebook: "Работа"})
	if err != nil {
		t.Fatal(err)
	}
	before := n.Path

	if err := v.Move(n, "Работа"); err != nil {
		t.Fatalf("Move в тот же ноутбук: %v", err)
	}
	if n.Path != before {
		t.Errorf("путь изменился: %q → %q", before, n.Path)
	}
	if _, err := os.Stat(before); err != nil {
		t.Errorf("файл пропал: %v", err)
	}
}

func TestMoveToRoot(t *testing.T) {
	v, _ := testVault(t)
	n, err := v.Create(NewNote{Title: "Заметка", Notebook: "Работа/Баги"})
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Move(n, ""); err != nil {
		t.Fatalf("Move: %v", err)
	}
	if n.Notebook != "" {
		t.Errorf("Notebook = %q, ожидался корень", n.Notebook)
	}
	if filepath.Dir(n.Path) != v.Root() {
		t.Errorf("файл не в корне: %s", n.Path)
	}
}

func TestMoveRejectsBadNotebooks(t *testing.T) {
	v, _ := testVault(t)

	for _, nb := range []string{"../снаружи", "/etc", "Работа/../../снаружи"} {
		n, err := v.Create(NewNote{Title: "Заметка", Notebook: "Работа"})
		if err != nil {
			t.Fatal(err)
		}
		if err := v.Move(n, nb); !errors.Is(err, ErrOutsideVault) {
			t.Errorf("Move(%q) вернул %v, ожидалась ErrOutsideVault", nb, err)
		}
	}
	for _, nb := range []string{".git", ".tasker"} {
		n, err := v.Create(NewNote{Title: "Заметка", Notebook: "Работа"})
		if err != nil {
			t.Fatal(err)
		}
		if err := v.Move(n, nb); !errors.Is(err, ErrHiddenPath) {
			t.Errorf("Move(%q) вернул %v, ожидалась ErrHiddenPath", nb, err)
		}
	}
}
