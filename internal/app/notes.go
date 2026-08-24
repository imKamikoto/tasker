package app

import (
	"context"
	"encoding/base64"
	"fmt"

	"tasker/internal/index"
	"tasker/internal/notes"
	"tasker/internal/vault"
)

// Notes — сервис Wails поверх internal/notes.
//
// Тонкая обёртка и ничего больше: каждый метод разбирает аргументы и зовёт
// слой ниже. Бизнес-правила живут в internal/notes, потому что тот же код
// нужен tasker-mcp, который про Wails не знает (CLAUDE.md, инвариант 3).
type Notes struct {
	service *notes.Service
}

// NewNotes создаёт сервис.
func NewNotes(service *notes.Service) *Notes {
	return &Notes{service: service}
}

// Search находит заметки по запросу на языке из SPEC §8.5.
//
// Пустой запрос означает все заметки. hideCompleted убирает завершённое и
// брошенное — так список ноутбука выглядит по умолчанию (SPEC §8.3).
func (n *Notes) Search(
	ctx context.Context, query string, limit int, hideCompleted bool, sortField string, reversed bool,
) ([]notes.Note, error) {
	return n.service.Search(ctx, query, notes.SearchOptions{
		Limit:         limit,
		HideCompleted: hideCompleted,
		Sort:          index.Sort{Field: sortField_(sortField), Reversed: reversed},
	})
}

// sortField_ переводит имя поля из интерфейса во внутреннее.
//
// Строкой, а не числом: числа в биндингах читаются как магия, а список полей
// закрыт и меняется вместе со спекой (SPEC §8.4).
func sortField_(name string) index.SortField {
	switch name {
	case "created":
		return index.SortCreated
	case "title":
		return index.SortTitle
	default:
		return index.SortUpdated
	}
}

// Tasks — что сейчас в работе: active и onHold из всех ноутбуков.
//
// Отдельный метод, а не запрос: язык соединяет условия только через И, а здесь
// нужно ИЛИ по статусам. Это псевдо-ноутбук «Активные», главный экран рабочего
// дня (SPEC §8.3).
func (n *Notes) Tasks(ctx context.Context, limit int) ([]notes.Note, error) {
	return n.service.Tasks(ctx, notes.TasksParams{Limit: limit})
}

// Get читает заметку целиком: тело, исходящие ссылки и бэклинки.
func (n *Notes) Get(ctx context.Context, id string) (notes.Note, error) {
	return n.service.Get(ctx, id)
}

// Create заводит заметку и возвращает её строку индекса.
func (n *Notes) Create(ctx context.Context, title, notebook string) (index.Record, error) {
	return n.service.Create(ctx, notes.CreateParams{Title: title, Notebook: notebook})
}

// Save сохраняет заголовок и тело открытой заметки.
//
// Отдельный метод, а не общий Update: редактор сохраняет ровно эти два поля, и
// пропускать их через общий путь значит рисковать задеть остальные.
func (n *Notes) Save(ctx context.Context, id, title, body string) (index.Record, error) {
	return n.service.Update(ctx, notes.UpdateParams{ID: id, Title: &title, Body: &body})
}

// SetStatus меняет статус заметки.
func (n *Notes) SetStatus(ctx context.Context, id, status string) (index.Record, error) {
	parsed, err := vault.ParseStatus(status)
	if err != nil {
		return index.Record{}, err
	}
	return n.service.SetStatus(ctx, id, parsed)
}

// Trash переносит заметку в корзину.
func (n *Notes) Trash(ctx context.Context, id string) error {
	return n.service.Trash(ctx, id)
}

// Массовые операции: одна блокировка и один коммит на всю пачку (SPEC §8.4).

func (n *Notes) TrashMany(ctx context.Context, ids []string) ([]index.Record, error) {
	return n.service.TrashMany(ctx, ids)
}

func (n *Notes) MoveMany(ctx context.Context, ids []string, notebook string) ([]index.Record, error) {
	return n.service.MoveMany(ctx, ids, notebook)
}

func (n *Notes) SetStatusMany(ctx context.Context, ids []string, status string) ([]index.Record, error) {
	parsed, err := vault.ParseStatus(status)
	if err != nil {
		return nil, err
	}
	return n.service.SetStatusMany(ctx, ids, parsed)
}

func (n *Notes) SetPinnedMany(ctx context.Context, ids []string, pinned bool) ([]index.Record, error) {
	return n.service.SetPinnedMany(ctx, ids, pinned)
}

// Move переносит заметку в другой ноутбук. Пустая строка — корень vault.
func (n *Notes) Move(ctx context.Context, id, notebook string) (index.Record, error) {
	return n.service.Update(ctx, notes.UpdateParams{ID: id, Notebook: &notebook})
}

// SetPinned закрепляет заметку или снимает закрепление.
func (n *Notes) SetPinned(ctx context.Context, id string, pinned bool) (index.Record, error) {
	return n.service.SetPinned(ctx, id, pinned)
}

// Duplicate создаёт копию заметки рядом с оригиналом.
func (n *Notes) Duplicate(ctx context.Context, id string) (index.Record, error) {
	return n.service.Duplicate(ctx, id)
}

// Trashed возвращает содержимое корзины.
func (n *Notes) Trashed(ctx context.Context, limit int) ([]notes.Note, error) {
	return n.service.Search(ctx, "", notes.SearchOptions{Limit: limit, Trash: index.TrashOnly})
}

// Restore возвращает заметку из корзины туда, откуда она уехала.
func (n *Notes) Restore(ctx context.Context, id string) (index.Record, error) {
	return n.service.Restore(ctx, id)
}

// Delete удаляет заметку насовсем. Только из корзины и только отсюда: агенту
// это не дано (docs/MCP.md §4).
func (n *Notes) Delete(ctx context.Context, id string) error {
	return n.service.Delete(ctx, id)
}

// RenameTag переименовывает тег во всех заметках одним коммитом (SPEC §8.2).
func (n *Notes) RenameTag(ctx context.Context, from, to string) ([]index.Record, error) {
	return n.service.RenameTag(ctx, from, to)
}

// Attachment — сохранённое вложение и готовая вставка для текста.
type Attachment struct {
	Path     string
	Image    bool
	Markdown string
}

// AddAttachment сохраняет вложение из вебвью.
//
// Данные приходят строкой base64, а не массивом байтов: через биндинги массив
// на несколько мегабайт едет числами по одному, и вставка скриншота
// превращается в мегабайты JSON.
func (n *Notes) AddAttachment(ctx context.Context, filename, encoded string) (Attachment, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return Attachment{}, fmt.Errorf("attachment %q: %w", filename, err)
	}
	saved, err := n.service.AddAttachment(ctx, filename, data)
	if err != nil {
		return Attachment{}, err
	}
	return Attachment{
		Path:     saved.Path,
		Image:    saved.Image,
		Markdown: vault.AttachmentMarkdown(saved, ""),
	}, nil
}

// DeleteTag убирает тег из всех заметок одним коммитом, вместе с корзиной.
func (n *Notes) DeleteTag(ctx context.Context, name string) ([]index.Record, error) {
	return n.service.DeleteTag(ctx, name)
}

// SetTags заменяет теги заметки целиком: поле под заголовком правится так.
func (n *Notes) SetTags(ctx context.Context, id string, tags []string) (index.Record, error) {
	return n.service.SetTags(ctx, id, tags)
}

// SetTagColor выбирает цвет тега из палитры; -1 снимает выбор.
func (n *Notes) SetTagColor(ctx context.Context, name string, color int) error {
	return n.service.SetTagColor(ctx, name, color)
}

// CreateNotebook заводит пустой ноутбук.
func (n *Notes) CreateNotebook(ctx context.Context, path string) error {
	return n.service.CreateNotebook(ctx, path)
}

// RenameNotebook переименовывает ноутбук вместе со всем содержимым.
func (n *Notes) RenameNotebook(ctx context.Context, from, to string) ([]index.Record, error) {
	return n.service.RenameNotebook(ctx, from, to)
}

// DeleteNotebook переносит содержимое ноутбука в корзину и убирает папку.
func (n *Notes) DeleteNotebook(ctx context.Context, path string) ([]index.Record, error) {
	return n.service.DeleteNotebook(ctx, path)
}

// Notebooks возвращает дерево ноутбуков со счётчиками.
func (n *Notes) Notebooks(ctx context.Context) ([]index.Notebook, error) {
	return n.service.Notebooks(ctx)
}

// Tags возвращает теги со счётчиками.
func (n *Notes) Tags(ctx context.Context) ([]index.Tag, error) {
	return n.service.Tags(ctx)
}

// Counts возвращает счётчики для верхних пунктов сайдбара.
func (n *Notes) Counts(ctx context.Context) (index.Counts, error) {
	return n.service.Counts(ctx)
}
