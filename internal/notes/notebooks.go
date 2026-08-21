package notes

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"tasker/internal/index"
	"tasker/internal/vault"
)

// attachmentsDir не ноутбук: там лежат картинки, а не заметки (SPEC §4.4).
const attachmentsDir = "attachments"

// Notebooks возвращает дерево ноутбуков.
//
// Каталоги обходятся по диску, а не берутся из индекса: ноутбук — это папка
// (SPEC §4.1), и пустая папка тоже ноутбук. Индекс знает только про папки, в
// которых лежат заметки, поэтому только что созданный ноутбук из него не виден.
func (s *Service) Notebooks(ctx context.Context) ([]index.Notebook, error) {
	counted, err := s.index.Notebooks(ctx)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(counted))
	for _, notebook := range counted {
		counts[notebook.Path] = notebook.Count
	}

	dirs, err := s.notebookDirs()
	if err != nil {
		return nil, err
	}
	// Ноутбук мог остаться в индексе от заметки, которую только что удалили с
	// диска мимо нас: показываем и такие, иначе счётчик исчезнет вместе с
	// папкой ещё до сверки.
	for path := range counts {
		if !dirs[path] {
			dirs[path] = true
		}
	}

	paths := make([]string, 0, len(dirs))
	for path := range dirs {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	notebooks := make([]index.Notebook, 0, len(paths))
	for _, path := range paths {
		var children []string
		for _, other := range paths {
			if other != "" && parentOf(other) == path {
				children = append(children, other)
			}
		}
		notebooks = append(notebooks, index.Notebook{
			Path:     path,
			Count:    counts[path],
			Children: children,
		})
	}
	return notebooks, nil
}

// notebookDirs собирает папки vault, которые считаются ноутбуками.
func (s *Service) notebookDirs() (map[string]bool, error) {
	root := s.vault.Root()
	dirs := map[string]bool{"": true}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			// Нечитаемая папка — не повод не показать остальные.
			if path == root {
				return err
			}
			return fs.SkipDir
		}
		if !entry.IsDir() || path == root {
			return nil
		}
		// Скрытые каталоги и вложения ноутбуками не считаются: корзина живёт
		// отдельным пунктом сайдбара, а во вложениях нет заметок.
		if strings.HasPrefix(entry.Name(), ".") || entry.Name() == attachmentsDir {
			return fs.SkipDir
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		dirs[filepath.ToSlash(rel)] = true
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list notebooks: %w", err)
	}
	return dirs, nil
}

func parentOf(path string) string {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return ""
	}
	return path[:i]
}

// CreateNotebook заводит пустой ноутбук.
func (s *Service) CreateNotebook(ctx context.Context, path string) error {
	release, err := s.lock.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Проверку пути и создание каталога делает vault: правила о скрытых
	// именах и выходе за пределы живут там (SPEC §4.1).
	_, err = s.vault.EnsureNotebook(path)
	return err
}

// RenameNotebook переименовывает ноутбук вместе со всем содержимым.
//
// Заметки переносятся по одной, но одной блокировкой и одним коммитом:
// переименование папки с двадцатью заметками — одно действие пользователя, и в
// истории оно должно выглядеть одним.
func (s *Service) RenameNotebook(ctx context.Context, from, to string) ([]index.Record, error) {
	from, to = strings.Trim(from, "/"), strings.Trim(to, "/")
	if from == "" {
		return nil, fmt.Errorf("rename notebook: %w", vault.ErrHiddenPath)
	}
	if from == to {
		return nil, nil
	}

	inside, err := s.Search(ctx, "book:"+quoteTerm(from), SearchOptions{Trash: index.TrashHidden})
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(inside))
	targets := make(map[string]string, len(inside))
	for _, note := range inside {
		ids = append(ids, note.ID)
		// Вложенные ноутбуки переезжают вместе: Работа/Баги → Дефекты/Баги.
		targets[note.ID] = to + strings.TrimPrefix(note.Notebook, from)
	}

	moved, err := s.applyMany(ctx, ids, "rename notebook", func(n *vault.Note) error {
		return s.vault.Move(n, targets[n.Doc.Meta.ID()])
	})
	if err != nil {
		return moved, err
	}

	// Опустевшие каталоги убираем: иначе старое имя останется в сайдбаре.
	if err := s.vault.RemoveEmptyNotebook(from); err != nil {
		return moved, err
	}
	return moved, nil
}

// DeleteNotebook переносит содержимое ноутбука в корзину и убирает папку.
//
// Насовсем не удаляет ничего: заметки уходят в корзину, откуда их ещё можно
// вернуть (SPEC §8.1).
func (s *Service) DeleteNotebook(ctx context.Context, path string) ([]index.Record, error) {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil, fmt.Errorf("delete notebook: %w", vault.ErrHiddenPath)
	}

	inside, err := s.Search(ctx, "book:"+quoteTerm(path), SearchOptions{Trash: index.TrashHidden})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(inside))
	for _, note := range inside {
		ids = append(ids, note.ID)
	}

	trashed, err := s.applyMany(ctx, ids, "delete notebook", s.vault.Trash)
	if err != nil {
		return trashed, err
	}
	if err := s.vault.RemoveEmptyNotebook(path); err != nil {
		return trashed, err
	}
	return trashed, nil
}
