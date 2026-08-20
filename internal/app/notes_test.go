package app

import (
	"context"
	"errors"
	"testing"

	"tasker/internal/index"
	"tasker/internal/notes"
	"tasker/internal/vault"
)

// Сервис Wails тестируется поверх настоящего vault: он тонкий, и проверять в
// нём стоит ровно одно — что аргументы доезжают до слоя ниже и возвращаются
// обратно в том виде, который увидит фронтенд.
func testNotes(t *testing.T) *Notes {
	t.Helper()
	service, err := notes.Open(context.Background(), t.TempDir(), notes.Options{Origin: vault.OriginUser})
	if err != nil {
		t.Fatalf("notes.Open: %v", err)
	}
	t.Cleanup(func() { service.Close() })
	return NewNotes(service)
}

func TestCreateSearchGet(t *testing.T) {
	n := testNotes(t)
	ctx := context.Background()

	created, err := n.Create(ctx, "Счётчик перерасчёта", "Работа/Баги")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !vault.ValidID(created.ID) || created.Notebook != "Работа/Баги" {
		t.Fatalf("создано = %+v", created)
	}

	found, err := n.Search(ctx, "перерасч", 20, false)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(found) != 1 || found[0].ID != created.ID {
		t.Fatalf("поиск нашёл %d записей", len(found))
	}

	got, err := n.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "Счётчик перерасчёта" {
		t.Errorf("заголовок = %q", got.Title)
	}
}

// Save — то, что дёргает редактор. Он правит ровно заголовок и тело и не
// должен задевать остальное.
func TestSaveKeepsOtherFields(t *testing.T) {
	n := testNotes(t)
	ctx := context.Background()

	created, err := n.Create(ctx, "Исходный", "Работа")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := n.SetStatus(ctx, created.ID, "active"); err != nil {
		t.Fatal(err)
	}

	saved, err := n.Save(ctx, created.ID, "Новый заголовок", "новое тело\n")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.Title != "Новый заголовок" {
		t.Errorf("заголовок = %q", saved.Title)
	}
	if saved.Status != "active" {
		t.Errorf("статус = %q — Save задел лишнее", saved.Status)
	}
	if saved.Notebook != "Работа" {
		t.Errorf("ноутбук = %q", saved.Notebook)
	}

	got, err := n.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "новое тело\n" {
		t.Errorf("тело = %q", got.Body)
	}
}

func TestSetStatusRejectsNonsense(t *testing.T) {
	n := testNotes(t)
	ctx := context.Background()
	created, err := n.Create(ctx, "Заметка", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := n.SetStatus(ctx, created.ID, "почтиГотово"); !errors.Is(err, vault.ErrInvalidStatus) {
		t.Errorf("ошибка = %v, ожидалась ErrInvalidStatus", err)
	}
}

func TestTrashRemovesFromSearch(t *testing.T) {
	n := testNotes(t)
	ctx := context.Background()
	created, err := n.Create(ctx, "Ненужная", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Trash(ctx, created.ID); err != nil {
		t.Fatalf("Trash: %v", err)
	}
	found, err := n.Search(ctx, "", 20, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Errorf("удалённая осталась в выдаче: %d", len(found))
	}
}

// Сайдбар рисует то, что отдадут эти два метода.
func TestNotebooksAndTags(t *testing.T) {
	n := testNotes(t)
	ctx := context.Background()
	if _, err := n.Create(ctx, "Заметка", "Работа/Баги"); err != nil {
		t.Fatal(err)
	}

	books, err := n.Notebooks(ctx)
	if err != nil {
		t.Fatalf("Notebooks: %v", err)
	}
	var paths []string
	for _, b := range books {
		paths = append(paths, b.Path)
	}
	if len(paths) != 3 {
		t.Errorf("ноутбуки = %v, ожидались корень, Работа и Работа/Баги", paths)
	}

	if _, err := n.Tags(ctx); err != nil {
		t.Fatalf("Tags: %v", err)
	}
}

// Неизвестный id должен давать ошибку, а не пустую заметку: фронтенд по ней
// покажет плашку, а не пустой экран.
func TestGetUnknown(t *testing.T) {
	n := testNotes(t)
	if _, err := n.Get(context.Background(), "01K3QF8ZN7X2WPBV4YHMC6TDAE"); !errors.Is(err, index.ErrNotFound) {
		t.Errorf("ошибка = %v, ожидалась ErrNotFound", err)
	}
}

// «Активные» собирает active и onHold из всех ноутбуков, а завершённые прячет.
func TestTasksAndHideCompleted(t *testing.T) {
	n := testNotes(t)
	ctx := context.Background()

	for _, c := range []struct{ title, status string }{
		{"Активная", "active"}, {"Отложенная", "onHold"},
		{"Завершённая", "completed"}, {"Брошенная", "dropped"}, {"Без статуса", "none"},
	} {
		created, err := n.Create(ctx, c.title, "Работа")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := n.SetStatus(ctx, created.ID, c.status); err != nil {
			t.Fatal(err)
		}
	}

	tasks, err := n.Tasks(ctx, 50)
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("в работе %d, ожидалось 2: %+v", len(tasks), tasks)
	}

	visible, err := n.Search(ctx, "book:Работа", 50, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 3 {
		t.Errorf("видимых %d, ожидалось 3 (без завершённой и брошенной)", len(visible))
	}
	all, err := n.Search(ctx, "book:Работа", 50, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 5 {
		t.Errorf("всего %d, ожидалось 5", len(all))
	}
}

// Корзина целиком: удалить, увидеть в корзине, вернуть.
func TestTrashRestoreCycle(t *testing.T) {
	n := testNotes(t)
	ctx := context.Background()

	created, err := n.Create(ctx, "Ненужная", "Работа")
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Trash(ctx, created.ID); err != nil {
		t.Fatal(err)
	}

	trashed, err := n.Trashed(ctx, 50)
	if err != nil {
		t.Fatalf("Trashed: %v", err)
	}
	if len(trashed) != 1 || trashed[0].ID != created.ID {
		t.Fatalf("в корзине %+v", trashed)
	}
	if live, err := n.Search(ctx, "", 50, false); err != nil || len(live) != 0 {
		t.Errorf("удалённая видна в обычном списке: %+v, %v", live, err)
	}

	restored, err := n.Restore(ctx, created.ID)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restored.Trashed || restored.Notebook != "Работа" {
		t.Errorf("восстановлено как %+v", restored)
	}
	if empty, err := n.Trashed(ctx, 50); err != nil || len(empty) != 0 {
		t.Errorf("корзина не опустела: %+v, %v", empty, err)
	}
}

func TestDeleteForever(t *testing.T) {
	n := testNotes(t)
	ctx := context.Background()
	created, err := n.Create(ctx, "Насовсем", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Trash(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := n.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if trashed, err := n.Trashed(ctx, 50); err != nil || len(trashed) != 0 {
		t.Errorf("корзина = %+v, %v", trashed, err)
	}
}
