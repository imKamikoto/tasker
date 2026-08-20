package app

import (
	"context"

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
// Пустой запрос означает все заметки.
func (n *Notes) Search(ctx context.Context, query string, limit int) ([]notes.Note, error) {
	return n.service.Search(ctx, query, notes.SearchOptions{Limit: limit})
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

// Notebooks возвращает дерево ноутбуков со счётчиками.
func (n *Notes) Notebooks(ctx context.Context) ([]index.Notebook, error) {
	return n.service.Notebooks(ctx)
}

// Tags возвращает теги со счётчиками.
func (n *Notes) Tags(ctx context.Context) ([]index.Tag, error) {
	return n.service.Tags(ctx)
}
