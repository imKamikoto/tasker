package vault

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Move переносит заметку в другой ноутбук, создавая его при необходимости.
//
// Имя файла и id сохраняются: на id держатся ссылки, а имя после создания
// автоматически не меняется (SPEC §4.1). Если имя в новом ноутбуке занято,
// добавляется суффикс.
//
// updated не трогается: перемещение — не правка содержимого, и выдавать его за
// правку значит врать в списке, отсортированном по дате изменения.
func (v *Vault) Move(n *Note, notebook string) error {
	dir, err := v.ensureNotebook(notebook)
	if err != nil {
		return err
	}
	if filepath.Dir(n.Path) == dir {
		return nil
	}

	base := filepath.Base(n.Path)
	slug := strings.TrimSuffix(base, filepath.Ext(base))
	moved, err := moveUnique(n.Path, dir, slug)
	if err != nil {
		return fmt.Errorf("move %s to %q: %w", n.Path, notebook, err)
	}

	info, err := os.Stat(moved)
	if err != nil {
		return fmt.Errorf("move %s to %q: %w", n.Path, notebook, err)
	}

	// Про оба пути: и откуда файл исчез, и куда появился — событие придёт по
	// каждому из них.
	v.wrote(n.Path, n.ModTime)
	n.Path = moved
	n.Notebook = v.notebookOf(moved)
	n.ModTime = info.ModTime()
	n.Size = info.Size()
	v.wrote(n.Path, n.ModTime)
	return nil
}

// EnsureNotebook создаёт ноутбук и возвращает путь к нему.
//
// Экспортирована ради пустых ноутбуков: их заводят до того, как появится первая
// заметка, и правила про скрытые имена и выход за пределы должны быть теми же.
func (v *Vault) EnsureNotebook(notebook string) (string, error) {
	return v.ensureNotebook(notebook)
}

// RemoveEmptyNotebook убирает опустевший каталог ноутбука вместе с опустевшими
// родителями.
//
// Только пустые: непустой каталог остаётся на месте молча. Удалять содержимое
// здесь нельзя — заметки уходят в корзину отдельным шагом (SPEC §8.1).
func (v *Vault) RemoveEmptyNotebook(notebook string) error {
	clean, err := v.cleanRelative(notebook)
	if err != nil || clean == "" {
		return err
	}

	dir := filepath.Join(v.root, clean)

	// Сначала опустевшие вложенные, снизу вверх: без этого сам каталог не
	// удалится — он не пуст, пока внутри лежат пустые подкаталоги.
	var nested []string
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && entry.IsDir() && path != dir {
			nested = append(nested, path)
		}
		return nil
	})
	for i := len(nested) - 1; i >= 0; i-- {
		// os.Remove на каталоге срабатывает только если он пуст — ровно то,
		// что нужно: непустой останется на месте.
		_ = os.Remove(nested[i])
	}

	// Теперь сам каталог и опустевшие родители.
	for dir != v.root {
		if err := os.Remove(dir); err != nil {
			return nil
		}
		dir = filepath.Dir(dir)
	}
	return nil
}
