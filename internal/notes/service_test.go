package notes

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"tasker/internal/index"
	"tasker/internal/vault"
)

func testService(t *testing.T, origin vault.Origin) (*Service, string) {
	t.Helper()
	root := t.TempDir()
	s, err := Open(context.Background(), root, Options{Origin: origin})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, root
}

func gitLog(t *testing.T, root string) string {
	t.Helper()
	c := exec.Command("git", "-c", "core.quotepath=false", "log", "--format=%s")
	c.Dir = root
	out, err := c.CombinedOutput()
	if err != nil {
		return ""
	}
	return string(out)
}

func str(v string) *string { return &v }

func TestOpenCreatesEverything(t *testing.T) {
	_, root := testService(t, vault.OriginUser)

	for _, want := range []string{".git", ".tasker/index.sqlite", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(want))); err != nil {
			t.Errorf("нет %s: %v", want, err)
		}
	}
}

func TestCreate(t *testing.T) {
	s, root := testService(t, vault.OriginAgent)
	ctx := context.Background()

	rec, err := s.Create(ctx, CreateParams{
		Title:    "Счётчик перерасчёта не обновляется",
		Body:     "Описание бага.\n",
		Notebook: "Работа/Баги",
		Tags:     []string{"баг", "armz"},
		Status:   vault.StatusActive,
		Context:  &vault.Context{Repo: "tasker", Branch: "main"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if !vault.ValidID(rec.ID) {
		t.Errorf("id = %q", rec.ID)
	}
	if rec.Notebook != "Работа/Баги" || rec.Status != "active" {
		t.Errorf("запись = %+v", rec)
	}

	// Файл на диске.
	path := filepath.Join(root, filepath.FromSlash(rec.Path))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("файла нет: %v", err)
	}
	if !strings.Contains(string(raw), "origin: agent") {
		t.Errorf("заметка агента не помечена:\n%s", raw)
	}
	if !strings.Contains(string(raw), "repo: tasker") {
		t.Errorf("контекст не записан:\n%s", raw)
	}

	// Индекс знает про неё.
	if _, err := s.Index().Get(ctx, rec.ID); err != nil {
		t.Errorf("заметки нет в индексе: %v", err)
	}

	// И коммит с сообщением агента (SPEC §4.5).
	if !strings.Contains(gitLog(t, root), `agent: create "Счётчик перерасчёта не обновляется"`) {
		t.Errorf("коммит агента не найден:\n%s", gitLog(t, root))
	}
}

func TestCreateUserOrigin(t *testing.T) {
	s, root := testService(t, vault.OriginUser)
	if _, err := s.Create(context.Background(), CreateParams{Title: "Моя заметка"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gitLog(t, root), "notes: Моя заметка") {
		t.Errorf("сообщение коммита не то:\n%s", gitLog(t, root))
	}
}

// LinkTo проставляет взаимные ссылки: обе заметки должны знать друг о друге.
func TestCreateWithLink(t *testing.T) {
	s, _ := testService(t, vault.OriginAgent)
	ctx := context.Background()

	target, err := s.Create(ctx, CreateParams{Title: "Текущая задача", Body: "тело\n"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.Create(ctx, CreateParams{Title: "Найденный баг", Body: "описание\n", LinkTo: target.ID})
	if err != nil {
		t.Fatalf("Create с LinkTo: %v", err)
	}

	got, err := s.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Body, "tasker://note/"+target.ID) {
		t.Errorf("в новой заметке нет ссылки на цель:\n%s", got.Body)
	}

	back, err := s.Get(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(back.Body, "tasker://note/"+created.ID) {
		t.Errorf("в целевой заметке нет обратной ссылки:\n%s", back.Body)
	}
	if len(back.Backlinks) != 1 || back.Backlinks[0].ID != created.ID {
		t.Errorf("бэклинки цели = %+v", back.Backlinks)
	}
}

func TestCreateWithUnknownLink(t *testing.T) {
	s, _ := testService(t, vault.OriginAgent)
	_, err := s.Create(context.Background(), CreateParams{
		Title: "Заметка", LinkTo: "01K3QF8ZN7X2WPBV4YHMC6TDAE",
	})
	if !errors.Is(err, index.ErrNotFound) {
		t.Errorf("ошибка = %v, ожидалась ErrNotFound", err)
	}
}

// Указатели: не переданное поле не трогается, переданный ноль — трогается.
func TestUpdateOnlyTouchesPassedFields(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()

	rec, err := s.Create(ctx, CreateParams{
		Title: "Исходный", Body: "тело\n", Tags: []string{"один"},
		Status: vault.StatusActive, Pinned: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := s.Update(ctx, UpdateParams{ID: rec.ID, Title: str("Новый")})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Title != "Новый" {
		t.Errorf("title = %q", updated.Title)
	}
	if updated.Status != "active" || !updated.Pinned || len(updated.Tags) != 1 {
		t.Errorf("непереданные поля изменились: %+v", updated)
	}

	// Переданный ноль — это значение, а не «не передано».
	no := false
	updated, err = s.Update(ctx, UpdateParams{ID: rec.ID, Pinned: &no})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Pinned {
		t.Error("pinned=false не применился")
	}
	if updated.Title != "Новый" {
		t.Errorf("title сбросился: %q", updated.Title)
	}
}

func TestUpdateAppendAndPrepend(t *testing.T) {
	s, _ := testService(t, vault.OriginAgent)
	ctx := context.Background()

	rec, err := s.Create(ctx, CreateParams{Title: "Заметка", Body: "исходное тело\n"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(ctx, UpdateParams{ID: rec.ID, Append: str("дописанное")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(ctx, UpdateParams{ID: rec.ID, Prepend: str("сверху")}); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := "сверху\n\nисходное тело\n\nдописанное\n"
	if got.Body != want {
		t.Errorf("тело = %q, ожидалось %q", got.Body, want)
	}
}

func TestUpdateBodyAndAppendConflict(t *testing.T) {
	s, _ := testService(t, vault.OriginAgent)
	ctx := context.Background()
	rec, err := s.Create(ctx, CreateParams{Title: "Заметка"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Update(ctx, UpdateParams{ID: rec.ID, Body: str("целиком"), Append: str("и дописать")})
	if !errors.Is(err, ErrConflictingParams) {
		t.Errorf("ошибка = %v, ожидалась ErrConflictingParams", err)
	}
}

func TestUpdateTags(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()
	rec, err := s.Create(ctx, CreateParams{Title: "Заметка", Tags: []string{"один", "два"}})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := s.Update(ctx, UpdateParams{
		ID: rec.ID, AddTags: []string{"три", "один"}, RemoveTags: []string{"два"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(updated.Tags, ",")
	if got != "один,три" {
		t.Errorf("теги = %q, ожидалось «один,три»", got)
	}
}

func TestUpdateMovesToNotebook(t *testing.T) {
	s, root := testService(t, vault.OriginUser)
	ctx := context.Background()
	rec, err := s.Create(ctx, CreateParams{Title: "Заметка", Notebook: "Работа"})
	if err != nil {
		t.Fatal(err)
	}
	before := filepath.Join(root, filepath.FromSlash(rec.Path))

	updated, err := s.Update(ctx, UpdateParams{ID: rec.ID, Notebook: str("Личное/Идеи")})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Notebook != "Личное/Идеи" {
		t.Errorf("ноутбук = %q", updated.Notebook)
	}
	if _, err := os.Stat(before); !os.IsNotExist(err) {
		t.Error("файл остался на старом месте")
	}
	if updated.ID != rec.ID {
		t.Error("id сменился при перемещении")
	}

	// Индекс не должен помнить старый путь.
	states, err := s.Index().States(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, stale := states[rec.Path]; stale {
		t.Errorf("старый путь остался в индексе: %v", states)
	}
}

func TestSetStatus(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()
	rec, err := s.Create(ctx, CreateParams{Title: "Заметка"})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := s.SetStatus(ctx, rec.ID, vault.StatusCompleted)
	if err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if updated.Status != "completed" {
		t.Errorf("статус = %q", updated.Status)
	}
	if _, err := s.SetStatus(ctx, rec.ID, vault.Status("выдуманный")); err == nil {
		t.Error("выдуманный статус принят")
	}
}

func TestTrash(t *testing.T) {
	s, root := testService(t, vault.OriginUser)
	ctx := context.Background()
	rec, err := s.Create(ctx, CreateParams{Title: "Ненужная", Notebook: "Работа"})
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Trash(ctx, rec.ID); err != nil {
		t.Fatalf("Trash: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rec.Path))); !os.IsNotExist(err) {
		t.Error("файл остался на месте")
	}

	got, err := s.Index().Get(ctx, rec.ID)
	if err != nil {
		t.Fatalf("заметка пропала из индекса: %v", err)
	}
	if !got.Trashed {
		t.Error("в индексе не помечена как удалённая")
	}
	// И из поиска она уходит.
	found, err := s.Search(ctx, "", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Errorf("удалённая всё ещё ищется: %+v", found)
	}
}

func TestGet(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()
	rec, err := s.Create(ctx, CreateParams{Title: "Заметка", Body: "тело заметки\n", Tags: []string{"тег"}})
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.Get(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Body != "тело заметки\n" {
		t.Errorf("тело = %q", got.Body)
	}
	if got.Title != "Заметка" || len(got.Tags) != 1 {
		t.Errorf("запись = %+v", got.Record)
	}

	if _, err := s.Get(ctx, "01K3QF8ZN7X2WPBV4YHMC6TDAE"); !errors.Is(err, index.ErrNotFound) {
		t.Errorf("ошибка = %v, ожидалась ErrNotFound", err)
	}
}

func TestSearchAndTasks(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()

	if _, err := s.Create(ctx, CreateParams{Title: "Активная", Status: vault.StatusActive, Tags: []string{"работа"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, CreateParams{Title: "Отложенная", Status: vault.StatusOnHold}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, CreateParams{Title: "Завершённая", Status: vault.StatusCompleted}); err != nil {
		t.Fatal(err)
	}

	found, err := s.Search(ctx, "tag:работа", SearchOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(found) != 1 || found[0].Title != "Активная" {
		t.Errorf("поиск нашёл %+v", found)
	}

	// list_tasks по умолчанию — active и onHold (docs/MCP.md §3).
	tasks, err := s.Tasks(ctx, TasksParams{})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("задач %d, ожидалось 2: %+v", len(tasks), tasks)
	}
	for _, task := range tasks {
		if task.Status == "completed" {
			t.Errorf("завершённая попала в список задач: %+v", task)
		}
	}
}

func TestSearchIncludeBody(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()
	if _, err := s.Create(ctx, CreateParams{Title: "Заметка", Body: "полное тело\n"}); err != nil {
		t.Fatal(err)
	}

	found, err := s.Search(ctx, "", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if found[0].Body != "" {
		t.Errorf("тело пришло без спроса: %q", found[0].Body)
	}

	found, err = s.Search(ctx, "", SearchOptions{IncludeBody: true})
	if err != nil {
		t.Fatal(err)
	}
	if found[0].Body != "полное тело\n" {
		t.Errorf("тело = %q", found[0].Body)
	}
}

func TestNotebooksAndTags(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()
	if _, err := s.Create(ctx, CreateParams{Title: "А", Notebook: "Работа/Баги", Tags: []string{"баг"}}); err != nil {
		t.Fatal(err)
	}

	books, err := s.Notebooks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) == 0 {
		t.Error("ноутбуков нет")
	}
	tags, err := s.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0].Name != "баг" {
		t.Errorf("теги = %+v", tags)
	}
}

// Файл, положенный в vault снаружи, подхватывается сверкой.
func TestSync(t *testing.T) {
	s, root := testService(t, vault.OriginUser)
	ctx := context.Background()

	if err := os.WriteFile(filepath.Join(root, "снаружи.md"), []byte("тело\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := s.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.Added != 1 {
		t.Errorf("добавлено %d: %+v", res.Added, res)
	}
	found, err := s.Search(ctx, "", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Title != "снаружи" {
		t.Errorf("поиск = %+v", found)
	}
}

func TestRestore(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()

	created, err := s.Create(ctx, CreateParams{Title: "Вернётся", Notebook: "Работа", Body: "тело\n"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Trash(ctx, created.ID); err != nil {
		t.Fatal(err)
	}

	restored, err := s.Restore(ctx, created.ID)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restored.Trashed || restored.Notebook != "Работа" {
		t.Errorf("запись = %+v", restored)
	}
	if restored.Path != created.Path {
		t.Errorf("путь = %q, ожидался %q", restored.Path, created.Path)
	}

	// В индексе не должно остаться призрака по старому пути.
	states, err := s.Index().States(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 {
		t.Errorf("путей в индексе %d: %v", len(states), states)
	}

	// И заметка снова находится обычным поиском.
	found, err := s.Search(ctx, "", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Errorf("поиск нашёл %d", len(found))
	}
}

func TestDeleteForever(t *testing.T) {
	s, root := testService(t, vault.OriginUser)
	ctx := context.Background()

	created, err := s.Create(ctx, CreateParams{Title: "Насовсем"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Trash(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	trashed, err := s.Index().Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(trashed.Path))); !os.IsNotExist(err) {
		t.Errorf("файл на месте: %v", err)
	}
	if _, err := s.Index().Get(ctx, created.ID); !errors.Is(err, index.ErrNotFound) {
		t.Errorf("строка индекса осталась: %v", err)
	}
	// История помнит: вернуть можно только оттуда.
	if !strings.Contains(gitLog(t, root), "Насовсем") {
		t.Errorf("в истории нет следа:\n%s", gitLog(t, root))
	}
}

// Живую заметку насовсем удалить нельзя — только через корзину.
func TestDeleteRejectsLiveNote(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()
	created, err := s.Create(ctx, CreateParams{Title: "Живая"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, created.ID); !errors.Is(err, vault.ErrNotTrashed) {
		t.Errorf("ошибка = %v, ожидалась ErrNotTrashed", err)
	}
}

// Экран корзины показывает только удалённое.
func TestSearchTrashOnly(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()

	live, err := s.Create(ctx, CreateParams{Title: "Живая"})
	if err != nil {
		t.Fatal(err)
	}
	gone, err := s.Create(ctx, CreateParams{Title: "Удалённая"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Trash(ctx, gone.ID); err != nil {
		t.Fatal(err)
	}

	found, err := s.Search(ctx, "", SearchOptions{Trash: index.TrashOnly})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != gone.ID {
		t.Errorf("на экране корзины %+v", found)
	}
	_ = live
}
