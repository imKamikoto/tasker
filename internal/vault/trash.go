package vault

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// trashDirName — корзина. Единственный скрытый каталог, который vault не
// игнорирует (SPEC §4.1, §4.3).
const trashDirName = ".trash"

// ErrAlreadyTrashed — заметка уже в корзине.
var ErrAlreadyTrashed = errors.New("note already in trash")

// Trash переносит заметку в корзину, запоминая, откуда и когда она уехала.
//
// Удалить насовсем отсюда нельзя: агенту это не дано вовсе (docs/MCP.md §4), а
// пользователю — отдельной командой из приложения.
func (v *Vault) Trash(n *Note) error {
	if inTrash(n.Notebook) {
		return fmt.Errorf("trash %s: %w", n.Path, ErrAlreadyTrashed)
	}

	from := path.Join(n.Notebook, filepath.Base(n.Path))
	if err := n.Doc.Meta.Set(fieldTrashedFrom, from); err != nil {
		return fmt.Errorf("trash %s: %w", n.Path, err)
	}
	if err := n.Doc.Meta.setTimeField(fieldTrashedAt, v.now()); err != nil {
		return fmt.Errorf("trash %s: %w", n.Path, err)
	}

	// Сначала пишем поля на месте, потом переносим. В обратном порядке сбой
	// между шагами оставил бы в корзине заметку, про которую неизвестно, откуда
	// она там взялась, — то есть невосстановимую.
	if err := writeFileAtomic(n.Path, n.Doc.Bytes(), notePerm); err != nil {
		return fmt.Errorf("trash %s: %w", n.Path, err)
	}
	n.Doc.markClean()

	dir := filepath.Join(v.root, trashDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("trash %s: %w", n.Path, err)
	}

	base := filepath.Base(n.Path)
	slug := strings.TrimSuffix(base, filepath.Ext(base))
	moved, err := moveUnique(n.Path, dir, slug)
	if err != nil {
		return fmt.Errorf("trash %s: %w", n.Path, err)
	}

	info, err := os.Stat(moved)
	if err != nil {
		return fmt.Errorf("trash %s: %w", n.Path, err)
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

// inTrash — путь ноутбука ведёт в корзину.
func inTrash(notebook string) bool {
	return notebook == trashDirName || strings.HasPrefix(notebook, trashDirName+"/")
}

// moveUnique переносит файл в каталог под первым свободным именем.
//
// Через os.Link, а не os.Rename: rename молча затирает то, что уже лежит по
// целевому пути, а в корзине легко встречаются две заметки с одинаковым именем
// из разных ноутбуков.
func moveUnique(src, dir, slug string) (string, error) {
	dst, err := linkFirstFree(dir, slug, src, os.Link)
	if err != nil {
		return "", err
	}
	if err := os.Remove(src); err != nil {
		return "", fmt.Errorf("remove %s: %w", src, err)
	}
	if err := syncDir(dir); err != nil {
		return "", fmt.Errorf("fsync dir %s: %w", dir, err)
	}
	return dst, nil
}

// ErrNotTrashed — заметка не в корзине, а операция только для корзины.
var ErrNotTrashed = errors.New("note is not in trash")

// Restore возвращает заметку туда, откуда она уехала в корзину.
//
// Удалённый за это время ноутбук создаётся заново — так заметка оказывается
// ровно там, где была. В корень она попадает только если trashedFrom ведёт
// туда, куда писать нельзя: скрытый каталог или путь за пределы vault. Такое
// бывает у файла, поправленного руками, и терять из-за этого заметку нельзя.
func (v *Vault) Restore(n *Note) error {
	if !inTrash(n.Notebook) {
		return fmt.Errorf("restore %s: %w", n.Path, ErrNotTrashed)
	}

	from, err := n.Doc.Meta.TrashedFrom()
	if err != nil {
		return fmt.Errorf("restore %s: %w", n.Path, err)
	}

	notebook := path.Dir(from)
	if notebook == "." || from == "" {
		notebook = ""
	}
	dir, err := v.ensureNotebook(notebook)
	if err != nil {
		dir = v.root
	}

	base := filepath.Base(n.Path)
	slug := strings.TrimSuffix(base, filepath.Ext(base))
	moved, err := moveUnique(n.Path, dir, slug)
	if err != nil {
		return fmt.Errorf("restore %s: %w", n.Path, err)
	}

	// Сначала переносим, потом чистим поля. В обратном порядке сбой между
	// шагами оставил бы в корзине заметку без записи о том, откуда она, —
	// то есть невосстановимую.
	n.Path = moved
	n.Notebook = v.notebookOf(moved)
	n.Doc.Meta.Remove(fieldTrashedFrom)
	n.Doc.Meta.Remove(fieldTrashedAt)
	if err := writeFileAtomic(n.Path, n.Doc.Bytes(), notePerm); err != nil {
		return fmt.Errorf("restore %s: %w", n.Path, err)
	}
	n.Doc.markClean()

	info, err := os.Stat(n.Path)
	if err != nil {
		return fmt.Errorf("restore %s: %w", n.Path, err)
	}
	n.ModTime = info.ModTime()
	n.Size = info.Size()
	v.wrote(n.Path, n.ModTime)
	return nil
}

// Delete удаляет заметку насовсем.
//
// Только из корзины: удалить мимо неё нельзя ни человеку, ни тем более агенту —
// ему это не дано вовсе (docs/MCP.md §4). Восстановить после этого можно только
// из истории git.
func (v *Vault) Delete(n *Note) error {
	if !inTrash(n.Notebook) {
		return fmt.Errorf("delete %s: %w", n.Path, ErrNotTrashed)
	}
	if err := os.Remove(n.Path); err != nil {
		return fmt.Errorf("delete %s: %w", n.Path, err)
	}
	dir := filepath.Dir(n.Path)
	if err := syncDir(dir); err != nil {
		return fmt.Errorf("delete %s: %w", n.Path, err)
	}
	v.wrote(n.Path, time.Time{})
	return nil
}
