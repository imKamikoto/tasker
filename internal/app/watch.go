package app

import (
	"context"
	"errors"
	"path/filepath"

	"tasker/internal/index"
	"tasker/internal/notes"
	"tasker/internal/watcher"
)

// Имена событий, которые ядро шлёт в интерфейс (SPEC §6).
const (
	// EventNotesChanged — список заметок надо перечитать. Без нагрузки.
	EventNotesChanged = "tasker:notes-changed"
	// EventNoteChanged — конкретная заметка изменилась на диске.
	EventNoteChanged = "tasker:note-changed"
)

// NoteChanged — какая заметка изменилась.
type NoteChanged struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

// Watch связывает наблюдение за файлами с интерфейсом.
//
// Заметка, созданная агентом через tasker-mcp при открытом приложении, должна
// появиться в списке сама — это самый важный шаг сценария приёмки (MCP.md §6),
// потому что он проверяет всю цепочку разом: чужой процесс пишет файл, watcher
// его замечает, индекс обновляется, событие доходит до окна.
type Watch struct {
	service *notes.Service
	emit    func(name string, data any)
	onError func(error)
}

// NewWatch создаёт цикл. emit получает имя события и полезную нагрузку —
// функцией, а не объектом Wails, чтобы это можно было проверить без окна.
func NewWatch(service *notes.Service, emit func(name string, data any), onError func(error)) *Watch {
	if onError == nil {
		onError = func(error) {}
	}
	return &Watch{service: service, emit: emit, onError: onError}
}

// Run обрабатывает пакеты изменений до отмены контекста.
func (w *Watch) Run(ctx context.Context, batches <-chan watcher.Batch) {
	for {
		select {
		case <-ctx.Done():
			return
		case batch, ok := <-batches:
			if !ok {
				return
			}
			w.handle(ctx, batch)
		}
	}
}

func (w *Watch) handle(ctx context.Context, batch watcher.Batch) {
	// Сверка инкрементальная и по пакету, и по всему дереву: watcher — только
	// оптимизация задержки, правду он не знает (SPEC §5.3).
	result, err := w.service.Sync(ctx)
	if err != nil {
		w.onError(err)
		return
	}
	for _, failure := range result.Failed {
		w.onError(failure)
	}

	// Считать только по результату скана нельзя: tasker-mcp обновляет индекс
	// сам, и к моменту нашей сверки там уже всё на месте — Added будет нулём,
	// хотя заметка только что появилась. Наличие путей в пакете значит, что
	// файлы менял кто-то другой (свои записи watcher отфильтровал), и список
	// надо перечитать независимо от того, кто успел раньше.
	if result.Added+result.Updated+result.Removed > 0 || len(batch.Paths) > 0 {
		// Без нагрузки: числа описывали бы наш скан, а он в главном сценарии
		// законно находит ноль — индекс уже обновил агент. Такие числа только
		// вводят в заблуждение, а интерфейсу нужен сам сигнал «перечитай».
		w.emit(EventNotesChanged, nil)
	}

	// Про каждую конкретную заметку — отдельно: открытый редактор слушает
	// именно это событие, а перечитывать заметку на каждое изменение списка
	// значило бы дёргать буфер под руками.
	root := w.service.Vault().Root()
	for _, path := range batch.Paths {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			continue
		}
		record, err := w.service.Index().GetByPath(ctx, filepath.ToSlash(rel))
		if errors.Is(err, index.ErrNotFound) {
			// Файл удалили — списка это касается, отдельной заметки уже нет.
			continue
		}
		if err != nil {
			w.onError(err)
			continue
		}
		w.emit(EventNoteChanged, NoteChanged{ID: record.ID, Path: record.Path})
	}
}
