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

func TestDuplicate(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()

	original, err := s.Create(ctx, CreateParams{
		Title: "Оригинал", Body: "тело оригинала\n", Notebook: "Работа/Баги",
		Tags: []string{"баг", "срочно"}, Status: vault.StatusActive, Pinned: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	copy, err := s.Duplicate(ctx, original.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}

	if copy.ID == original.ID {
		t.Error("копия получила id оригинала — ссылки бы разъехались")
	}
	if copy.Title != "Оригинал (копия)" {
		t.Errorf("заголовок = %q", copy.Title)
	}
	if copy.Path == original.Path {
		t.Errorf("копия легла поверх оригинала: %q", copy.Path)
	}
	if copy.Notebook != "Работа/Баги" || copy.Status != "active" || !copy.Pinned {
		t.Errorf("свойства не перенесены: %+v", copy)
	}
	if len(copy.Tags) != 2 {
		t.Errorf("теги = %v", copy.Tags)
	}

	got, err := s.Get(ctx, copy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != "тело оригинала\n" {
		t.Errorf("тело = %q", got.Body)
	}

	// Оригинал на месте и не изменился.
	if _, err := s.Get(ctx, original.ID); err != nil {
		t.Errorf("оригинал пострадал: %v", err)
	}
	found, err := s.Search(ctx, "", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Errorf("заметок %d, ожидалось 2", len(found))
	}
}

func TestSetPinned(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()
	created, err := s.Create(ctx, CreateParams{Title: "Заметка"})
	if err != nil {
		t.Fatal(err)
	}

	pinned, err := s.SetPinned(ctx, created.ID, true)
	if err != nil {
		t.Fatalf("SetPinned: %v", err)
	}
	if !pinned.Pinned {
		t.Error("не закрепилась")
	}
	unpinned, err := s.SetPinned(ctx, created.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if unpinned.Pinned {
		t.Error("не открепилась")
	}
	// Заголовок при этом трогать нельзя.
	if unpinned.Title != "Заметка" {
		t.Errorf("заголовок = %q", unpinned.Title)
	}
}

// Копия копии не должна затирать первую копию.
func TestDuplicateTwice(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()
	original, err := s.Create(ctx, CreateParams{Title: "Оригинал"})
	if err != nil {
		t.Fatal(err)
	}

	first, err := s.Duplicate(ctx, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Duplicate(ctx, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Path == second.Path || first.ID == second.ID {
		t.Errorf("копии совпали: %q и %q", first.Path, second.Path)
	}
}

func makeNotes(t *testing.T, s *Service, titles ...string) []string {
	t.Helper()
	var ids []string
	for _, title := range titles {
		created, err := s.Create(context.Background(), CreateParams{Title: title, Notebook: "Работа"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, created.ID)
	}
	return ids
}

func commitCount(t *testing.T, root string) int {
	t.Helper()
	return strings.Count(strings.TrimSpace(gitLog(t, root)), "\n") + 1
}

// Главное требование к массовым операциям: один коммит на пачку, а не по одному
// на заметку (SPEC §4.5).
func TestBulkMakesOneCommit(t *testing.T) {
	s, root := testService(t, vault.OriginUser)
	ctx := context.Background()
	ids := makeNotes(t, s, "Первая", "Вторая", "Третья", "Четвёртая")
	before := commitCount(t, root)

	updated, err := s.SetStatusMany(ctx, ids, vault.StatusCompleted)
	if err != nil {
		t.Fatalf("SetStatusMany: %v", err)
	}
	if len(updated) != 4 {
		t.Errorf("обновлено %d, ожидалось 4", len(updated))
	}
	for _, rec := range updated {
		if rec.Status != "completed" {
			t.Errorf("%q со статусом %q", rec.Title, rec.Status)
		}
	}

	if added := commitCount(t, root) - before; added != 1 {
		t.Errorf("коммитов добавилось %d, ожидался один", added)
	}
	if !strings.Contains(gitLog(t, root), "notes: 4 изменено") {
		t.Errorf("сообщение коммита не то:\n%s", gitLog(t, root))
	}
}

func TestBulkMove(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()
	ids := makeNotes(t, s, "Раз", "Два", "Три")

	updated, err := s.MoveMany(ctx, ids, "Личное/Идеи")
	if err != nil {
		t.Fatalf("MoveMany: %v", err)
	}
	for _, rec := range updated {
		if rec.Notebook != "Личное/Идеи" {
			t.Errorf("%q в ноутбуке %q", rec.Title, rec.Notebook)
		}
	}
	found, err := s.Search(ctx, "book:Личное/Идеи", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 3 {
		t.Errorf("в целевом ноутбуке %d заметок", len(found))
	}
}

func TestBulkTrashAndPin(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()
	ids := makeNotes(t, s, "Раз", "Два")

	if _, err := s.SetPinnedMany(ctx, ids, true); err != nil {
		t.Fatalf("SetPinnedMany: %v", err)
	}
	found, err := s.Search(ctx, "is:pinned", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Errorf("закреплённых %d", len(found))
	}

	if _, err := s.TrashMany(ctx, ids); err != nil {
		t.Fatalf("TrashMany: %v", err)
	}
	live, err := s.Search(ctx, "", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 0 {
		t.Errorf("осталось живых: %d", len(live))
	}
}

// Одна сорвавшаяся заметка не должна отменять остальные: остановиться
// посередине значит оставить пачку в непонятном состоянии.
func TestBulkContinuesAfterFailure(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()
	ids := makeNotes(t, s, "Первая", "Вторая")
	// В середину подсовываем несуществующий идентификатор.
	withGhost := []string{ids[0], "01K3QF8ZN7X2WPBV4YHMC6TDAE", ids[1]}

	updated, err := s.SetStatusMany(ctx, withGhost, vault.StatusActive)
	if err == nil {
		t.Error("об ошибке не сообщили")
	}
	if len(updated) != 2 {
		t.Errorf("обновлено %d, ожидались обе живые", len(updated))
	}
	found, err := s.Search(ctx, "status:active", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Errorf("статус проставлен %d заметкам", len(found))
	}
}

func TestBulkEmptyIsNoop(t *testing.T) {
	s, root := testService(t, vault.OriginUser)
	makeNotes(t, s, "Одна")
	before := commitCount(t, root)

	updated, err := s.TrashMany(context.Background(), nil)
	if err != nil || len(updated) != 0 {
		t.Errorf("на пустой пачке: %+v, %v", updated, err)
	}
	if commitCount(t, root) != before {
		t.Error("пустая пачка создала коммит")
	}
}

// От агента сообщение коммита должно оставаться агентским.
func TestBulkAgentMessage(t *testing.T) {
	s, root := testService(t, vault.OriginAgent)
	ctx := context.Background()
	ids := makeNotes(t, s, "Раз", "Два")

	if _, err := s.SetStatusMany(ctx, ids, vault.StatusCompleted); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gitLog(t, root), `agent: status "2 заметок"`) {
		t.Errorf("сообщение не агентское:\n%s", gitLog(t, root))
	}
}

// Сбой на самом действии, а не на поиске заметки: остальные всё равно должны
// быть обработаны. Уже удалённая заметка — самый простой способ его получить.
func TestBulkContinuesAfterActionFailure(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()
	ids := makeNotes(t, s, "Первая", "Уже в корзине", "Третья")

	if err := s.Trash(ctx, ids[1]); err != nil {
		t.Fatal(err)
	}

	updated, err := s.TrashMany(ctx, ids)
	if !errors.Is(err, vault.ErrAlreadyTrashed) {
		t.Errorf("ошибка = %v, ожидалась ErrAlreadyTrashed", err)
	}
	if len(updated) != 2 {
		t.Fatalf("обработано %d, ожидались две оставшиеся", len(updated))
	}

	live, err := s.Search(ctx, "", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 0 {
		t.Errorf("живых осталось %d — пачка оборвалась на сбое", len(live))
	}
}

// Критерий приёмки SPEC §8.2: переименование проходит по всем заметкам,
// переписывает frontmatter, одной операцией и одним коммитом.
func TestRenameTag(t *testing.T) {
	s, root := testService(t, vault.OriginUser)
	ctx := context.Background()

	for _, tags := range [][]string{{"баг", "срочно"}, {"баг"}, {"идея"}} {
		if _, err := s.Create(ctx, CreateParams{Title: "Заметка " + tags[0], Tags: tags}); err != nil {
			t.Fatal(err)
		}
	}
	before := commitCount(t, root)

	updated, err := s.RenameTag(ctx, "баг", "дефект")
	if err != nil {
		t.Fatalf("RenameTag: %v", err)
	}
	if len(updated) != 2 {
		t.Fatalf("тронуто %d заметок, ожидалось 2", len(updated))
	}
	if added := commitCount(t, root) - before; added != 1 {
		t.Errorf("коммитов добавилось %d, ожидался один", added)
	}

	// Старого тега больше нет ни у кого, новый есть у обеих.
	if old, err := s.Search(ctx, "tag:баг", SearchOptions{}); err != nil || len(old) != 0 {
		t.Errorf("по старому тегу нашлось %d: %v", len(old), err)
	}
	renamed, err := s.Search(ctx, "tag:дефект", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(renamed) != 2 {
		t.Errorf("по новому тегу нашлось %d", len(renamed))
	}
	// Соседние теги не пострадали.
	if other, err := s.Search(ctx, "tag:срочно", SearchOptions{}); err != nil || len(other) != 1 {
		t.Errorf("тег «срочно» = %d: %v", len(other), err)
	}
}

// Заметка несла и старый, и новый тег: после переименования он должен остаться
// один, а не задвоиться.
func TestRenameTagMerges(t *testing.T) {
	s, root := testService(t, vault.OriginUser)
	ctx := context.Background()
	created, err := s.Create(ctx, CreateParams{Title: "Обе", Tags: []string{"баг", "дефект"}})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.RenameTag(ctx, "баг", "дефект"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "дефект" {
		t.Errorf("теги = %v, ожидался один «дефект»", got.Tags)
	}

	// Проверяем по файлу, а не только по индексу: в note_tags стоит
	// INSERT OR IGNORE, и дубль там схлопнется, оставшись в frontmatter.
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(got.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "tags: [дефект]") {
		t.Errorf("во frontmatter теги задвоились:\n%s", raw)
	}
}

// Латиница в тегах регистронезависима, поэтому BUG должен переименоваться
// вместе с bug (см. TestSearchTagCaseFolding в internal/index).
func TestRenameTagAsciiCase(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()
	created, err := s.Create(ctx, CreateParams{Title: "Заметка", Tags: []string{"BUG"}})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.RenameTag(ctx, "bug", "defect"); err != nil {
		t.Fatalf("RenameTag: %v", err)
	}
	got, err := s.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "defect" {
		t.Errorf("теги = %v", got.Tags)
	}
}

func TestRenameTagEdgeCases(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()
	if _, err := s.Create(ctx, CreateParams{Title: "Заметка", Tags: []string{"баг"}}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.RenameTag(ctx, "  ", "дефект"); !errors.Is(err, ErrEmptyTag) {
		t.Errorf("пустое имя: %v", err)
	}
	if _, err := s.RenameTag(ctx, "баг", ""); !errors.Is(err, ErrEmptyTag) {
		t.Errorf("пустое новое имя: %v", err)
	}
	if updated, err := s.RenameTag(ctx, "баг", "баг"); err != nil || len(updated) != 0 {
		t.Errorf("переименование в себя: %+v, %v", updated, err)
	}
	if updated, err := s.RenameTag(ctx, "которого-нет", "новый"); err != nil || len(updated) != 0 {
		t.Errorf("несуществующий тег: %+v, %v", updated, err)
	}
}

func TestSetTags(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()
	created, err := s.Create(ctx, CreateParams{Title: "Заметка", Tags: []string{"старый", "тоже старый"}})
	if err != nil {
		t.Fatal(err)
	}

	// Замена целиком, а не добавление.
	updated, err := s.SetTags(ctx, created.ID, []string{"новый"})
	if err != nil {
		t.Fatalf("SetTags: %v", err)
	}
	if len(updated.Tags) != 1 || updated.Tags[0] != "новый" {
		t.Errorf("теги = %v", updated.Tags)
	}

	// Пустые и повторы отбрасываются, регистр учитывается как в поиске.
	cleaned, err := s.SetTags(ctx, created.ID, []string{" один ", "", "  ", "один", "ОДИН", "два"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cleaned.Tags) != 2 {
		t.Errorf("теги = %v, ожидались два", cleaned.Tags)
	}
}

func TestTagColors(t *testing.T) {
	s, root := testService(t, vault.OriginUser)
	ctx := context.Background()
	if _, err := s.Create(ctx, CreateParams{Title: "Заметка", Tags: []string{"баг", "работа"}}); err != nil {
		t.Fatal(err)
	}

	if err := s.SetTagColor(ctx, "баг", 5); err != nil {
		t.Fatalf("SetTagColor: %v", err)
	}
	if got := s.TagColors()["баг"]; got != 5 {
		t.Errorf("цвет = %d", got)
	}

	// Файл лежит рядом с заметками, а не внутри индекса.
	if _, err := os.Stat(filepath.Join(root, ".tasker", "tags.json")); err != nil {
		t.Errorf("файла цветов нет: %v", err)
	}

	tags, err := s.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]string{}
	for _, tag := range tags {
		byName[tag.Name] = tag.Color
	}
	if byName["баг"] != "5" {
		t.Errorf("в индексе цвет = %q", byName["баг"])
	}
	if byName["работа"] != "default" {
		t.Errorf("нетронутый тег получил цвет %q", byName["работа"])
	}

	// Снятие цвета возвращает тег к автоматическому.
	if err := s.SetTagColor(ctx, "баг", AutoColor); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.TagColors()["баг"]; ok {
		t.Error("цвет остался в файле после снятия")
	}
	// И в индексе тоже: сброс должен доехать туда, а не остаться от прошлого
	// применения.
	after, err := s.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range after {
		if tag.Color != "default" {
			t.Errorf("в индексе тег %q остался с цветом %q", tag.Name, tag.Color)
		}
	}
}

// Главное, ради чего цвета вынесены в файл: они переживают пересборку индекса.
func TestTagColorsSurviveIndexRebuild(t *testing.T) {
	s, root := testService(t, vault.OriginUser)
	ctx := context.Background()
	if _, err := s.Create(ctx, CreateParams{Title: "Заметка", Tags: []string{"баг"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTagColor(ctx, "баг", 9); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Сносим индекс целиком — ровно то, что делает смена версии схемы.
	if err := os.Remove(filepath.Join(root, ".tasker", "index.sqlite")); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, root, Options{Origin: vault.OriginUser})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := reopened.Sync(ctx); err != nil {
		t.Fatal(err)
	}

	tags, err := reopened.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0].Color != "9" {
		t.Errorf("после пересборки теги = %+v", tags)
	}
}

func TestTagColorsRejectOutsidePalette(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()
	for _, color := range []int{TagPalette, TagPalette + 5, -2} {
		if err := s.SetTagColor(ctx, "баг", color); !errors.Is(err, ErrBadColor) {
			t.Errorf("цвет %d: ошибка = %v", color, err)
		}
	}
}

// Испорченный файл не должен ронять запуск и стирать уцелевшее.
func TestTagColorsSurviveCorruptFile(t *testing.T) {
	s, root := testService(t, vault.OriginUser)
	path := filepath.Join(root, ".tasker", "tags.json")
	if err := os.WriteFile(path, []byte(`{"colors":{"баг":5,"плохой":99,"":3}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	colors := s.TagColors()
	if colors["баг"] != 5 {
		t.Errorf("годный цвет потерян: %v", colors)
	}
	if _, ok := colors["плохой"]; ok {
		t.Error("цвет вне палитры принят")
	}
	if _, ok := colors[""]; ok {
		t.Error("пустое имя принято")
	}

	if err := os.WriteFile(path, []byte("не json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if len(s.TagColors()) != 0 {
		t.Error("из битого файла что-то прочиталось")
	}
	if err := s.SetTagColor(context.Background(), "баг", 1); err != nil {
		t.Errorf("запись поверх битого файла: %v", err)
	}
}

// Пустой ноутбук должен быть виден: иначе создать его некуда.
func TestCreateEmptyNotebookIsVisible(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()

	if err := s.CreateNotebook(ctx, "Работа/Идеи"); err != nil {
		t.Fatalf("CreateNotebook: %v", err)
	}

	books, err := s.Notebooks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]int{}
	for _, book := range books {
		paths[book.Path] = book.Count
	}
	if _, ok := paths["Работа/Идеи"]; !ok {
		t.Errorf("пустой ноутбук не виден: %v", paths)
	}
	if _, ok := paths["Работа"]; !ok {
		t.Errorf("промежуточный ноутбук не виден: %v", paths)
	}
	if paths["Работа/Идеи"] != 0 {
		t.Errorf("в пустом ноутбуке %d заметок", paths["Работа/Идеи"])
	}
}

func TestCreateNotebookRejectsBadPaths(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()
	for _, path := range []string{"../снаружи", "/etc", ".git"} {
		if err := s.CreateNotebook(ctx, path); err == nil {
			t.Errorf("ноутбук %q принят", path)
		}
	}
}

// Служебные каталоги ноутбуками не считаются.
func TestNotebooksSkipSpecialDirs(t *testing.T) {
	s, root := testService(t, vault.OriginUser)
	ctx := context.Background()
	for _, dir := range []string{".trash", "attachments/2026/08", ".git/objects"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	books, err := s.Notebooks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, book := range books {
		if strings.HasPrefix(book.Path, ".") || strings.HasPrefix(book.Path, "attachments") {
			t.Errorf("служебный каталог показан ноутбуком: %q", book.Path)
		}
	}
}

func TestRenameNotebook(t *testing.T) {
	s, root := testService(t, vault.OriginUser)
	ctx := context.Background()
	if _, err := s.Create(ctx, CreateParams{Title: "Внутри", Notebook: "Работа"}); err != nil {
		t.Fatal(err)
	}
	// Два уровня вложенности, а не один: порядок удаления опустевших каталогов
	// виден только на глубине — сверху вниз родитель не удалится.
	if _, err := s.Create(ctx, CreateParams{Title: "Глубже", Notebook: "Работа/Баги/Старые"}); err != nil {
		t.Fatal(err)
	}
	before := commitCount(t, root)

	moved, err := s.RenameNotebook(ctx, "Работа", "Дела")
	if err != nil {
		t.Fatalf("RenameNotebook: %v", err)
	}
	if len(moved) != 2 {
		t.Fatalf("перенесено %d заметок", len(moved))
	}
	if added := commitCount(t, root) - before; added != 1 {
		t.Errorf("коммитов %d, ожидался один", added)
	}

	books := map[string]bool{}
	list, err := s.Notebooks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, book := range list {
		books[book.Path] = true
	}
	if books["Работа"] || books["Работа/Баги"] || books["Работа/Баги/Старые"] {
		t.Errorf("старое имя осталось: %v", books)
	}
	// Вложенные ноутбуки переезжают вместе с родителем.
	if !books["Дела"] || !books["Дела/Баги"] || !books["Дела/Баги/Старые"] {
		t.Errorf("новое дерево не собралось: %v", books)
	}
}

func TestDeleteNotebookMovesToTrash(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()
	created, err := s.Create(ctx, CreateParams{Title: "Внутри", Notebook: "Ненужный"})
	if err != nil {
		t.Fatal(err)
	}

	trashed, err := s.DeleteNotebook(ctx, "Ненужный")
	if err != nil {
		t.Fatalf("DeleteNotebook: %v", err)
	}
	if len(trashed) != 1 {
		t.Fatalf("в корзину уехало %d", len(trashed))
	}

	// Заметка не пропала — её можно вернуть.
	got, err := s.Index().Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("заметка исчезла: %v", err)
	}
	if !got.Trashed {
		t.Error("заметка не помечена удалённой")
	}

	list, err := s.Notebooks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, book := range list {
		if book.Path == "Ненужный" {
			t.Error("папка удалённого ноутбука осталась")
		}
	}
}

// Корень удалить или переименовать нельзя.
func TestNotebookOpsRejectRoot(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()
	if _, err := s.RenameNotebook(ctx, "", "Новый"); err == nil {
		t.Error("корень переименован")
	}
	if _, err := s.DeleteNotebook(ctx, ""); err == nil {
		t.Error("корень удалён")
	}
}

// Тег должно быть можно не только переименовать, но и убрать.
//
// Отдельного списка тегов нет: тег живёт ровно столько, сколько стоит хотя бы
// в одной заметке, — поэтому удаление это снятие его со всех, одним коммитом.
func TestDeleteTag(t *testing.T) {
	s, root := testService(t, vault.OriginUser)
	ctx := context.Background()

	for _, tags := range [][]string{{"баг", "срочно"}, {"баг"}, {"идея"}} {
		if _, err := s.Create(ctx, CreateParams{Title: "Заметка " + tags[0], Tags: tags}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetTagColor(ctx, "баг", 3); err != nil {
		t.Fatal(err)
	}
	before := commitCount(t, root)

	updated, err := s.DeleteTag(ctx, "баг")
	if err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	if len(updated) != 2 {
		t.Fatalf("тронуто %d заметок, ожидалось 2", len(updated))
	}
	if added := commitCount(t, root) - before; added != 1 {
		t.Errorf("коммитов добавилось %d, ожидался один", added)
	}

	if gone, err := s.Search(ctx, "tag:баг", SearchOptions{}); err != nil || len(gone) != 0 {
		t.Errorf("по удалённому тегу нашлось %d: %v", len(gone), err)
	}
	// Соседние теги в тех же заметках не пострадали.
	if other, err := s.Search(ctx, "tag:срочно", SearchOptions{}); err != nil || len(other) != 1 {
		t.Errorf("тег «срочно» = %d: %v", len(other), err)
	}
	if other, err := s.Search(ctx, "tag:идея", SearchOptions{}); err != nil || len(other) != 1 {
		t.Errorf("тег «идея» = %d: %v", len(other), err)
	}
	// Тег исчез из списка целиком, а не остался с нулевым счётчиком.
	tags, err := s.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range tags {
		if tag.Name == "баг" {
			t.Errorf("тег остался в списке: %+v", tags)
		}
	}

	// Выбранный руками цвет пережил бы удаление и достался бы новому тегу с
	// тем же именем — а человек его для этого тега не выбирал.
	if color, ok := s.TagColors()["баг"]; ok {
		t.Errorf("цвет удалённого тега остался: %d", color)
	}
}

func TestDeleteTagEdgeCases(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()
	if _, err := s.Create(ctx, CreateParams{Title: "Заметка", Tags: []string{"BUG"}}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.DeleteTag(ctx, "  "); !errors.Is(err, ErrEmptyTag) {
		t.Errorf("пустое имя: %v", err)
	}
	if updated, err := s.DeleteTag(ctx, "которого-нет"); err != nil || len(updated) != 0 {
		t.Errorf("несуществующий тег: %+v, %v", updated, err)
	}
	// Латиница в тегах регистронезависима — bug обязан убрать BUG.
	updated, err := s.DeleteTag(ctx, "bug")
	if err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	if len(updated) != 1 || len(updated[0].Tags) != 0 {
		t.Errorf("после удаления: %+v", updated)
	}
}

// Удаление тега достаёт и до корзины.
//
// Иначе тег воскресает дважды: ближайшая сверка возвращает его в сайдбар, —
// файлы источник правды, а на диске он остался, — и восстановленная заметка
// приносит его обратно.
func TestDeleteTagReachesTrash(t *testing.T) {
	s, _ := testService(t, vault.OriginUser)
	ctx := context.Background()

	created, err := s.Create(ctx, CreateParams{Title: "Удалённая", Tags: []string{"баг"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Trash(ctx, created.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := s.DeleteTag(ctx, "баг"); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}

	// Сверка перечитывает диск: если бы тег остался в файле, он вернулся бы сюда.
	if _, err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	tags, err := s.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 0 {
		t.Errorf("после сверки теги = %+v, ожидался пустой список", tags)
	}

	restored, err := s.Restore(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, restored.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tags) != 0 {
		t.Errorf("восстановленная заметка принесла тег обратно: %v", got.Tags)
	}
}
