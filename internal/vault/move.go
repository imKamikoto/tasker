package vault

import (
	"fmt"
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
