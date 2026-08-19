package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Backfill дописывает в заметку обязательные поля, которых в ней нет, и
// сохраняет файл. Возвращает false, если дописывать было нечего.
//
// Нужен для файлов, попавших в vault снаружи: у них может не быть frontmatter
// вовсе, а id — первичный ключ индекса и основа ссылок, взять его больше
// неоткуда (SPEC §4.1).
//
// Записывает сам, а не через Save, по одной причине: Save проставляет updated
// текущим временем, а здесь правки пользователя не было — были только наши
// служебные поля. Даты берутся из mtime, каким он был до нашей записи, и
// описывают, когда файл трогал человек.
//
// Существующие значения не перезаписываются никогда, включая id, который не
// похож на ULID: мы не знаем, чем он был и кто на него ссылается.
func (v *Vault) Backfill(n *Note) (bool, error) {
	f := n.Doc.Meta
	// mtime до записи: наша собственная запись его сдвинет, а датами заметки
	// это считаться не должно.
	edited := n.ModTime
	changed := false

	if f.ID() == "" {
		if err := f.setScalarRaw(fieldID, NewID()); err != nil {
			return false, fmt.Errorf("backfill %s: %w", n.Path, err)
		}
		changed = true
	}

	title, err := f.Title()
	if err != nil {
		return false, fmt.Errorf("backfill %s: %w", n.Path, err)
	}
	if strings.TrimSpace(title) == "" {
		if err := f.SetTitle(titleFromPath(n.Path, f.ID())); err != nil {
			return false, fmt.Errorf("backfill %s: %w", n.Path, err)
		}
		changed = true
	}

	created, err := f.Created()
	if err != nil {
		return false, fmt.Errorf("backfill %s: %w", n.Path, err)
	}
	if created.IsZero() {
		if err := f.SetCreated(edited); err != nil {
			return false, fmt.Errorf("backfill %s: %w", n.Path, err)
		}
		changed = true
	}

	updated, err := f.Updated()
	if err != nil {
		return false, fmt.Errorf("backfill %s: %w", n.Path, err)
	}
	if updated.IsZero() {
		if err := f.SetUpdated(edited); err != nil {
			return false, fmt.Errorf("backfill %s: %w", n.Path, err)
		}
		changed = true
	}

	if !changed {
		return false, nil
	}

	if err := writeFileAtomic(n.Path, n.Doc.Bytes(), notePerm); err != nil {
		return false, fmt.Errorf("backfill %s: %w", n.Path, err)
	}
	n.Doc.markClean()

	info, err := os.Stat(n.Path)
	if err != nil {
		return false, fmt.Errorf("backfill %s: %w", n.Path, err)
	}
	n.ModTime = info.ModTime()
	n.Size = info.Size()
	return true, nil
}

// titleFromPath делает заголовок из имени файла.
//
// Имя файла, а не первый H1 в теле: H1 заголовком не считается (SPEC §4.2), и
// угадывать по содержимому — значит иногда угадывать неверно. Имя файла
// предсказуемо, а поправить заголовок в приложении — одна строка.
func titleFromPath(path, fallback string) string {
	base := filepath.Base(path)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if strings.TrimSpace(stem) == "" {
		return fallback
	}
	return stem
}
