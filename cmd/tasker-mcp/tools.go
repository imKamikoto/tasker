package main

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"tasker/internal/index"
	"tasker/internal/notes"
	"tasker/internal/vault"
)

// maxLimit — потолок выдачи. Больше сотни заметок агенту в один ответ не нужно,
// а контекст он ими забьёт (docs/MCP.md §3).
const maxLimit = 100

const defaultLimit = 20

// NoteSummary — заметка в ответе инструмента (docs/MCP.md §3).
type NoteSummary struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Notebook string   `json:"notebook"`
	Status   string   `json:"status"`
	Tags     []string `json:"tags"`
	Pinned   bool     `json:"pinned"`
	Created  string   `json:"created"`
	Updated  string   `json:"updated"`
	Excerpt  string   `json:"excerpt"`
	URL      string   `json:"url"`
	Body     string   `json:"body,omitempty"`
}

func summarize(n notes.Note) NoteSummary {
	tags := n.Tags
	if tags == nil {
		tags = []string{}
	}
	return NoteSummary{
		ID:       n.ID,
		Title:    n.Title,
		Notebook: n.Notebook,
		Status:   n.Status,
		Tags:     tags,
		Pinned:   n.Pinned,
		Created:  n.Created.Format(time.RFC3339),
		Updated:  n.Updated.Format(time.RFC3339),
		Excerpt:  n.Excerpt,
		URL:      noteURL(n.ID),
		Body:     n.Body,
	}
}

func summarizeRecord(r index.Record) NoteSummary {
	return summarize(notes.Note{Record: r})
}

func noteURL(id string) string { return "tasker://note/" + id }

func clampLimit(limit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

// register вешает на сервер все инструменты из docs/MCP.md §3.
func register(server *mcp.Server, svc *notes.Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "search_notes",
		Description: "Найти заметки. Язык запросов: слово, \"точная фраза\", " +
			"book:Работа, tag:баг, status:active, title:, body:, is:pinned, has:task, " +
			"отрицание через минус, пробел означает И.",
	}, wrap(svc, searchNotes))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_note",
		Description: "Прочитать заметку целиком: тело, исходящие ссылки и обратные ссылки на неё.",
	}, wrap(svc, getNote))

	mcp.AddTool(server, &mcp.Tool{
		Name: "create_note",
		Description: "Завести заметку. Ноутбук и теги создаются, если их нет. " +
			"Заметка помечается origin: agent.",
	}, wrap(svc, createNote))

	mcp.AddTool(server, &mcp.Tool{
		Name: "update_note",
		Description: "Изменить заметку. Передавайте только те поля, которые надо поменять; " +
			"остальные останутся прежними. Для дописывания в конец используйте append, " +
			"а не body — body заменяет тело целиком.",
	}, wrap(svc, updateNote))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_status",
		Description: "Поменять статус заметки.",
	}, wrap(svc, setStatus))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_tasks",
		Description: "Что сейчас в работе. По умолчанию статусы active и onHold.",
	}, wrap(svc, listTasks))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_notebooks",
		Description: "Дерево ноутбуков со счётчиками заметок.",
	}, wrap(svc, listNotebooks))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_tags",
		Description: "Теги со счётчиками заметок.",
	}, wrap(svc, listTags))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "trash_note",
		Description: "Перенести заметку в корзину. Удалить насовсем нельзя.",
	}, wrap(svc, trashNote))
}

// wrap синхронизирует индекс перед каждым вызовом.
//
// Приложение может быть закрыто, а заметки — правиться руками или другим
// агентом. Скан инкрементальный и стоит миллисекунды на сотне заметок, так что
// платить за него на каждом вызове дешевле, чем отвечать по устаревшему
// индексу (SPEC §5.2).
func wrap[In, Out any](
	svc *notes.Service,
	fn func(context.Context, *notes.Service, In) (Out, error),
) mcp.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		var zero Out
		if _, err := svc.Sync(ctx); err != nil {
			return nil, zero, err
		}
		out, err := fn(ctx, svc, in)
		if err != nil {
			return nil, zero, err
		}
		return nil, out, nil
	}
}

type SearchParams struct {
	Query       string `json:"query" jsonschema:"запрос на языке поиска, пустой означает все заметки"`
	Limit       int    `json:"limit,omitempty" jsonschema:"по умолчанию 20, максимум 100"`
	IncludeBody bool   `json:"includeBody,omitempty" jsonschema:"вернуть тело каждой заметки целиком"`
}

type SearchResult struct {
	Total int           `json:"total"`
	Notes []NoteSummary `json:"notes"`
}

func searchNotes(ctx context.Context, svc *notes.Service, p SearchParams) (SearchResult, error) {
	found, err := svc.Search(ctx, p.Query, notes.SearchOptions{
		Limit:       clampLimit(p.Limit),
		IncludeBody: p.IncludeBody,
	})
	if err != nil {
		return SearchResult{}, err
	}
	return SearchResult{Total: len(found), Notes: summarizeAll(found)}, nil
}

type GetParams struct {
	ID string `json:"id" jsonschema:"ULID заметки"`
}

type GetResult struct {
	NoteSummary
	Links     []NoteSummary `json:"links"`
	Backlinks []NoteSummary `json:"backlinks"`
}

func getNote(ctx context.Context, svc *notes.Service, p GetParams) (GetResult, error) {
	n, err := svc.Get(ctx, p.ID)
	if err != nil {
		return GetResult{}, err
	}
	out := GetResult{NoteSummary: summarize(n)}
	out.Body = n.Body
	for _, l := range n.Links {
		out.Links = append(out.Links, summarizeRecord(l))
	}
	for _, b := range n.Backlinks {
		out.Backlinks = append(out.Backlinks, summarizeRecord(b))
	}
	return out, nil
}

type CreateParams struct {
	Title    string   `json:"title" jsonschema:"заголовок, конкретный и самодостаточный"`
	Body     string   `json:"body,omitempty" jsonschema:"тело в markdown: что не так, где, как воспроизвести, почему не чиним сейчас"`
	Notebook string   `json:"notebook,omitempty" jsonschema:"например Работа/Баги, создаётся если нет"`
	Tags     []string `json:"tags,omitempty" jsonschema:"несуществующие создаются"`
	Status   string   `json:"status,omitempty" jsonschema:"none, active, onHold, completed или dropped"`
	Pinned   bool     `json:"pinned,omitempty" jsonschema:"закрепить наверху списка"`
	LinkTo   string   `json:"linkTo,omitempty" jsonschema:"id заметки: добавит взаимные ссылки"`
	Context  *Context `json:"context,omitempty" jsonschema:"откуда пришла заметка"`
}

type Context struct {
	Repo   string `json:"repo,omitempty" jsonschema:"имя репозитория, над которым шла работа"`
	Branch string `json:"branch,omitempty" jsonschema:"текущая ветка"`
	Commit string `json:"commit,omitempty" jsonschema:"текущий коммит, короткий хеш"`
	File   string `json:"file,omitempty" jsonschema:"файл, в котором нашлась проблема"`
}

type CreateResult struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	URL  string `json:"url"`
}

func createNote(ctx context.Context, svc *notes.Service, p CreateParams) (CreateResult, error) {
	status, err := vault.ParseStatus(p.Status)
	if err != nil {
		return CreateResult{}, err
	}

	var noteCtx *vault.Context
	if p.Context != nil {
		noteCtx = &vault.Context{
			Repo: p.Context.Repo, Branch: p.Context.Branch,
			Commit: p.Context.Commit, File: p.Context.File,
		}
	}

	rec, err := svc.Create(ctx, notes.CreateParams{
		Title: p.Title, Body: p.Body, Notebook: p.Notebook, Tags: p.Tags,
		Status: status, Pinned: p.Pinned, LinkTo: p.LinkTo, Context: noteCtx,
	})
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{ID: rec.ID, Path: rec.Path, URL: noteURL(rec.ID)}, nil
}

// UpdateParams — указатели, а не значения: иначе не отличить «не передано» от
// «передан ноль», и обновление сбрасывало бы всё, о чём не упомянули.
type UpdateParams struct {
	ID         string   `json:"id" jsonschema:"ULID заметки"`
	Title      *string  `json:"title,omitempty" jsonschema:"новый заголовок"`
	Body       *string  `json:"body,omitempty" jsonschema:"заменяет тело целиком, взаимоисключающе с append и prepend"`
	Append     *string  `json:"append,omitempty" jsonschema:"дописать в конец отдельным абзацем"`
	Prepend    *string  `json:"prepend,omitempty" jsonschema:"дописать в начало отдельным абзацем"`
	Status     *string  `json:"status,omitempty" jsonschema:"none, active, onHold, completed или dropped"`
	AddTags    []string `json:"addTags,omitempty" jsonschema:"добавить теги, существующие не дублируются"`
	RemoveTags []string `json:"removeTags,omitempty" jsonschema:"убрать теги"`
	Pinned     *bool    `json:"pinned,omitempty" jsonschema:"закрепить или открепить"`
	Notebook   *string  `json:"notebook,omitempty" jsonschema:"перемещает заметку"`
}

func updateNote(ctx context.Context, svc *notes.Service, p UpdateParams) (NoteSummary, error) {
	params := notes.UpdateParams{
		ID: p.ID, Title: p.Title, Body: p.Body, Append: p.Append, Prepend: p.Prepend,
		AddTags: p.AddTags, RemoveTags: p.RemoveTags, Pinned: p.Pinned, Notebook: p.Notebook,
	}
	if p.Status != nil {
		status, err := vault.ParseStatus(*p.Status)
		if err != nil {
			return NoteSummary{}, err
		}
		params.Status = &status
	}

	rec, err := svc.Update(ctx, params)
	if err != nil {
		return NoteSummary{}, err
	}
	return summarizeRecord(rec), nil
}

type StatusParams struct {
	ID     string `json:"id" jsonschema:"ULID заметки"`
	Status string `json:"status" jsonschema:"none, active, onHold, completed или dropped"`
}

func setStatus(ctx context.Context, svc *notes.Service, p StatusParams) (NoteSummary, error) {
	status, err := vault.ParseStatus(p.Status)
	if err != nil {
		return NoteSummary{}, err
	}
	rec, err := svc.SetStatus(ctx, p.ID, status)
	if err != nil {
		return NoteSummary{}, err
	}
	return summarizeRecord(rec), nil
}

type TasksParams struct {
	Status   []string `json:"status,omitempty" jsonschema:"по умолчанию active и onHold"`
	Notebook string   `json:"notebook,omitempty" jsonschema:"только из этого ноутбука, с вложенными"`
	Tag      string   `json:"tag,omitempty" jsonschema:"только с этим тегом"`
	Limit    int      `json:"limit,omitempty" jsonschema:"по умолчанию 20, максимум 100"`
}

func listTasks(ctx context.Context, svc *notes.Service, p TasksParams) (SearchResult, error) {
	statuses := make([]vault.Status, 0, len(p.Status))
	for _, raw := range p.Status {
		status, err := vault.ParseStatus(raw)
		if err != nil {
			return SearchResult{}, err
		}
		statuses = append(statuses, status)
	}

	found, err := svc.Tasks(ctx, notes.TasksParams{
		Status: statuses, Notebook: p.Notebook, Tag: p.Tag, Limit: clampLimit(p.Limit),
	})
	if err != nil {
		return SearchResult{}, err
	}
	return SearchResult{Total: len(found), Notes: summarizeAll(found)}, nil
}

type Empty struct{}

type Notebook struct {
	Path     string   `json:"path"`
	Count    int      `json:"count"`
	Children []string `json:"children"`
}

type NotebooksResult struct {
	Notebooks []Notebook `json:"notebooks"`
}

func listNotebooks(ctx context.Context, svc *notes.Service, _ Empty) (NotebooksResult, error) {
	books, err := svc.Notebooks(ctx)
	if err != nil {
		return NotebooksResult{}, err
	}
	out := NotebooksResult{Notebooks: make([]Notebook, 0, len(books))}
	for _, b := range books {
		children := b.Children
		if children == nil {
			children = []string{}
		}
		out.Notebooks = append(out.Notebooks, Notebook{Path: b.Path, Count: b.Count, Children: children})
	}
	return out, nil
}

type Tag struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Color string `json:"color"`
}

type TagsResult struct {
	Tags []Tag `json:"tags"`
}

func listTags(ctx context.Context, svc *notes.Service, _ Empty) (TagsResult, error) {
	tags, err := svc.Tags(ctx)
	if err != nil {
		return TagsResult{}, err
	}
	out := TagsResult{Tags: make([]Tag, 0, len(tags))}
	for _, t := range tags {
		out.Tags = append(out.Tags, Tag{Name: t.Name, Count: t.Count, Color: t.Color})
	}
	return out, nil
}

type TrashParams struct {
	ID string `json:"id" jsonschema:"ULID заметки"`
}

type TrashResult struct {
	ID      string `json:"id"`
	Trashed bool   `json:"trashed"`
}

func trashNote(ctx context.Context, svc *notes.Service, p TrashParams) (TrashResult, error) {
	if err := svc.Trash(ctx, p.ID); err != nil {
		return TrashResult{}, fmt.Errorf("trash %s: %w", p.ID, err)
	}
	return TrashResult{ID: p.ID, Trashed: true}, nil
}

func summarizeAll(found []notes.Note) []NoteSummary {
	out := make([]NoteSummary, 0, len(found))
	for _, n := range found {
		out = append(out, summarize(n))
	}
	return out
}
