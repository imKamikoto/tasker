package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"tasker/internal/vault"
)

// session поднимает настоящий сервер и настоящего клиента через транспорт в
// памяти: проверяем то, что увидит Claude Code, а не внутренние вызовы.
func session(t *testing.T) (*mcp.ClientSession, string) {
	t.Helper()
	root := t.TempDir()
	ctx := context.Background()

	// Через newService, а не notes.Open напрямую: иначе пометка origin: agent
	// живёт в main.go и ничем не проверяется.
	svc, err := newService(ctx, root)
	if err != nil {
		t.Fatalf("newService: %v", err)
	}
	t.Cleanup(func() { svc.Close() })

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	server := newServer(svc)
	go func() {
		if err := server.Run(ctx, serverTransport); err != nil {
			t.Logf("server.Run: %v", err)
		}
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "тест", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs, root
}

// call вызывает инструмент и разбирает структурированный ответ.
func call[T any](t *testing.T, cs *mcp.ClientSession, name string, args any) T {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("%s вернул ошибку: %s", name, textOf(res))
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%s: разбор ответа: %v (%s)", name, err, raw)
	}
	return out
}

// callErr ожидает, что инструмент откажет.
func callErr(t *testing.T, cs *mcp.ClientSession, name string, args any) string {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return err.Error()
	}
	if !res.IsError {
		t.Fatalf("%s не вернул ошибку", name)
	}
	return textOf(res)
}

func textOf(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// Все девять инструментов из docs/MCP.md §3 должны быть объявлены.
func TestToolsAreRegistered(t *testing.T) {
	cs, _ := session(t)

	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	got := map[string]bool{}
	for _, tool := range res.Tools {
		got[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("у инструмента %q нет описания — агент не поймёт, когда его звать", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("у инструмента %q нет схемы параметров", tool.Name)
		}
	}
	for _, want := range []string{
		"search_notes", "get_note", "create_note", "update_note", "set_status",
		"list_tasks", "list_notebooks", "list_tags", "trash_note",
	} {
		if !got[want] {
			t.Errorf("нет инструмента %q", want)
		}
	}
	if len(res.Tools) != 9 {
		t.Errorf("инструментов %d, ожидалось 9", len(res.Tools))
	}
}

// Сценарий приёмки из docs/MCP.md §6, шаги 1–4.
func TestAcceptanceScenario(t *testing.T) {
	cs, root := session(t)

	created := call[CreateResult](t, cs, "create_note", CreateParams{
		Title:    "Ломается экранирование кавычек в парсере",
		Body:     "Что не так: парсер срывается на вложенных кавычках.\n\nПочему не чиним сейчас: заняты другой задачей.\n",
		Notebook: "Работа/Баги",
		Tags:     []string{"tasker"},
		Status:   "active",
		Context:  &Context{Repo: "tasker", Branch: "main", Commit: "3f9a1c2"},
	})

	if !vault.ValidID(created.ID) {
		t.Fatalf("id = %q", created.ID)
	}
	if created.URL != "tasker://note/"+created.ID {
		t.Errorf("url = %q", created.URL)
	}

	// Шаг 3: файл на диске с корректным frontmatter, индекс обновлён, коммит есть.
	want := filepath.Join(root, "Работа", "Баги", "lomaetsya-ekranirovanie-kavychek-v-parsere.md")
	raw, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("файла нет по ожидаемому пути %s: %v", want, err)
	}
	for _, fragment := range []string{
		"title: Ломается экранирование кавычек в парсере",
		"status: active",
		"origin: agent",
		"repo: tasker",
		"commit: 3f9a1c2",
	} {
		if !strings.Contains(string(raw), fragment) {
			t.Errorf("во frontmatter нет %q:\n%s", fragment, raw)
		}
	}

	log := gitLog(t, root)
	if !strings.Contains(log, `agent: create "Ломается экранирование кавычек в парсере"`) {
		t.Errorf("коммита агента нет:\n%s", log)
	}

	// Заметка находится поиском и попадает в список задач.
	found := call[SearchResult](t, cs, "search_notes", SearchParams{Query: "экранирован tag:tasker"})
	if found.Total != 1 || found.Notes[0].ID != created.ID {
		t.Errorf("поиск нашёл %+v", found)
	}
	tasks := call[SearchResult](t, cs, "list_tasks", TasksParams{})
	if tasks.Total != 1 {
		t.Errorf("в работе %d задач, ожидалась одна", tasks.Total)
	}

	// Шаги 6–7: пользователь отмечает выполненной — из списка задач уходит.
	call[NoteSummary](t, cs, "set_status", StatusParams{ID: created.ID, Status: "completed"})
	tasks = call[SearchResult](t, cs, "list_tasks", TasksParams{})
	if tasks.Total != 0 {
		t.Errorf("завершённая задача осталась в списке: %+v", tasks)
	}
}

func TestGetNote(t *testing.T) {
	cs, _ := session(t)

	first := call[CreateResult](t, cs, "create_note", CreateParams{Title: "Первая", Body: "тело первой\n"})
	second := call[CreateResult](t, cs, "create_note", CreateParams{
		Title: "Вторая", Body: "тело второй\n", LinkTo: first.ID,
	})

	got := call[GetResult](t, cs, "get_note", GetParams{ID: first.ID})
	if !strings.Contains(got.Body, "тело первой") {
		t.Errorf("тело = %q", got.Body)
	}
	if len(got.Backlinks) != 1 || got.Backlinks[0].ID != second.ID {
		t.Errorf("бэклинки = %+v", got.Backlinks)
	}
	if len(got.Links) != 1 || got.Links[0].ID != second.ID {
		t.Errorf("ссылки = %+v", got.Links)
	}
}

// Указатели в update_note: не переданное поле не трогается.
func TestUpdateNoteKeepsUntouchedFields(t *testing.T) {
	cs, _ := session(t)

	created := call[CreateResult](t, cs, "create_note", CreateParams{
		Title: "Исходный", Body: "тело\n", Tags: []string{"один"}, Status: "active", Pinned: true,
	})

	updated := call[NoteSummary](t, cs, "update_note", map[string]any{
		"id": created.ID, "append": "дописанное агентом",
	})
	if updated.Title != "Исходный" || updated.Status != "active" || !updated.Pinned {
		t.Errorf("непереданные поля изменились: %+v", updated)
	}
	if len(updated.Tags) != 1 {
		t.Errorf("теги = %v", updated.Tags)
	}

	got := call[GetResult](t, cs, "get_note", GetParams{ID: created.ID})
	if got.Body != "тело\n\nдописанное агентом\n" {
		t.Errorf("тело = %q", got.Body)
	}
}

func TestUpdateNoteRejectsConflictingFields(t *testing.T) {
	cs, _ := session(t)
	created := call[CreateResult](t, cs, "create_note", CreateParams{Title: "Заметка"})

	msg := callErr(t, cs, "update_note", map[string]any{
		"id": created.ID, "body": "целиком", "append": "и дописать",
	})
	if !strings.Contains(msg, "conflicting") {
		t.Errorf("сообщение об ошибке невнятное: %q", msg)
	}
}

func TestTrashNote(t *testing.T) {
	cs, _ := session(t)
	created := call[CreateResult](t, cs, "create_note", CreateParams{Title: "Ненужная"})

	res := call[TrashResult](t, cs, "trash_note", TrashParams{ID: created.ID})
	if !res.Trashed || res.ID != created.ID {
		t.Errorf("ответ = %+v", res)
	}
	found := call[SearchResult](t, cs, "search_notes", SearchParams{})
	if found.Total != 0 {
		t.Errorf("удалённая всё ещё ищется: %+v", found)
	}
}

func TestListNotebooksAndTags(t *testing.T) {
	cs, _ := session(t)
	call[CreateResult](t, cs, "create_note", CreateParams{
		Title: "Заметка", Notebook: "Работа/Баги", Tags: []string{"баг", "срочно"},
	})

	books := call[NotebooksResult](t, cs, "list_notebooks", Empty{})
	var paths []string
	for _, b := range books.Notebooks {
		paths = append(paths, b.Path)
	}
	if !strings.Contains(strings.Join(paths, ","), "Работа/Баги") {
		t.Errorf("ноутбуки = %v", paths)
	}

	tags := call[TagsResult](t, cs, "list_tags", Empty{})
	if len(tags.Tags) != 2 {
		t.Errorf("теги = %+v", tags.Tags)
	}
}

// Выдумка в статусе должна дать понятный отказ, а не тихо сохраниться.
func TestInvalidStatusRejected(t *testing.T) {
	cs, _ := session(t)
	created := call[CreateResult](t, cs, "create_note", CreateParams{Title: "Заметка"})

	if msg := callErr(t, cs, "set_status", StatusParams{ID: created.ID, Status: "почтиГотово"}); msg == "" {
		t.Error("пустое сообщение об ошибке")
	}
	if msg := callErr(t, cs, "create_note", CreateParams{Title: "Другая", Status: "выдуманный"}); msg == "" {
		t.Error("пустое сообщение об ошибке")
	}
}

func TestUnknownNoteRejected(t *testing.T) {
	cs, _ := session(t)
	for _, name := range []string{"get_note", "trash_note"} {
		if msg := callErr(t, cs, name, map[string]any{"id": "01K3QF8ZN7X2WPBV4YHMC6TDAE"}); msg == "" {
			t.Errorf("%s: пустое сообщение об ошибке", name)
		}
	}
}

// Заметка, положенная в vault руками, должна найтись без перезапуска сервера:
// индекс сверяется перед каждым вызовом.
func TestPicksUpExternalChanges(t *testing.T) {
	cs, root := session(t)

	if err := os.WriteFile(filepath.Join(root, "снаружи.md"),
		[]byte("---\nid: 01K3QF8ZN7X2WPBV4YHMC6TDAE\ntitle: Извне\n---\nтело\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	found := call[SearchResult](t, cs, "search_notes", SearchParams{Query: "title:Извне"})
	if found.Total != 1 {
		t.Errorf("внешняя заметка не найдена: %+v", found)
	}
}

func TestLimitIsCapped(t *testing.T) {
	cs, _ := session(t)
	for i := 0; i < 3; i++ {
		call[CreateResult](t, cs, "create_note", CreateParams{Title: "Заметка " + strings.Repeat("х", i+1)})
	}
	found := call[SearchResult](t, cs, "search_notes", SearchParams{Limit: 1000})
	if found.Total != 3 {
		t.Errorf("найдено %d", found.Total)
	}
	found = call[SearchResult](t, cs, "search_notes", SearchParams{Limit: 2})
	if found.Total != 2 {
		t.Errorf("лимит не соблюдён: %d", found.Total)
	}
}

func TestRunRequiresVault(t *testing.T) {
	var stderr strings.Builder
	if err := run(context.Background(), nil, &stderr); err == nil {
		t.Error("ожидалась ошибка без --vault")
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Errorf("нет подсказки:\n%s", stderr.String())
	}
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

// Потолок выдачи проверяем прямо: чтобы поймать его сквозным тестом, пришлось
// бы заводить сотню заметок ради одного числа.
func TestClampLimit(t *testing.T) {
	cases := map[int]int{0: defaultLimit, -5: defaultLimit, 1: 1, 20: 20, 100: 100, 101: maxLimit, 1000: maxLimit}
	for in, want := range cases {
		if got := clampLimit(in); got != want {
			t.Errorf("clampLimit(%d) = %d, ожидалось %d", in, got, want)
		}
	}
}
